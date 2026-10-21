package main

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	sls "github.com/aliyun/aliyun-log-go-sdk"
)

type sampleClient struct {
	sls.ClientInterface
	createErr, readErr, deleteErr, absentErr error
	deletes                                  int
	queries                                  []*sls.GetGraphStoreDataResponse
	writes                                   []*sls.PostGraphStoreDataRequest
}

func (c *sampleClient) CreateGraphStore(string, *sls.CreateGraphStoreRequest) error {
	return c.createErr
}
func (c *sampleClient) GetGraphStore(string, string) (*sls.GetGraphStoreResponse, error) {
	if c.deletes > 0 {
		return nil, c.absentErr
	}
	return nil, c.readErr
}
func (c *sampleClient) DeleteGraphStore(string, string) error {
	c.deletes++
	return c.deleteErr
}
func (c *sampleClient) PostGraphStoreData(_, _ string, req *sls.PostGraphStoreDataRequest) error {
	c.writes = append(c.writes, req)
	return nil
}
func (c *sampleClient) GetGraphStoreData(_, _ string, req *sls.GetGraphStoreDataRequest) (*sls.GetGraphStoreDataResponse, error) {
	if req.ResultFormat != "typed_json" {
		panic("sample must explicitly request typed_json")
	}
	if req.Start != 0 || req.End != 4294967295 {
		panic("query must use the full event-time range")
	}
	if len(c.queries) == 0 {
		return nil, c.readErr
	}
	response := c.queries[0]
	c.queries = c.queries[1:]
	return response, nil
}

func TestCreateAndCleanupFailures(t *testing.T) {
	primary := errors.New("business failure")
	for _, tc := range []struct {
		name                 string
		createErr, deleteErr error
		wantDeletes          int
	}{
		{"unknown Create result", primary, nil, 0},
		{"business failure cleans its Store", nil, nil, 1},
		{"cleanup failure preserves business error", nil, errors.New("cleanup failure"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &sampleClient{createErr: tc.createErr, readErr: primary, deleteErr: tc.deleteErr,
				absentErr: &sls.Error{HTTPCode: 404, Code: "GraphStoreNotExist"}}
			s := sample{client: client, project: "project", store: "own-store"}
			if err := s.run(); err != primary {
				t.Fatalf("lost original error: %v", err)
			}
			if client.deletes != tc.wantDeletes {
				t.Fatalf("Delete calls = %d, want %d", client.deletes, tc.wantDeletes)
			}
		})
	}
}

func TestDeleteRequiresExactAbsence(t *testing.T) {
	for _, tc := range []struct {
		code   string
		status int32
		ok     bool
	}{
		{"GraphStoreNotExist", 404, true}, {"ProjectNotExist", 404, false}, {"GraphStoreNotExist", 403, false},
	} {
		t.Run(tc.code+http.StatusText(int(tc.status)), func(t *testing.T) {
			client := &sampleClient{absentErr: &sls.Error{Code: tc.code, HTTPCode: tc.status}}
			err := (sample{client: client}).deleteStore()
			if (err == nil) != tc.ok {
				t.Fatalf("delete result = %v, want success=%t", err, tc.ok)
			}
		})
	}
}

func TestReadbackRejectsMismatchAndTruncation(t *testing.T) {
	stop := errors.New("stop polling")
	for _, tc := range []struct {
		name   string
		fields []string
		rows   [][]interface{}
		status string
	}{
		{"wrong fields", []string{"other"}, [][]interface{}{{typedValue("String", "")}}, "success"},
		{"null differs from empty string", []string{"name"}, [][]interface{}{{typedValue("Null", nil)}}, "success"},
		{"wrong value", []string{"name"}, [][]interface{}{{typedValue("String", "other")}}, "success"},
		{"duplicate row", []string{"name"}, [][]interface{}{{typedValue("String", "")}, {typedValue("String", "")}}, "success"},
		{"plain cell", []string{"name"}, [][]interface{}{{""}}, "success"},
		{"wrong tag", []string{"name"}, [][]interface{}{{typedValue("Integer", "")}}, "success"},
		{"truncated", []string{"name"}, [][]interface{}{{typedValue("String", "")}}, "incomplete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &sampleClient{readErr: stop, queries: []*sls.GetGraphStoreDataResponse{{
				Data: sls.GraphStoreQueryData{Fields: tc.fields, Values: tc.rows}, Status: tc.status}}}
			err := (sample{client: client}).waitForRows(personQuery, []string{"name"}, [][]interface{}{{typedValue("String", "")}})
			if err != stop {
				t.Fatalf("accepted incorrect readback: %v", err)
			}
		})
	}
}

