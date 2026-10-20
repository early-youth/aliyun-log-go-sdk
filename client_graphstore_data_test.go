package sls

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aliyun/aliyun-log-go-sdk/internal/testutil"
	"github.com/gogo/protobuf/proto"
	"github.com/jarcoal/httpmock"
	"github.com/pierrec/lz4/v4"
	"github.com/stretchr/testify/require"
)

const graphStoreRows = `{"data":{"fields":["a","b","node"],"values":[[null,"","{\"id\":\"雪\"}"],["x\u0000y","雪","s"]]},"meta":{"truncated":true,"elapsedMs":-9223372036854775808,"warnings":["","雪"]}}`

func TestGraphStoreQueryStructuredValues(t *testing.T) {
	body := []byte(`{"data":{"fields":["v"],"values":[[9223372036854775807],[true],[[1,{"name":"雪","text":"[1,2]"}]],[{"labels":["person"],"properties":{"age":30}}],["123"],["true"],["[]"],[null],[""]]},"meta":{"truncated":false,"elapsedMs":1,"warnings":[]}}`)
	result, err := decodeGraphStoreQuery(body)
	require.NoError(t, err)
	require.Equal(t, json.Number("9223372036854775807"), result.Data.Values[0][0])
	require.Equal(t, true, result.Data.Values[1][0])
	require.Equal(t, []interface{}{json.Number("1"), map[string]interface{}{"name": "雪", "text": "[1,2]"}}, result.Data.Values[2][0])
	require.Equal(t, map[string]interface{}{"labels": []interface{}{"person"}, "properties": map[string]interface{}{"age": json.Number("30")}}, result.Data.Values[3][0])
	for i, value := range []interface{}{"123", "true", "[]", nil, ""} {
		require.Equal(t, value, result.Data.Values[i+4][0])
	}
	for _, version := range []AuthVersionType{AuthV1, AuthV4} {
		client := CreateNormalInterfaceV2("http://example.com", NewStaticCredentialsProvider("key", "secret", ""))
		client.SetAuthVersion(version)
		client.SetRegion("cn-test")
		transport := testutil.NewMockTransport()
		client.SetHTTPClient(&http.Client{Transport: transport})
		transport.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
			response := httpmock.NewStringResponse(200, string(body))
			response.Header.Set(RequestIDHeader, "structured-request")
			return response, nil
		})
		decoded, err := client.GetGraphStoreData("project", "store", NewGetGraphStoreDataRequest("RETURN 1"))
		require.NoError(t, err)
		require.Equal(t, result.Data.Values, decoded.Data.Values)
		require.Equal(t, "structured-request", decoded.RequestID)
	}
	for _, suffix := range []string{" null", " trailing", "\x00"} {
		_, err := decodeGraphStoreQuery(append(append([]byte(nil), body...), suffix...))
		require.Error(t, err)
	}
}

