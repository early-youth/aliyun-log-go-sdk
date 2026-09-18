package sls

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// CreateGraphStoreRequest supplies logical schema JSON; the service owns schema validation.
type CreateGraphStoreRequest struct {
	StoreName         string          `json:"storeName"`
	InitialShardCount int32           `json:"initialShardCount"`
	BaseSchema        json.RawMessage `json:"baseSchema"`
}

type ListGraphStoresRequest struct {
	Offset *int
	Size   *int
}

type GraphStore struct {
	ProjectID         int32  `json:"projectId"`
	StoreName         string `json:"storeName"`
	State             string `json:"state"`
	CurrentShardCount int32  `json:"currentShardCount"`
}

type ListGraphStoresResponse struct {
	Stores []GraphStore `json:"stores"`
	Count  uint64       `json:"count"`
	Total  int64        `json:"total"`
}

type GetGraphStoreBaseSchemaResponse struct {
	ProjectID  int32           `json:"projectId"`
	StoreName  string          `json:"storeName"`
	StoreID    int64           `json:"storeId"`
	BaseSchema json.RawMessage `json:"baseSchema"`
}

type GetGraphStoreCollectionSchemaResponse struct {
	ProjectID        int32           `json:"projectId"`
	StoreName        string          `json:"storeName"`
	CollectionSchema json.RawMessage `json:"collectionSchema"`
}

type ListGraphStoreCollectionSchemasResponse struct {
	ProjectID         int32             `json:"projectId"`
	StoreName         string            `json:"storeName"`
	CollectionSchemas []json.RawMessage `json:"collectionSchemas"`
	NextPageToken     string            `json:"nextPageToken,omitempty"`
}

// Decode required wire fields without coercing null, floats or overflowing integers.
// Unknown fields stay forward-compatible; schema internals remain raw JSON.
func decodeGraphStoreFields(data []byte, fields map[string]interface{}) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("invalid UTF-8 in GraphStore JSON")
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, fmt.Errorf("GraphStore response must be a JSON object")
	}
	for name, target := range fields {
		raw, ok := object[name]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, fmt.Errorf("missing or null GraphStore field %s", name)
		}
		if err := json.Unmarshal(raw, target); err != nil {
			return nil, err
		}
	}
	return object, nil
}

func (s *GraphStore) UnmarshalJSON(data []byte) error {
	_, err := decodeGraphStoreFields(data, map[string]interface{}{
		"projectId": &s.ProjectID, "storeName": &s.StoreName,
		"state": &s.State, "currentShardCount": &s.CurrentShardCount,
	})
	return err
}

func (r *ListGraphStoresResponse) UnmarshalJSON(data []byte) error {
	_, err := decodeGraphStoreFields(data, map[string]interface{}{
		"stores": &r.Stores, "count": &r.Count, "total": &r.Total,
	})
	return err
}

func graphStoreSchemaObject(schema json.RawMessage) error {
	value := bytes.TrimSpace(schema)
	if len(value) == 0 || value[0] != '{' || !utf8.Valid(value) || !json.Valid(value) {
		return fmt.Errorf("GraphStore schema must be a JSON object")
	}
	return nil
}

func (r *GetGraphStoreBaseSchemaResponse) UnmarshalJSON(data []byte) error {
	_, err := decodeGraphStoreFields(data, map[string]interface{}{
		"projectId": &r.ProjectID, "storeName": &r.StoreName,
		"storeId": &r.StoreID, "baseSchema": &r.BaseSchema,
	})
	if err != nil {
		return err
	}
	if r.BaseSchema[0] != '{' {
		return fmt.Errorf("GraphStore schema must be a JSON object")
	}
	return nil
}

func checkGraphStoreCollection(schema json.RawMessage) error {
	var id int64
	_, err := decodeGraphStoreFields(schema, map[string]interface{}{"collectionSchemaId": &id})
	return err
}

func (r *GetGraphStoreCollectionSchemaResponse) UnmarshalJSON(data []byte) error {
	_, err := decodeGraphStoreFields(data, map[string]interface{}{
		"projectId": &r.ProjectID, "storeName": &r.StoreName, "collectionSchema": &r.CollectionSchema,
	})
	if err != nil {
		return err
	}
	return checkGraphStoreCollection(r.CollectionSchema)
}

func (r *ListGraphStoreCollectionSchemasResponse) UnmarshalJSON(data []byte) error {
	object, err := decodeGraphStoreFields(data, map[string]interface{}{
		"projectId": &r.ProjectID, "storeName": &r.StoreName, "collectionSchemas": &r.CollectionSchemas,
	})
	if err != nil {
		return err
	}
	r.NextPageToken = ""
	if raw, ok := object["nextPageToken"]; ok {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("null GraphStore nextPageToken")
		}
		if err := json.Unmarshal(raw, &r.NextPageToken); err != nil {
			return err
		}
	}
	for _, schema := range r.CollectionSchemas {
		if err := checkGraphStoreCollection(schema); err != nil {
			return err
		}
	}
	return nil
}

func graphStorePath(name string) (string, error) {
	if name == "" || strings.IndexFunc(name, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-')
	}) >= 0 {
		return "", fmt.Errorf("invalid GraphStore path segment")
	}
	return "/graphstore/" + name, nil
}

