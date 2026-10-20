package sls

import (
	"encoding/json"
)

// GraphStoreLayoutType selects a logical vertex or edge collection.
type GraphStoreLayoutType string

const (
	GraphStoreLayoutVertex GraphStoreLayoutType = "vertex"
	GraphStoreLayoutEdge   GraphStoreLayoutType = "edge"
)

// GraphStoreFieldType is the property type; INT is a signed 64-bit integer.
type GraphStoreFieldType string

const (
	GraphStoreFieldString GraphStoreFieldType = "STRING"
	GraphStoreFieldInt    GraphStoreFieldType = "INT"
	GraphStoreFieldDouble GraphStoreFieldType = "DOUBLE"
	GraphStoreFieldBool   GraphStoreFieldType = "BOOL"
)

// GraphStoreFullTextIndex describes logical schema configuration, without physical storage metadata.
type GraphStoreFullTextIndex struct {
	// Required nonempty token characters.
	Token string `json:"token"`
	// Optional nonempty subset of token characters; nil omits the field.
	OptionToken *string `json:"optionToken,omitempty"`
	// Required case sensitivity setting.
	CaseSensitive bool `json:"caseSensitive"`
	// Required Chinese tokenization setting.
	Chn bool `json:"chn"`
}

// GraphStoreRangeIndex describes logical schema configuration, without physical storage metadata.
type GraphStoreRangeIndex struct {
}

// GraphStoreIndexConfig describes logical schema configuration, without physical storage metadata.
type GraphStoreIndexConfig struct {
	// Required; true without a specialized index selects Exact.
	Enable bool `json:"enable"`
	// Optional FullText configuration.
	FullTextIndex *GraphStoreFullTextIndex `json:"fullTextIndex,omitempty"`
	// Optional Range configuration; the service validates index combinations.
	RangeIndex *GraphStoreRangeIndex `json:"rangeIndex,omitempty"`
}

// GraphStoreFieldSchema describes logical schema configuration, without physical storage metadata.
type GraphStoreFieldSchema struct {
	// Required nonempty property field name.
	Name string `json:"name"`
	// Required property type; INT is signed int64.
	Type GraphStoreFieldType `json:"type"`
	// Required field index configuration.
	IndexConfig GraphStoreIndexConfig `json:"indexConfig"`
}

// GraphStoreCollectionKeyValue describes logical schema configuration, without physical storage metadata.
type GraphStoreCollectionKeyValue struct {
	// Required nonempty CK field name.
	Name string `json:"name"`
	// Required logical string constant; an empty string is allowed.
	Value string `json:"value"`
}

// GraphStoreEndpoint describes logical schema configuration, without physical storage metadata.
type GraphStoreEndpoint struct {
	// Required source or target label field name.
	Label string `json:"label"`
	// Required source or target ID field name.
	ID string `json:"id"`
}

// GraphStoreCollectionSchema describes logical schema configuration, without physical storage metadata.
type GraphStoreCollectionSchema struct {
	// Required nonempty logical collection name; no numeric collection ID is exposed.
	Name string `json:"name"`
	// Required nonempty description.
	Description string `json:"description"`
	// Required fixed vertex or edge layout.
	GraphLayout GraphStoreLayoutType `json:"graphLayout"`
	// Required complete user CK values; system CK values belong to the service.
	CollectionKeyValues []GraphStoreCollectionKeyValue `json:"collectionKeyValues"`
	// Nil inherits BaseSchema; explicit TTL must be positive seconds.
	TTLSeconds *int64 `json:"ttlSeconds,omitempty"`
	// Required property array; an empty array is valid.
	Fields []GraphStoreFieldSchema `json:"fields"`
}

// GraphStoreCollectionSchemaUpdate describes logical schema configuration, without physical storage metadata.
type GraphStoreCollectionSchemaUpdate struct {
	// Required nonempty name, identical to the request path.
	Name string `json:"name"`
	// Required nonempty replacement description.
	Description string `json:"description"`
	// Nil restores inheritance; explicit TTL must be positive seconds. Replacement, not patch.
	TTLSeconds *int64 `json:"ttlSeconds,omitempty"`
	// Required complete replacement property array; an empty array is valid.
	Fields []GraphStoreFieldSchema `json:"fields"`
}

// GraphStoreVertexLayout names key fields. Nil CK/PK/RouteKey use server defaults;
// non-nil empty slices are encoded as [] and are distinct from omitted keys.
type GraphStoreVertexLayout struct {
	// Required vertex label field name.
	Label string `json:"label"`
	// Required vertex ID field name.
	ID string `json:"id"`
	// Nil omits the key; an explicit empty list is preserved. Defaults and key rules belong to the service.
	CK []string `json:"ck,omitempty"`
	// Nil omits the key; an explicit empty list is preserved. Defaults and key rules belong to the service.
	PK []string `json:"pk,omitempty"`
	// Nil omits the key; an explicit empty list is preserved. Defaults and key rules belong to the service.
	RouteKey []string `json:"routeKey,omitempty"`
	// Optional typed key index configuration; nil omits the field.
	KeyIndexConfigs map[string]GraphStoreIndexConfig `json:"keyIndexConfigs,omitempty"`
}

// Name is the fixed wire discriminator.
func (GraphStoreVertexLayout) Name() string { return "vertex" }