func TestGraphStoreDataWire(t *testing.T) {
	for _, version := range []AuthVersionType{AuthV1, AuthV4} {
		for _, token := range []string{"", "token"} {
			t.Run(string(version)+"/"+token, func(t *testing.T) {
				client := CreateNormalInterfaceV2("http://example.com", NewStaticCredentialsProvider("key", "secret", token))
				client.SetAuthVersion(version)
				client.SetRegion("cn-test")
				transport := testutil.NewMockTransport()
				client.SetHTTPClient(&http.Client{Transport: transport})
				var captured *http.Request
				var sent []byte
				transport.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
					captured = req
					var err error
					sent, err = io.ReadAll(req.Body)
					require.NoError(t, err)
					response := httpmock.NewStringResponse(200, graphStoreRows)
					response.Header.Set(RequestIDHeader, "data-request")
					return response, nil
				})
				check := func(path, contentType string, rawSize int) {
					t.Helper()
					require.Equal(t, http.MethodPost, captured.Method)
					require.Equal(t, "/graphstore/store/"+path, captured.URL.Path)
					require.Empty(t, captured.URL.RawQuery)
					require.Equal(t, "project.example.com", captured.URL.Host)
					require.Equal(t, contentType, captured.Header.Get("Content-Type"))
					require.Equal(t, strconv.Itoa(rawSize), captured.Header.Get(HTTPHeaderBodyRawSize))
					require.Equal(t, token, captured.Header.Get(HTTPHeaderAcsSecurityToken))
					if version == AuthV4 {
						hash := sha256.Sum256(sent)
						require.Equal(t, hex.EncodeToString(hash[:]), captured.Header.Get("x-log-content-sha256"))
						require.True(t, strings.HasPrefix(captured.Header.Get("Authorization"), "SLS4-HMAC-SHA256"))
					} else {
						hash := md5.Sum(sent)
						require.Equal(t, strings.ToUpper(hex.EncodeToString(hash[:])), captured.Header.Get("Content-MD5"))
						require.True(t, strings.HasPrefix(captured.Header.Get("Authorization"), "LOG key:"))
					}
				}
				groups := []*LogGroup{
					{Logs: []*Log{{Time: proto.Uint32(1)}}}, // Incompressible LZ4 block.
					{Logs: []*Log{{Time: proto.Uint32(1), Contents: []*LogContent{
						{Key: proto.String("\x00\xffkey"), Value: proto.String("\x00\xff" + strings.Repeat("雪", 256))},
					}}}},
				}
				for _, group := range groups {
					raw, err := proto.Marshal(group)
					require.NoError(t, err)
					for _, compression := range []int{Compress_None, Compress_LZ4, Compress_ZSTD} {
						require.NoError(t, client.PostGraphStoreData("project", "store", &PostGraphStoreDataRequest{LogGroup: group, CompressType: compression}))
						check("write", "application/x-protobuf", len(raw))
						decoded := sent
						switch compression {
						case Compress_LZ4:
							require.Equal(t, "lz4", captured.Header.Get("x-log-compresstype"))
							decoded = make([]byte, len(raw))
							n, err := lz4.UncompressBlock(sent, decoded)
							require.NoError(t, err)
							require.Equal(t, len(raw), n)
						case Compress_ZSTD:
							require.Equal(t, "zstd", captured.Header.Get("x-log-compresstype"))
							decoded, err = slsZstdCompressor.Decompress(sent, nil)
							require.NoError(t, err)
						default:
							require.Empty(t, captured.Header.Get("x-log-compresstype"))
						}
						require.Equal(t, raw, decoded)
					}
				}
				query := &GetGraphStoreDataRequest{Query: "RETURN '雪\\\"\n\x00'", Start: 0, End: 4294967295, Session: "opaque\x00session"}
				result, err := client.GetGraphStoreData("project", "store", query)
				require.NoError(t, err)
				check("query", "application/json", len(sent))
				require.Equal(t, "lz4", captured.Header.Get("Accept-Encoding"))
				var actual GetGraphStoreDataRequest
				require.NoError(t, json.Unmarshal(sent, &actual))
				require.Equal(t, *query, actual)
				require.Equal(t, "data-request", result.RequestID)
				require.Nil(t, result.Data.Values[0][0])
				require.Equal(t, "", result.Data.Values[0][1])
				require.Equal(t, `{"id":"雪"}`, result.Data.Values[0][2])
				require.Equal(t, "x\x00y", result.Data.Values[1][0])
				require.Equal(t, int64(-9223372036854775808), result.Meta.ElapsedMs)
				require.Equal(t, []string{"", "雪"}, result.Meta.Warnings)
				require.True(t, result.Meta.Truncated)
			})
		}
	}
}

func TestGraphStoreDataInput(t *testing.T) {
	client := CreateNormalInterface("example.com", "key", "secret", "")
	transport := testutil.NewMockTransport()
	client.SetHTTPClient(&http.Client{Transport: transport})
	for _, group := range []*LogGroup{nil, {}, {Logs: []*Log{}}} {
		for _, compression := range []int{Compress_LZ4, Compress_None, Compress_ZSTD} {
			require.NoError(t, client.PostGraphStoreData("", "", &PostGraphStoreDataRequest{LogGroup: group, CompressType: compression}))
		}
		for _, compression := range []int{-1, Compress_Max} {
			require.Equal(t, InvalidCompressError, client.PostGraphStoreData("project", "store", &PostGraphStoreDataRequest{LogGroup: group, CompressType: compression}))
		}
	}
	write := &PostGraphStoreDataRequest{LogGroup: &LogGroup{Logs: []*Log{{Time: proto.Uint32(1)}}}}
	require.Error(t, client.PostGraphStoreData("project", "store", nil))
	for _, names := range [][2]string{{"", "store"}, {"project", ""}, {"project", "a/b"}, {"project", "a?b"}} {
		require.Error(t, client.PostGraphStoreData(names[0], names[1], write))
		result, err := client.GetGraphStoreData(names[0], names[1], &GetGraphStoreDataRequest{Query: "RETURN 1", Start: 10, End: 20})
		require.Error(t, err)
		require.Nil(t, result)
	}
	for _, query := range []*GetGraphStoreDataRequest{nil, {}, {Query: "q", Start: -1, End: 1}, {Query: "q", Start: 2, End: 1}} {
		result, err := client.GetGraphStoreData("project", "store", query)
		require.Error(t, err)
		require.Nil(t, result)
	}
	write.LogGroup.Logs[0].Time = nil
	require.Error(t, client.PostGraphStoreData("project", "store", write))
	require.Zero(t, transport.GetTotalCallCount())
}

