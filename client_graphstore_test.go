package sls

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aliyun/aliyun-log-go-sdk/internal/testutil"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/require"
)

func TestGraphStoreManagementRequests(t *testing.T) {
	for _, mode := range []string{"v1", "v4", "token"} {
		t.Run(mode, func(t *testing.T) {
			var client ClientInterface = CreateNormalInterfaceV2("example.com",
				NewUpdateFuncProviderAdapter(func() (string, string, string, time.Time, error) {
					return "key", "secret", "token", time.Now().Add(time.Hour), nil
				}))
			if mode == "v4" {
				client.SetAuthVersion(AuthV4)
				client.SetRegion("cn-test")
			}
			if mode == "token" {
				client = &TokenAutoUpdateClient{logClient: client, maxTryTimes: 3}
			}
			transport := testutil.NewMockTransport()
			client.SetHTTPClient(&http.Client{Transport: transport})
			var captured *http.Request
			var requestBody []byte
			responseBody := ""
			transport.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
				captured = req
				var err error
				requestBody, err = io.ReadAll(req.Body)
				require.NoError(t, err)
				response := httpmock.NewStringResponse(200, responseBody)
				response.Header.Set("x-log-requestid", "test-request")
				return response, nil
			})
			check := func(method, path string) {
				t.Helper()
				require.Equal(t, method, captured.Method)
				require.Equal(t, path, captured.URL.Path)
				require.Equal(t, "project.example.com", captured.Host)
				require.Equal(t, "0.6.0", captured.Header.Get("x-log-apiversion"))
				require.Equal(t, "application/json", captured.Header.Get("Content-Type"))
				require.Equal(t, strconv.Itoa(len(requestBody)), captured.Header.Get("x-log-bodyrawsize"))
				require.Equal(t, "token", captured.Header.Get("x-acs-security-token"))
				if mode == "v4" {
					require.True(t, strings.HasPrefix(captured.Header.Get("Authorization"), "SLS4-HMAC-SHA256"))
				} else {
					require.True(t, strings.HasPrefix(captured.Header.Get("Authorization"), "LOG key:"))
				}
			}
			require.NoError(t, client.CreateGraphStore("project", &CreateGraphStoreRequest{
				StoreName: "store", InitialShardCount: 2,
				BaseSchema: json.RawMessage(`{"unknown":"雪","id":9223372036854775807}`),
			}))
			check(http.MethodPost, "/graphstore")
			var body map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(requestBody, &body))
			require.Len(t, body, 3)
			require.Contains(t, string(body["baseSchema"]), "9223372036854775807")
			require.Equal(t, byte('{'), body["baseSchema"][0])
			if mode == "v4" {
				require.NotEmpty(t, captured.Header.Get("x-log-content-sha256"))
			} else {
				require.NotEmpty(t, captured.Header.Get("Content-MD5"))
			}
			maxHash := uint64(math.MaxUint64)
			for _, hash := range []*uint64{nil, new(uint64), &maxHash} {
				require.NoError(t, client.SplitGraphStoreShard("project", "store", 3, hash))
				check(http.MethodPost, "/graphstore/store/shards/3/split")
				if hash == nil {
					require.Empty(t, requestBody)
				} else {
					require.Equal(t, fmt.Sprintf(`{"splitHash":%d}`, *hash), string(requestBody))
				}
			}
			require.NoError(t, client.CreateGraphStoreCollectionSchema("project", "store", json.RawMessage(`{"name":"雪"}`)))
			check(http.MethodPost, "/graphstore/store/collectionschemas")
			require.JSONEq(t, `{"value":{"name":"雪"}}`, string(requestBody))
			require.NoError(t, client.UpdateGraphStoreCollectionSchema("project", "store", math.MaxInt64, json.RawMessage(`{"fields":[]}`)))
			check(http.MethodPut, "/graphstore/store/collectionschemas/9223372036854775807")
			require.JSONEq(t, `{"value":{"fields":[]}}`, string(requestBody))
			require.NoError(t, client.DeleteGraphStoreCollectionSchema("project", "store", math.MaxInt64))
			check(http.MethodDelete, "/graphstore/store/collectionschemas/9223372036854775807")
			require.NoError(t, client.DeleteGraphStore("project", "store"))
			check(http.MethodDelete, "/graphstore/store")

			responseBody = `{"projectId":2147483647,"storeName":"store","state":"READY","currentShardCount":2}`
			store, err := client.GetGraphStore("project", "store")
			require.NoError(t, err)
			check(http.MethodGet, "/graphstore/store")
			require.Equal(t, int32(math.MaxInt32), store.ProjectID)
			require.Equal(t, int32(2), store.CurrentShardCount)
			require.Equal(t, "READY", store.State)
			responseBody = `{"stores":[` + responseBody + `],"count":1,"total":9223372036854775807}`
			offset, size := 0, 1
			stores, err := client.ListGraphStores("project", &ListGraphStoresRequest{Offset: &offset, Size: &size})
			require.NoError(t, err)
			check(http.MethodGet, "/graphstore")
			require.Equal(t, url.Values{"offset": {"0"}, "size": {"1"}}, captured.URL.Query())
			require.Len(t, stores.Stores, 1)
			require.Equal(t, uint64(1), stores.Count)
			require.Equal(t, int64(math.MaxInt64), stores.Total)
			_, err = client.ListGraphStores("project", nil)
			require.NoError(t, err)
			require.Empty(t, captured.URL.RawQuery)
			responseBody = `{"projectId":1,"storeName":"store","storeId":9223372036854775807,"baseSchema":{"id":9223372036854775807}}`
			base, err := client.GetGraphStoreBaseSchema("project", "store")
			require.NoError(t, err)
			check(http.MethodGet, "/graphstore/store/baseschema")
			require.Equal(t, int64(math.MaxInt64), base.StoreID)
			require.Contains(t, string(base.BaseSchema), "9223372036854775807")
			schema := `{"collectionSchemaId":9223372036854775807,"unknown":"opaque"}`
			responseBody = `{"projectId":1,"storeName":"store","collectionSchema":` + schema + "}"
			collection, err := client.GetGraphStoreCollectionSchema("project", "store", math.MaxInt64)
			require.NoError(t, err)
			check(http.MethodGet, "/graphstore/store/collectionschemas/9223372036854775807")
			require.Equal(t, schema, string(collection.CollectionSchema))
			responseBody = `{"projectId":1,"storeName":"store","collectionSchemas":[` + schema + `],"nextPageToken":"a+b/=&"}`
			page, err := client.ListGraphStoreCollectionSchemas("project", "store", 1, "a+b/=&")
			require.NoError(t, err)
			check(http.MethodGet, "/graphstore/store/collectionschemas")
			require.Equal(t, url.Values{"pageSize": {"1"}, "pageToken": {"a+b/=&"}}, captured.URL.Query())
			require.Equal(t, "a+b/=&", page.NextPageToken)
			require.Equal(t, schema, string(page.CollectionSchemas[0]))
			responseBody = `{"projectId":1,"storeName":"store","collectionSchemas":[]}`
			page, err = client.ListGraphStoreCollectionSchemas("project", "store", 0, "")
			require.NoError(t, err)
			require.Empty(t, captured.URL.RawQuery)
			require.Empty(t, page.NextPageToken)
		})
	}
}

