package sls

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGraphStoreTypedSchemaWireAndCompatibility(t *testing.T) {
	base := graphStoreTestBase()
	base.VertexLayout.CK = []string{}
	base.VertexLayout.KeyIndexConfigs = map[string]GraphStoreIndexConfig{}
	base.EdgeLayout.RouteKey = []string{"sl"}
	base.FullTextIndex = &GraphStoreFullTextIndex{Token: " ,", CaseSensitive: true}
	encoded := graphStoreTestJSON(t, base)
	require.Contains(t, encoded, `"graphLayouts"`)
	require.NotContains(t, encoded, `"VertexLayout"`)
	require.Contains(t, encoded, `"ck":[]`)
	require.Contains(t, encoded, `"keyIndexConfigs":{}`)
	require.NotContains(t, encoded, `"pk"`)
	var read GraphStoreBaseSchema
	require.NoError(t, json.Unmarshal([]byte(encoded), &read))
	base.CommonFieldNames = []string{}
	require.Equal(t, base, read)
	for _, replacement := range [][2]string{
		{`"ttlSeconds":9223372036854775807`, `"ttlSeconds":9223372036854775808`},
		{`"ttlSeconds":9223372036854775807`, `"ttlSeconds":1.5`},
		{`"ttlSeconds":9223372036854775807`, `"ttlSeconds":null`},
		{`"timeStateEnable":false`, `"timeStateEnable":"false"`},
		{`"name":"vertex"`, `"name":"other"`},
		{`"name":"vertex"`, `"source":null,"name":"vertex"`},
		{`"name":"edge"`, `"id":null,"name":"edge"`},
		{`"chn":false`, `"chn":false,"newOption":1`},
		{`"graphLayouts":`, `"newField":true,"graphLayouts":`},
	} {
		bad := strings.Replace(encoded, replacement[0], replacement[1], 1)
		require.NotEqual(t, encoded, bad)
		require.Error(t, json.Unmarshal([]byte(bad), &GraphStoreBaseSchema{}), bad)
	}
	ttl := int64(math.MaxInt64)
	collection := graphStoreTestCollection("雪\x00")
	collection.TTLSeconds = &ttl
	collection.CollectionKeyValues = []GraphStoreCollectionKeyValue{{Name: "vlabel", Value: ""}}
	collection.Fields = []GraphStoreFieldSchema{
		{Name: "exact", Type: GraphStoreFieldInt, IndexConfig: GraphStoreIndexConfig{Enable: true}},
		{Name: "full", Type: GraphStoreFieldString, IndexConfig: GraphStoreIndexConfig{Enable: true, FullTextIndex: base.FullTextIndex}},
		{Name: "range", Type: GraphStoreFieldDouble, IndexConfig: GraphStoreIndexConfig{Enable: true, RangeIndex: &GraphStoreRangeIndex{}}},
		{Name: "bool", Type: GraphStoreFieldBool},
	}
	encoded = graphStoreTestJSON(t, collection)
	var decoded GraphStoreCollectionSchema
	require.NoError(t, json.Unmarshal([]byte(encoded), &decoded))
	require.Equal(t, *collection, decoded)
	for _, replacement := range [][2]string{
		{`"type":"INT"`, `"type":"UNKNOWN"`},
		{`"graphLayout":"vertex"`, `"graphLayout":"unknown"`},
		{`"rangeIndex":{}`, `"rangeIndex":{"unsupported":true}`},
		{`"enable":true`, `"enable":null`},
		{`"fields":`, `"unsupported":1,"fields":`},
	} {
		bad := strings.Replace(encoded, replacement[0], replacement[1], 1)
		require.NotEqual(t, encoded, bad)
		require.Error(t, json.Unmarshal([]byte(bad), &GraphStoreCollectionSchema{}), bad)
	}
	for _, ttl := range []*int64{nil, new(int64), &ttl} {
		update := graphStoreTestUpdate("collection")
		update.TTLSeconds = ttl
		body := graphStoreTestJSON(t, update)
		require.NotContains(t, body, "graphLayout")
		require.NotContains(t, body, "collectionKeyValues")
		var decoded GraphStoreCollectionSchemaUpdate
		require.NoError(t, json.Unmarshal([]byte(body), &decoded))
		require.Equal(t, *update, decoded)
	}
	var update GraphStoreCollectionSchemaUpdate
	require.NoError(t, json.Unmarshal([]byte(`{"name":"c","description":"schema","ttlSeconds":null,"fields":[]}`), &update))
	require.Nil(t, update.TTLSeconds)
	negative := int64(-1)
	collection.TTLSeconds = &negative
	require.Error(t, collection.validate())
}