func TestGraphStoreClosedInt64TimeRange(t *testing.T) {
	client := CreateNormalInterface("example.com", "key", "secret", "")
	transport := testutil.NewMockTransport()
	client.SetHTTPClient(&http.Client{Transport: transport})
	for _, query := range []*GetGraphStoreDataRequest{
		NewGetGraphStoreDataRequest("RETURN 1"),
		{Query: "RETURN 1", Start: 0, End: 0},
		{Query: "RETURN 1", Start: 4294967296, End: 9223372036854775807},
	} {
		transport.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			var actual map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &actual))
			require.NotContains(t, actual, "from")
			require.NotContains(t, actual, "to")
			require.Equal(t, strconv.FormatInt(query.Start, 10), string(actual["start"]))
			require.Equal(t, strconv.FormatInt(query.End, 10), string(actual["end"]))
			require.NotContains(t, actual, "session")
			return httpmock.NewStringResponse(200, graphStoreRows), nil
		})
		_, err := client.GetGraphStoreData("project", "store", query)
		require.NoError(t, err)
	}
}

func TestGraphStoreQueryResponses(t *testing.T) {
	client := CreateNormalInterface("example.com", "key", "secret", "")
	transport := testutil.NewMockTransport()
	client.SetHTTPClient(&http.Client{Transport: transport})
	body := graphStoreRows
	compression, rawSize := "", ""
	transport.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
		response := httpmock.NewStringResponse(200, body)
		response.Header.Set("x-log-compresstype", compression)
		response.Header.Set(HTTPHeaderBodyRawSize, rawSize)
		response.Header.Set(RequestIDHeader, "query-request")
		return response, nil
	})
	query := func(valid bool) {
		t.Helper()
		before := transport.GetTotalCallCount()
		result, err := client.GetGraphStoreData("project", "store", &GetGraphStoreDataRequest{Query: "RETURN 1", Start: 10, End: 20})
		if valid {
			require.NoError(t, err)
			require.NotNil(t, result)
		} else {
			var badResponse *BadResponseError
			require.ErrorAs(t, err, &badResponse, body)
			require.Equal(t, 200, badResponse.HTTPCode)
			require.Equal(t, "query-request", http.Header(badResponse.RespHeader).Get(RequestIDHeader))
			require.Nil(t, result)
		}
		require.Equal(t, before+1, transport.GetTotalCallCount())
	}
	query(true)
	for _, body = range []string{"", "null", "[]", "{}", graphStoreRows + "{}", graphStoreRows + "\x00"} {
		query(false)
	}
	for _, replacement := range [][2]string{
		{`"fields":["a","b","node"]`, `"fields":[1,"b","node"]`},
		{`"truncated":true`, `"truncated":1`},
		{`"elapsedMs":-9223372036854775808`, `"elapsedMs":-9223372036854775809`},
		{`"elapsedMs":-9223372036854775808`, `"elapsedMs":1.5`},
		{`"elapsedMs":-9223372036854775808`, `"elapsedMs":"1"`},
		{`"warnings":["","雪"]`, `"warnings":[false]`},
	} {
		body = strings.Replace(graphStoreRows, replacement[0], replacement[1], 1)
		require.NotEqual(t, graphStoreRows, body)
		query(false)
	}
	for _, replacement := range [][2]string{
		{`"fields":["a","b","node"]`, `"fields":[null,"b","node"]`},
		{`[null,"","{\"id\":\"雪\"}"]`, `null`},
		{`[null,"","{\"id\":\"雪\"}"]`, `[]`},
		{`"truncated":true`, `"rowCount":2,"truncated":true`},
		{`"warnings":["","雪"]`, `"warnings":[null]`},
		{`"data":{`, `"diagnostic":{},"data":{"extra":1,`},
	} {
		body = strings.Replace(graphStoreRows, replacement[0], replacement[1], 1)
		require.NotEqual(t, graphStoreRows, body)
		query(true)
	}
	body = strings.Replace(graphStoreRows, "雪", "\xff", 1)
	query(true) // encoding/json replaces invalid UTF-8, as it does for LogStore.
	for _, scope := range []string{"", "meta", "data"} {
		var outer map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(graphStoreRows), &outer))
		fields := outer
		if scope != "" {
			fields = nil
			require.NoError(t, json.Unmarshal(outer[scope], &fields))
		}
		for key, original := range fields {
			for _, null := range []bool{true, false} {
				if null {
					fields[key] = json.RawMessage("null")
				} else {
					delete(fields, key)
				}
				if scope != "" {
					outer[scope], _ = json.Marshal(fields)
				}
				encoded, err := json.Marshal(outer)
				require.NoError(t, err)
				body = string(encoded)
				query(false)
			}
			fields[key] = original
		}
	}
	body = `{"data":{"fields":[],"values":[]},"meta":{"truncated":false,"elapsedMs":0,"warnings":[]}}`
	query(true)
	body = strings.Replace(body, `"values":[]`, `"values":[null]`, 1)
	query(true)
	compressed := make([]byte, lz4.CompressBlockBound(len(graphStoreRows)))
	n, err := lz4.CompressBlock([]byte(graphStoreRows), compressed, nil)
	require.NoError(t, err)
	if n == 0 {
		n, _ = copyIncompressible([]byte(graphStoreRows), compressed)
	}
	body, compression, rawSize = string(compressed[:n]), "lz4", strconv.Itoa(len(graphStoreRows))
	query(true)
	for _, body = range []string{string(compressed[:n-1]), string(compressed[:n]) + "\x00"} {
		query(false)
	}
	body = string(compressed[:n])
	for _, rawSize = range []string{"", "-1", "x", "9223372036854775808", "0", strconv.Itoa(len(graphStoreRows) + 1)} {
		query(false)
	}
	body, rawSize = "not-lz4", strconv.Itoa(len(graphStoreRows))
	query(false)
	for _, compression = range []string{"none", "zstd", "deflate", "unknown"} {
		query(false)
	}
}