func TestGraphStoreInputAndResponseValidation(t *testing.T) {
	client := CreateNormalInterface("example.com", "key", "secret", "")
	transport := testutil.NewMockTransport()
	client.SetHTTPClient(&http.Client{Transport: transport})
	for _, schema := range []string{"", "null", "[]", `"{}"`, "{} {}", "{}\x00tail", "{bad}", "{\"x\":\"\xff\"}"} {
		require.Error(t, client.CreateGraphStore("project", &CreateGraphStoreRequest{BaseSchema: json.RawMessage(schema)}))
		require.Error(t, client.CreateGraphStoreCollectionSchema("project", "store", json.RawMessage(schema)))
		require.Error(t, client.UpdateGraphStoreCollectionSchema("project", "store", 1, json.RawMessage(schema)))
	}
	require.Error(t, client.CreateGraphStore("project", nil))
	require.Error(t, client.DeleteGraphStore("project", "../logstores/store"))
	_, err := client.GetGraphStore("", "store")
	require.Error(t, err)
	_, err = client.GetGraphStore("project", "")
	require.Error(t, err)
	require.Equal(t, 0, transport.GetTotalCallCount())
	for _, response := range []interface{}{
		&GraphStore{}, &ListGraphStoresResponse{}, &GetGraphStoreBaseSchemaResponse{},
		&GetGraphStoreCollectionSchemaResponse{}, &ListGraphStoreCollectionSchemasResponse{},
	} {
		for _, body := range []string{"{}", "null", "[]", "{} {}", "{}\x00tail"} {
			require.Error(t, json.Unmarshal([]byte(body), response))
		}
	}
	for _, id := range []string{"null", "1.5", `"42"`, "9223372036854775808"} {
		require.Error(t, json.Unmarshal([]byte(`{"projectId":1,"storeName":"s","storeId":`+id+`,"baseSchema":{}}`), &GetGraphStoreBaseSchemaResponse{}))
		schema := `{"collectionSchemaId":` + id + "}"
		require.Error(t, json.Unmarshal([]byte(`{"projectId":1,"storeName":"s","collectionSchema":`+schema+"}"), &GetGraphStoreCollectionSchemaResponse{}))
		require.Error(t, json.Unmarshal([]byte(`{"projectId":1,"storeName":"s","collectionSchemas":[`+schema+"]}"), &ListGraphStoreCollectionSchemasResponse{}))
	}
	require.Error(t, json.Unmarshal([]byte(`{"stores":[{}],"count":1,"total":1}`), &ListGraphStoresResponse{}))
	require.Error(t, json.Unmarshal([]byte(`{"stores":[],"count":-1,"total":0}`), &ListGraphStoresResponse{}))
	for _, schema := range []string{"null", "[]", `"{}"`, "1", "true", "{bad}", "{\"x\":\"\xff\"}"} {
		require.Error(t, json.Unmarshal([]byte(`{"projectId":1,"storeName":"s","storeId":1,"baseSchema":`+schema+`}`), &GetGraphStoreBaseSchemaResponse{}))
	}
	var base GetGraphStoreBaseSchemaResponse
	require.NoError(t, json.Unmarshal([]byte("{\"projectId\":1,\"storeName\":\"s\",\"storeId\":1,\"baseSchema\": \n {\"id\":9223372036854775807} \t }"), &base))
	require.Equal(t, `{"id":9223372036854775807}`, string(base.BaseSchema))
	require.Error(t, json.Unmarshal([]byte(`{"projectId":1,"storeName":"s","collectionSchemas":[],"nextPageToken":null}`), &ListGraphStoreCollectionSchemasResponse{}))
	require.Error(t, json.Unmarshal([]byte(`{"projectId":1.0,"storeName":"s","state":"READY","currentShardCount":1}`), &GraphStore{}))
	require.Error(t, json.Unmarshal([]byte(`{"projectId":2147483648,"storeName":"s","state":"READY","currentShardCount":1}`), &GraphStore{}))
}

