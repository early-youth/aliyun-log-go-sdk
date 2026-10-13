package sls

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gogo/protobuf/proto"
	"github.com/pierrec/lz4/v4"
)

// PostGraphStoreDataRequest uses the same LogGroup and compression options as PostLogStoreLogs.
type PostGraphStoreDataRequest struct {
	LogGroup     *LogGroup
	CompressType int // Zero defaults to Compress_LZ4.
}

type GetGraphStoreDataRequest struct {
	Query   string `json:"query"`
	Start   int64  `json:"start"`
	End     int64  `json:"end"`
	Session string `json:"session,omitempty"`
}

// NewGetGraphStoreDataRequest defaults to the full event-time range.
func NewGetGraphStoreDataRequest(query string) *GetGraphStoreDataRequest {
	return &GetGraphStoreDataRequest{Query: query, End: 4294967295}
}

type GraphStoreQueryMeta struct {
	ElapsedMs int64    `json:"elapsedMs"`
	Truncated bool     `json:"truncated"`
	Warnings  []string `json:"warnings"`
}

type GraphStoreQueryData struct {
	Fields []string    `json:"fields"`
	Values [][]*string `json:"values"`
}

type GetGraphStoreDataResponse struct {
	RequestID string              `json:"-"`
	Data      GraphStoreQueryData `json:"data"`
	Meta      GraphStoreQueryMeta `json:"meta"`
}

// PostGraphStoreData submits a LogGroup using the existing LogStore retry policy.
// A nil/empty group is a local no-op; an actual ACK does not imply query visibility or durability.
// A timeout can leave the write result unknown, and retries may duplicate delivery.
func (c *Client) PostGraphStoreData(project, graphStore string, req *PostGraphStoreDataRequest) error {
	if req == nil {
		return NewClientError(fmt.Errorf("PostGraphStoreData request is required"))
	}
	if req.CompressType < 0 || req.CompressType >= Compress_Max {
		return InvalidCompressError
	}
	if req.LogGroup == nil || len(req.LogGroup.Logs) == 0 {
		return nil
	}
	if project == "" {
		return NewClientError(fmt.Errorf("project is required"))
	}
	uri, err := graphStorePath(graphStore)
	if err != nil {
		return NewClientError(err)
	}
	body, err := proto.Marshal(req.LogGroup)
	if err != nil {
		return NewClientError(err)
	}
	headers := map[string]string{
		HTTPHeaderContentType: "application/x-protobuf",
		HTTPHeaderBodyRawSize: strconv.Itoa(len(body)),
	}
	switch req.CompressType {
	case Compress_LZ4:
		out := make([]byte, lz4.CompressBlockBound(len(body)))
		n, err := lz4.CompressBlock(body, out, nil)
		if err != nil {
			return NewClientError(err)
		}
		if n == 0 {
			n, _ = copyIncompressible(body, out)
		}
		body = out[:n]
		headers["x-log-compresstype"] = "lz4"
	case Compress_ZSTD:
		body, err = slsZstdCompressor.Compress(body, nil)
		if err != nil {
			return NewClientError(err)
		}
		headers["x-log-compresstype"] = "zstd"
	}
	response, err := request(convert(c, project), http.MethodPost, uri+"/write", headers, body)
	if err != nil {
		return NewClientError(err)
	}
	defer response.Body.Close()
	if _, err = io.Copy(io.Discard, response.Body); err != nil {
		return readResponseError(err)
	}
	return nil
}

// GetGraphStoreData returns a fully validated row result. Complex graph values stay strings.
func (c *Client) GetGraphStoreData(project, graphStore string, req *GetGraphStoreDataRequest) (*GetGraphStoreDataResponse, error) {
	if project == "" || req == nil || req.Query == "" || req.Start < 0 || req.Start > req.End {
		return nil, NewClientError(fmt.Errorf("project, GraphStore query and 0 <= start <= end are required"))
	}
	uri, err := graphStorePath(graphStore)
	if err != nil {
		return nil, NewClientError(err)
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, NewClientError(err)
	}
	headers := map[string]string{
		HTTPHeaderContentType: "application/json",
		HTTPHeaderBodyRawSize: strconv.Itoa(len(body)),
		"Accept-Encoding":     "lz4",
	}
	response, err := request(convert(c, project), http.MethodPost, uri+"/query", headers, body)
	if err != nil {
		return nil, NewClientError(err)
	}
	defer response.Body.Close()
	body, err = io.ReadAll(response.Body)
	if err != nil {
		return nil, readResponseError(err)
	}
	switch compression := response.Header.Get("x-log-compresstype"); compression {
	case "":
	case "lz4":
		rawSize, sizeErr := strconv.ParseUint(response.Header.Get(HTTPHeaderBodyRawSize), 10, 32)
		if sizeErr != nil || rawSize > 256*1024*1024 {
			return nil, newBadResponseError("", response.Header, response.StatusCode, fmt.Errorf("invalid GraphStore result raw size"))
		}
		body, err = decompressResponse(int(rawSize), body, response)
		if err != nil {
			return nil, newBadResponseError("", response.Header, response.StatusCode, err)
		}
	default:
		return nil, newBadResponseError("", response.Header, response.StatusCode, fmt.Errorf("unsupported GraphStore result compression: %s", compression))
	}
	result, err := decodeGraphStoreQuery(body)
	if err != nil {
		return nil, newBadResponseError(string(body), response.Header, response.StatusCode, err)
	}
	result.RequestID = response.Header.Get(RequestIDHeader)
	return result, nil
}

func decodeGraphStoreQuery(body []byte) (*GetGraphStoreDataResponse, error) {
	result := &GetGraphStoreDataResponse{}
	var header, warnings []*string
	var data, meta json.RawMessage
	fields, err := decodeGraphStoreFields(body, map[string]interface{}{"data": &data, "meta": &meta})
	if err != nil {
		return nil, err
	}
	if len(fields) != 2 {
		return nil, fmt.Errorf("invalid GraphStore query response fields")
	}
	fields, err = decodeGraphStoreFields(data, map[string]interface{}{"fields": &header, "values": &result.Data.Values})
	if err != nil {
		return nil, err
	}
	if len(fields) != 2 {
		return nil, fmt.Errorf("invalid GraphStore query data fields")
	}
	fields, err = decodeGraphStoreFields(meta, map[string]interface{}{
		"elapsedMs": &result.Meta.ElapsedMs,
		"truncated": &result.Meta.Truncated, "warnings": &warnings,
	})
	if err != nil {
		return nil, err
	}
	if len(fields) != 3 {
		return nil, fmt.Errorf("invalid GraphStore query metadata fields")
	}
	result.Data.Fields = make([]string, len(header))
	for i, column := range header {
		if column == nil {
			return nil, fmt.Errorf("GraphStore query header must contain strings")
		}
		result.Data.Fields[i] = *column
	}
	result.Meta.Warnings = make([]string, len(warnings))
	for i, warning := range warnings {
		if warning == nil {
			return nil, fmt.Errorf("GraphStore query warnings must contain strings")
		}
		result.Meta.Warnings[i] = *warning
	}
	for _, row := range result.Data.Values {
		if row == nil || len(row) != len(result.Data.Fields) {
			return nil, fmt.Errorf("invalid GraphStore query row")
		}
	}
	return result, nil
}