func TestGraphStoreDataRetriesAndToken(t *testing.T) {
	oldRetry := RetryOnServerErrorEnabled
	defer func() { RetryOnServerErrorEnabled = oldRetry }()
	for _, operation := range []string{"write", "query"} {
		t.Run(operation, func(t *testing.T) {
			client := CreateNormalInterface("example.com", "key", "secret", "old-token")
			client.SetRetryTimeout(2 * time.Second)
			transport := testutil.NewMockTransport()
			client.SetHTTPClient(&http.Client{Transport: transport})
			invoke := func(c ClientInterface) error {
				if operation == "write" {
					return c.PostGraphStoreData("project", "store", &PostGraphStoreDataRequest{LogGroup: &LogGroup{Logs: []*Log{{Time: proto.Uint32(1)}}}})
				}
				_, err := c.GetGraphStoreData("project", "store", &GetGraphStoreDataRequest{Query: "RETURN 1", Start: 10, End: 20})
				return err
			}
			calls, status, failCount := 0, 500, 1
			transport.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls > failCount {
					return httpmock.NewStringResponse(200, graphStoreRows), nil
				}
				response := httpmock.NewStringResponse(status, `{"errorCode":"ServiceError","errorMessage":"failed"}`)
				response.Header.Set(RequestIDHeader, "last-request")
				return response, nil
			})
			RetryOnServerErrorEnabled = true
			for _, status = range []int{500, 502, 503} {
				calls = 0
				require.NoError(t, invoke(client))
				require.Equal(t, 2, calls)
			}
			for _, status = range []int{400, 401, 403, 404, 409, 413, 501, 504} {
				calls = 0
				var serviceError *Error
				require.ErrorAs(t, invoke(client), &serviceError)
				require.Equal(t, int32(status), serviceError.HTTPCode)
				require.Equal(t, "last-request", serviceError.RequestID)
				require.Equal(t, 1, calls)
			}
			RetryOnServerErrorEnabled = false
			calls, status = 0, 503
			require.Error(t, invoke(client))
			require.Equal(t, 1, calls)
			RetryOnServerErrorEnabled = true
			client.SetRetryTimeout(10 * time.Millisecond)
			calls, failCount = 0, 100
			require.ErrorContains(t, invoke(client), "stopped retrying")
			require.Equal(t, 1, calls)
			refreshes := 0
			token := &TokenAutoUpdateClient{logClient: client, maxTryTimes: 3, tokenUpdateFunc: func() (string, string, string, time.Time, error) {
				refreshes++
				return "key", "secret", "new-token", time.Now().Add(time.Hour), nil
			}}
			transport.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
				if req.Header.Get(HTTPHeaderAcsSecurityToken) == "old-token" {
					return httpmock.NewStringResponse(401, `{"errorCode":"Unauthorized","errorMessage":"expired"}`), nil
				}
				require.Equal(t, "new-token", req.Header.Get(HTTPHeaderAcsSecurityToken))
				return httpmock.NewStringResponse(200, graphStoreRows), nil
			})
			require.NoError(t, invoke(token))
			require.Equal(t, 1, refreshes)
			calls = 0
			transport.RegisterNoResponder(func(req *http.Request) (*http.Response, error) { calls++; return nil, errors.New("connection lost") })
			require.Error(t, invoke(client))
			require.Equal(t, 1, calls)
			transport.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
				time.Sleep(20 * time.Millisecond)
				return httpmock.NewStringResponse(200, graphStoreRows), nil
			})
			require.NoError(t, invoke(client)) // Native retryTimeout does not cancel an in-flight HTTP request.
		})
	}
}