func TestGraphStoreErrorsAndTokenRefresh(t *testing.T) {
	normal := CreateNormalInterface("example.com", "key", "secret", "old-token")
	refreshes := 0
	token := &TokenAutoUpdateClient{
		logClient: normal, maxTryTimes: 3,
		tokenUpdateFunc: func() (string, string, string, time.Time, error) {
			refreshes++
			return "key", "secret", "new-token", time.Now().Add(time.Hour), nil
		},
	}
	transport := testutil.NewMockTransport()
	normal.SetHTTPClient(&http.Client{Transport: transport})
	calls := 0
	status := 400
	transport.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
		calls++
		response := httpmock.NewStringResponse(status, `{"errorCode":"ServiceError","errorMessage":"failed"}`)
		response.Header.Set("x-log-requestid", "error-request")
		return response, nil
	})
	for _, client := range []ClientInterface{normal, token} {
		for _, status = range []int{400, 403, 404, 409, 413, 500, 503} {
			before := calls
			err := client.DeleteGraphStore("project", "store")
			var serviceError *Error
			require.ErrorAs(t, err, &serviceError)
			require.Equal(t, int32(status), serviceError.HTTPCode)
			require.Equal(t, "error-request", serviceError.RequestID)
			require.Equal(t, before+1, calls)
		}
	}
	require.Zero(t, refreshes)
	transport.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("x-acs-security-token") == "old-token" {
			return httpmock.NewStringResponse(401, `{"errorCode":"Unauthorized","errorMessage":"expired"}`), nil
		}
		require.Equal(t, "new-token", req.Header.Get("x-acs-security-token"))
		return httpmock.NewStringResponse(200, ""), nil
	})
	require.NoError(t, token.DeleteGraphStore("project", "store"))
	require.Equal(t, 1, refreshes)
	transport.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("connection lost")
	})
	before := calls
	require.Error(t, token.CreateGraphStore("project", &CreateGraphStoreRequest{StoreName: "store", BaseSchema: json.RawMessage("{}")}))
	require.Equal(t, before+1, calls)
}
