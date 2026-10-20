package sls

import (
	"bytes"
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
	Fields []string        `json:"fields"`
	Values [][]interface{} `json:"values"`
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

// GetGraphStoreData preserves JSON values; numbers use json.Number to retain integer precision.
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
		rawSize, sizeErr := strconv.ParseUint(response.Header.Get(HTTPHeaderBodyRawSize), 10, strconv.IntSize-1)
		if sizeErr != nil {
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
	var value struct {
		Data *struct {
			Fields *[]string        `json:"fields"`
			Values *[][]interface{} `json:"values"`
		} `json:"data"`
		Meta *struct {
			ElapsedMs *int64    `json:"elapsedMs"`
			Truncated *bool     `json:"truncated"`
			Warnings  *[]string `json:"warnings"`
		} `json:"meta"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("invalid trailing GraphStore query content: %v", err)
	}
	if value.Data == nil || value.Meta == nil || value.Data.Fields == nil || value.Data.Values == nil ||
		value.Meta.ElapsedMs == nil || value.Meta.Truncated == nil || value.Meta.Warnings == nil {
		return nil, fmt.Errorf("missing or null GraphStore query field")
	}
	return &GetGraphStoreDataResponse{
		Data: GraphStoreQueryData{Fields: *value.Data.Fields, Values: *value.Data.Values},
		Meta: GraphStoreQueryMeta{ElapsedMs: *value.Meta.ElapsedMs, Truncated: *value.Meta.Truncated, Warnings: *value.Meta.Warnings},
	}, nil
}
