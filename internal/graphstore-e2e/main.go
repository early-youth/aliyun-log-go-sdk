//go:build e2e

// A single-operation bridge for the GraphStore E2E Runner. No scenario or retry logic.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	sls "github.com/aliyun/aliyun-log-go-sdk"
	"github.com/gogo/protobuf/proto"
)

type invocation struct {
	Action               string          `json:"action"`
	Endpoint             string          `json:"endpoint"`
	DialAddress          string          `json:"dial_address"`
	Project              string          `json:"project"`
	StoreName            string          `json:"store_name"`
	TimeoutMS            int             `json:"timeout_ms"`
	SignatureVersion     string          `json:"signature_version"`
	Region               string          `json:"region"`
	BadCredentials       bool            `json:"bad_credentials"`
	Body                 json.RawMessage `json:"body_json"`
	Offset               *int            `json:"offset"`
	Size                 *int            `json:"size"`
	PageSize             int             `json:"page_size"`
	PageToken            string          `json:"page_token"`
	CollectionSchemaName string          `json:"collection_schema_name"`
	ShardID              int32           `json:"shard_id"`
	SplitHash            *uint64         `json:"split_hash"`
	RequestBase64        string          `json:"request_base64"`
	Compression          string          `json:"compression"`
	Query                string          `json:"query"`
	Start                *int64          `json:"start"`
	End                  *int64          `json:"end"`
	Session              string          `json:"session"`
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func main() {
	var p invocation
	decoder := json.NewDecoder(os.Stdin)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		panic(err)
	}
	key, secret := os.Getenv("GRAPHSTORE_E2E_ACCESS_KEY_ID"), os.Getenv("GRAPHSTORE_E2E_ACCESS_KEY_SECRET")
	if key == "" || secret == "" || p.TimeoutMS <= 0 {
		panic("missing E2E credentials or timeout")
	}
	if p.BadCredentials {
		secret = "intentionally-invalid-e2e-secret"
	}
	client := sls.CreateNormalInterface(p.Endpoint, key, secret, "")
	if p.SignatureVersion == "v4" {
		client.SetAuthVersion(sls.AuthV4)
		client.SetRegion(p.Region)
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		if p.DialAddress != "" {
			address = p.DialAddress
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}}
	defer transport.CloseIdleConnections()
	requestStarted := false
	responseStatus, responseRequestID := 0, ""
	client.SetHTTPClient(&http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		requestStarted = true
		response, err := transport.RoundTrip(request)
		if response != nil {
			responseStatus, responseRequestID = response.StatusCode, response.Header.Get(sls.RequestIDHeader)
		}
		return response, err
	}), Timeout: time.Duration(p.TimeoutMS) * time.Millisecond})
	var result interface{}
	var err error
	output := map[string]interface{}{"http_status": 200}
	switch p.Action {
	case "PostGraphStoreData":
		compression, ok := map[string]int{"none": sls.Compress_None, "lz4": sls.Compress_LZ4, "zstd": sls.Compress_ZSTD}[p.Compression]
		if !ok {
			panic("unsupported GraphStore write compression")
		}
		raw, decodeErr := base64.StdEncoding.DecodeString(p.RequestBase64)
		group := &sls.LogGroup{}
		if decodeErr != nil || proto.Unmarshal(raw, group) != nil || len(group.Logs) == 0 {
			panic("E2E write requires a nonempty standard LogGroup")
		}
		err = client.PostGraphStoreData(p.Project, p.StoreName, &sls.PostGraphStoreDataRequest{LogGroup: group, CompressType: compression})
	case "GetGraphStoreData":
		if p.Start == nil || p.End == nil {
			panic("GraphStore query requires start and end")
		}
		var rows *sls.GetGraphStoreDataResponse
		rows, err = client.GetGraphStoreData(p.Project, p.StoreName, &sls.GetGraphStoreDataRequest{Query: p.Query, Start: *p.Start, End: *p.End, Session: p.Session})
		if err == nil {
			output["request_id"], output["header"], output["data"] = rows.RequestID, rows.Data.Fields, rows.Data.Values
			output["meta"] = map[string]interface{}{
				"row_count": len(rows.Data.Values), "truncated": rows.Meta.Truncated,
				"elapsed_ms": rows.Meta.ElapsedMs, "warnings": rows.Meta.Warnings,
			}
		}
	case "CreateGraphStore":
		var req sls.CreateGraphStoreRequest
		if err = json.Unmarshal(p.Body, &req); err == nil {
			err = client.CreateGraphStore(p.Project, &req)
		}
	case "GetGraphStore":
		result, err = client.GetGraphStore(p.Project, p.StoreName)
	case "ListGraphStores":
		result, err = client.ListGraphStores(p.Project, &sls.ListGraphStoresRequest{Offset: p.Offset, Size: p.Size})
	case "DeleteGraphStore":
		err = client.DeleteGraphStore(p.Project, p.StoreName)
	case "SplitGraphStoreShard":
		err = client.SplitGraphStoreShard(p.Project, p.StoreName, p.ShardID, p.SplitHash)
	case "GetGraphStoreBaseSchema":
		result, err = client.GetGraphStoreBaseSchema(p.Project, p.StoreName)
	case "CreateGraphStoreCollectionSchema":
		var schema sls.GraphStoreCollectionSchema
		if err = json.Unmarshal(p.Body, &schema); err == nil {
			err = client.CreateGraphStoreCollectionSchema(p.Project, p.StoreName, &schema)
		}
	case "GetGraphStoreCollectionSchema":
		result, err = client.GetGraphStoreCollectionSchema(p.Project, p.StoreName, p.CollectionSchemaName)
	case "ListGraphStoreCollectionSchemas":
		result, err = client.ListGraphStoreCollectionSchemas(p.Project, p.StoreName, p.PageSize, p.PageToken)
	case "UpdateGraphStoreCollectionSchema":
		var schema sls.GraphStoreCollectionSchemaUpdate
		if err = json.Unmarshal(p.Body, &schema); err == nil {
			err = client.UpdateGraphStoreCollectionSchema(p.Project, p.StoreName, p.CollectionSchemaName, &schema)
		}
	case "DeleteGraphStoreCollectionSchema":
		err = client.DeleteGraphStoreCollectionSchema(p.Project, p.StoreName, p.CollectionSchemaName)
	default:
		panic("unsupported GraphStore operation")
	}
	if p.Action != "PostGraphStoreData" && p.Action != "GetGraphStoreData" {
		output["request_id"], output["body"] = responseRequestID, result
	}
	if responseStatus != 0 {
		output["http_status"] = responseStatus
	}
	if err != nil {
		serviceError, ok := err.(*sls.Error)
		if ok {
			output["http_status"] = serviceError.HTTPCode
			output["request_id"] = serviceError.RequestID
			output["error_code"] = serviceError.Code
			output["error_message"] = serviceError.Message
		} else if !requestStarted {
			output["http_status"], output["request_id"] = 0, ""
			output["local_error"], output["error_code"] = true, "SDK_LOCAL_ERROR"
			output["error_message"] = err.Error()
		} else {
			fmt.Fprintln(os.Stderr, "SDK transport/decode error:", err)
			os.Exit(2)
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
		panic(err)
	}
}