// GraphStoreEdgeLayout names the source, label and target fields. Optional keys
// have the same omitted/empty semantics as GraphStoreVertexLayout.
type GraphStoreEdgeLayout struct {
	// Required source field names.
	Source GraphStoreEndpoint `json:"source"`
	// Required edge label field name.
	Label string `json:"label"`
	// Required target field names.
	Target GraphStoreEndpoint `json:"target"`
	// Nil omits the key; an explicit empty list is preserved. Defaults and key rules belong to the service.
	CK []string `json:"ck,omitempty"`
	// Nil omits the key; an explicit empty list is preserved. Defaults and key rules belong to the service.
	PK []string `json:"pk,omitempty"`
	// Nil omits the key; an explicit empty list is preserved. Defaults and key rules belong to the service.
	RouteKey []string `json:"routeKey,omitempty"`
	// Optional typed key index configuration; nil omits the field.
	KeyIndexConfigs map[string]GraphStoreIndexConfig `json:"keyIndexConfigs,omitempty"`
}

func (GraphStoreEdgeLayout) Name() string { return "edge" }

func (s GraphStoreVertexLayout) MarshalJSON() ([]byte, error) {
	type plain GraphStoreVertexLayout
	value := struct {
		*plain
		Name            string                            `json:"name"`
		CK              *[]string                         `json:"ck,omitempty"`
		PK              *[]string                         `json:"pk,omitempty"`
		RouteKey        *[]string                         `json:"routeKey,omitempty"`
		KeyIndexConfigs *map[string]GraphStoreIndexConfig `json:"keyIndexConfigs,omitempty"`
	}{plain: (*plain)(&s), Name: s.Name()}
	if s.CK != nil {
		value.CK = &s.CK
	}
	if s.PK != nil {
		value.PK = &s.PK
	}
	if s.RouteKey != nil {
		value.RouteKey = &s.RouteKey
	}
	if s.KeyIndexConfigs != nil {
		value.KeyIndexConfigs = &s.KeyIndexConfigs
	}
	return json.Marshal(value)
}

func (s GraphStoreEdgeLayout) MarshalJSON() ([]byte, error) {
	type plain GraphStoreEdgeLayout
	value := struct {
		*plain
		Name            string                            `json:"name"`
		CK              *[]string                         `json:"ck,omitempty"`
		PK              *[]string                         `json:"pk,omitempty"`
		RouteKey        *[]string                         `json:"routeKey,omitempty"`
		KeyIndexConfigs *map[string]GraphStoreIndexConfig `json:"keyIndexConfigs,omitempty"`
	}{plain: (*plain)(&s), Name: s.Name()}
	if s.CK != nil {
		value.CK = &s.CK
	}
	if s.PK != nil {
		value.PK = &s.PK
	}
	if s.RouteKey != nil {
		value.RouteKey = &s.RouteKey
	}
	if s.KeyIndexConfigs != nil {
		value.KeyIndexConfigs = &s.KeyIndexConfigs
	}
	return json.Marshal(value)
}

// GraphStoreBaseSchema owns both layouts; the wire representation remains graphLayouts.
type GraphStoreBaseSchema struct {
	// Required time state setting.
	TimeStateEnable bool `json:"timeStateEnable"`
	// Required positive seconds; validated by the service.
	TTLSeconds int64 `json:"ttlSeconds"`
	// Optional common property names; the service defaults omitted values to an empty array.
	CommonFieldNames []string `json:"commonFieldNames,omitempty"`
	// Required global index setting; configuration combinations are validated by the service.
	IndexEnable bool `json:"indexEnable"`
	// Optional global FullText configuration.
	FullTextIndex *GraphStoreFullTextIndex `json:"fullTextIndex,omitempty"`
	// Required vertex layout; encoded within graphLayouts.
	VertexLayout GraphStoreVertexLayout `json:"-"`
	// Required edge layout; encoded within graphLayouts.
	EdgeLayout GraphStoreEdgeLayout `json:"-"`
}

func (s GraphStoreBaseSchema) MarshalJSON() ([]byte, error) {
	type plain GraphStoreBaseSchema
	return json.Marshal(struct {
		plain
		GraphLayouts [2]interface{} `json:"graphLayouts"`
	}{plain(s), [2]interface{}{s.VertexLayout, s.EdgeLayout}})
}

func (s *GraphStoreBaseSchema) UnmarshalJSON(data []byte) error {
	type plain GraphStoreBaseSchema
	var candidate GraphStoreBaseSchema
	value := struct {
		*plain
		GraphLayouts []struct {
			GraphStoreVertexLayout
			Name   string             `json:"name"`
			Source GraphStoreEndpoint `json:"source"`
			Target GraphStoreEndpoint `json:"target"`
		} `json:"graphLayouts"`
	}{plain: (*plain)(&candidate)}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	for _, layout := range value.GraphLayouts {
		switch layout.Name {
		case "vertex":
			candidate.VertexLayout = layout.GraphStoreVertexLayout
		case "edge":
			candidate.EdgeLayout = GraphStoreEdgeLayout{layout.Source, layout.Label, layout.Target,
				layout.CK, layout.PK, layout.RouteKey, layout.KeyIndexConfigs}
		}
	}
	*s = candidate
	return nil
}