func TestDataCRUD(t *testing.T) {
	client := &sampleClient{}
	for _, data := range []sls.GraphStoreQueryData{
		{Fields: []string{"name", "age"}, Values: [][]interface{}{{typedValue("String", "Alice"), typedValue("Integer", "30")}, {typedValue("String", "Bob"), typedValue("Integer", "28")}}},
		{Fields: []string{"source", "target"}, Values: [][]interface{}{{typedValue("String", "Alice"), typedValue("String", "Bob")}}},
		{Fields: []string{"name", "age"}, Values: [][]interface{}{{typedValue("String", "Alice"), typedValue("Integer", "31")}, {typedValue("String", "Bob"), typedValue("Integer", "28")}}},
		{Fields: []string{"source", "target"}, Values: [][]interface{}{}},
		{Fields: []string{"name", "age"}, Values: [][]interface{}{}},
	} {
		client.queries = append(client.queries, &sls.GetGraphStoreDataResponse{Data: data, Status: "success"})
	}
	if err := (sample{client: client}).writeAndQuery(); err != nil {
		t.Fatal(err)
	}
	if len(client.writes) != 4 || len(client.queries) != 0 {
		t.Fatal("incomplete data CRUD")
	}
	for i, req := range client.writes {
		if req.CompressType != sls.Compress_LZ4 {
			t.Fatal("not using LZ4")
		}
		for _, log := range req.LogGroup.Logs {
			fields := map[string]string{}
			for _, field := range log.Contents {
				fields[field.GetKey()] = field.GetValue()
			}
			if fields["__method__"] != []string{"create", "update", "delete", "delete"}[i] {
				t.Fatalf("invalid event: %v", fields)
			}
			if fields["__layout__"] == "edge" && (fields["sid"] != aliceID || fields["tid"] != bobID) {
				t.Fatal("edge endpoints do not match vertices")
			}
			if i == 0 && fields["__layout__"] == "edge" && fields["weight"] == "" {
				t.Fatal("edge create needs a property")
			}
			if i == 1 && (fields["name"] != "Alice" || fields["age"] != "31") {
				t.Fatal("vertex update lost a property")
			}
		}
	}
}

func TestFailureHidesResponseBody(t *testing.T) {
	for _, err := range []error{
		&sls.Error{HTTPCode: 403, Code: "Forbidden", Message: "secret-do-not-print", RequestID: "request-id"},
		&sls.BadResponseError{HTTPCode: 502, RespBody: "secret-do-not-print", RespHeader: map[string][]string{http.CanonicalHeaderKey(sls.RequestIDHeader): {"request-id"}}},
	} {
		message := failure(err)
		if strings.Contains(message, "secret-do-not-print") || !strings.Contains(message, "request-id") {
			t.Fatalf("unsafe/incomplete diagnostic: %s", message)
		}
	}
}

func TestInvalidSigningConfigFailsBeforeCreate(t *testing.T) {
	for _, name := range []string{"GRAPHSTORE_ENDPOINT", "GRAPHSTORE_PROJECT", "ALIBABA_CLOUD_ACCESS_KEY_ID", "ALIBABA_CLOUD_ACCESS_KEY_SECRET"} {
		t.Setenv(name, "example")
	}
	for _, tc := range []struct{ version, region, message string }{
		{"v2", "", "GRAPHSTORE_SIGNATURE_VERSION must be v1 or v4"},
		{"V4", "", "GRAPHSTORE_SIGNATURE_VERSION must be v1 or v4"},
		{"v4", "", "set GRAPHSTORE_REGION for V4 signing"},
		{"v4", " \t", "set GRAPHSTORE_REGION for V4 signing"},
	} {
		t.Run(tc.version+tc.region, func(t *testing.T) {
			t.Setenv("GRAPHSTORE_SIGNATURE_VERSION", tc.version)
			t.Setenv("GRAPHSTORE_REGION", tc.region)
			if err := runExample(); err == nil || err.Error() != tc.message {
				t.Fatalf("unexpected configuration result: %v", err)
			}
		})
	}
}