// Request assembly only: authentication, HTTP errors and timeout remain owned by c.request.
func (c *Client) graphStoreRequest(project, method, uri string, query url.Values, body interface{}, result interface{}) error {
	if project == "" {
		return fmt.Errorf("project is required")
	}
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	if len(query) != 0 {
		uri += "?" + query.Encode()
	}
	headers := map[string]string{
		HTTPHeaderBodyRawSize: strconv.Itoa(len(data)),
		HTTPHeaderContentType: "application/json",
	}
	response, err := c.request(project, method, uri, headers, data)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if result == nil {
		_, err = io.Copy(io.Discard, response.Body)
		return err
	}
	data, err = io.ReadAll(response.Body)
	if err != nil {
		return readResponseError(err)
	}
	return json.Unmarshal(data, result)
}

func (c *Client) CreateGraphStore(project string, req *CreateGraphStoreRequest) error {
	if req == nil {
		return fmt.Errorf("CreateGraphStore request is required")
	}
	if err := graphStoreSchemaObject(req.BaseSchema); err != nil {
		return err
	}
	return c.graphStoreRequest(project, http.MethodPost, "/graphstore", nil, req, nil)
}

func (c *Client) GetGraphStore(project, graphStore string) (*GraphStore, error) {
	uri, err := graphStorePath(graphStore)
	if err != nil {
		return nil, err
	}
	result := &GraphStore{}
	if err := c.graphStoreRequest(project, http.MethodGet, uri, nil, nil, result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListGraphStores returns one page; nil offsets and sizes are omitted.
func (c *Client) ListGraphStores(project string, req *ListGraphStoresRequest) (*ListGraphStoresResponse, error) {
	query := url.Values{}
	if req != nil {
		if req.Offset != nil {
			query.Set("offset", strconv.Itoa(*req.Offset))
		}
		if req.Size != nil {
			query.Set("size", strconv.Itoa(*req.Size))
		}
	}
	result := &ListGraphStoresResponse{}
	if err := c.graphStoreRequest(project, http.MethodGet, "/graphstore", query, nil, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) DeleteGraphStore(project, graphStore string) error {
	uri, err := graphStorePath(graphStore)
	if err != nil {
		return err
	}
	return c.graphStoreRequest(project, http.MethodDelete, uri, nil, nil, nil)
}

// SplitGraphStoreShard distinguishes nil (server-selected split) from an explicit zero hash.
func (c *Client) SplitGraphStoreShard(project, graphStore string, shardID int32, splitHash *uint64) error {
	uri, err := graphStorePath(graphStore)
	if err != nil {
		return err
	}
	var body interface{}
	if splitHash != nil {
		body = struct {
			SplitHash uint64 `json:"splitHash"`
		}{*splitHash}
	}
	return c.graphStoreRequest(project, http.MethodPost,
		uri+"/shards/"+strconv.FormatInt(int64(shardID), 10)+"/split", nil, body, nil)
}

func (c *Client) GetGraphStoreBaseSchema(project, graphStore string) (*GetGraphStoreBaseSchemaResponse, error) {
	uri, err := graphStorePath(graphStore)
	if err != nil {
		return nil, err
	}
	result := &GetGraphStoreBaseSchemaResponse{}
	if err := c.graphStoreRequest(project, http.MethodGet, uri+"/baseschema", nil, nil, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) CreateGraphStoreCollectionSchema(project, graphStore string, schema json.RawMessage) error {
	uri, err := graphStorePath(graphStore)
	if err != nil {
		return err
	}
	if err := graphStoreSchemaObject(schema); err != nil {
		return err
	}
	body := struct {
		Value json.RawMessage `json:"value"`
	}{schema}
	return c.graphStoreRequest(project, http.MethodPost, uri+"/collectionschemas", nil, body, nil)
}

func (c *Client) GetGraphStoreCollectionSchema(project, graphStore string, collectionSchemaID int64) (*GetGraphStoreCollectionSchemaResponse, error) {
	uri, err := graphStorePath(graphStore)
	if err != nil {
		return nil, err
	}
	result := &GetGraphStoreCollectionSchemaResponse{}
	if err := c.graphStoreRequest(project, http.MethodGet,
		uri+"/collectionschemas/"+strconv.FormatInt(collectionSchemaID, 10), nil, nil, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) ListGraphStoreCollectionSchemas(project, graphStore string, pageSize int, pageToken string) (*ListGraphStoreCollectionSchemasResponse, error) {
	uri, err := graphStorePath(graphStore)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	if pageSize != 0 {
		query.Set("pageSize", strconv.Itoa(pageSize))
	}
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	result := &ListGraphStoreCollectionSchemasResponse{}
	if err := c.graphStoreRequest(project, http.MethodGet, uri+"/collectionschemas", query, nil, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) UpdateGraphStoreCollectionSchema(project, graphStore string, collectionSchemaID int64, schema json.RawMessage) error {
	uri, err := graphStorePath(graphStore)
	if err != nil {
		return err
	}
	if err := graphStoreSchemaObject(schema); err != nil {
		return err
	}
	body := struct {
		Value json.RawMessage `json:"value"`
	}{schema}
	return c.graphStoreRequest(project, http.MethodPut,
		uri+"/collectionschemas/"+strconv.FormatInt(collectionSchemaID, 10), nil, body, nil)
}

func (c *Client) DeleteGraphStoreCollectionSchema(project, graphStore string, collectionSchemaID int64) error {
	uri, err := graphStorePath(graphStore)
	if err != nil {
		return err
	}
	return c.graphStoreRequest(project, http.MethodDelete,
		uri+"/collectionschemas/"+strconv.FormatInt(collectionSchemaID, 10), nil, nil, nil)
}
