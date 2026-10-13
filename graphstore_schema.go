package sls

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// Schema objects must not silently discard configuration an older SDK cannot represent.
func decodeGraphStoreSchema(data []byte, value interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}

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

func (s *GraphStoreFullTextIndex) UnmarshalJSON(data []byte) error {
	type plain GraphStoreFullTextIndex
	var value struct {
		*plain
		Token         *string `json:"token"`
		CaseSensitive *bool   `json:"caseSensitive"`
		Chn           *bool   `json:"chn"`
	}
	var candidate GraphStoreFullTextIndex
	value.plain = (*plain)(&candidate)
	if err := decodeGraphStoreSchema(data, &value); err != nil {
		return err
	}
	if value.Token == nil || value.CaseSensitive == nil || value.Chn == nil {
		return fmt.Errorf("missing or null GraphStoreFullTextIndex field")
	}
	candidate.Token = *value.Token
	candidate.CaseSensitive = *value.CaseSensitive
	candidate.Chn = *value.Chn
	*s = candidate
	return nil
}

// GraphStoreRangeIndex describes logical schema configuration, without physical storage metadata.
type GraphStoreRangeIndex struct {
}

func (s *GraphStoreRangeIndex) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("GraphStoreRangeIndex must be an object")
	}
	return decodeGraphStoreSchema(data, &struct{}{})
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

func (s *GraphStoreIndexConfig) UnmarshalJSON(data []byte) error {
	type plain GraphStoreIndexConfig
	var value struct {
		*plain
		Enable *bool `json:"enable"`
	}
	var candidate GraphStoreIndexConfig
	value.plain = (*plain)(&candidate)
	if err := decodeGraphStoreSchema(data, &value); err != nil {
		return err
	}
	if value.Enable == nil {
		return fmt.Errorf("missing or null GraphStoreIndexConfig field")
	}
	candidate.Enable = *value.Enable
	*s = candidate
	return nil
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

func (s *GraphStoreFieldSchema) UnmarshalJSON(data []byte) error {
	type plain GraphStoreFieldSchema
	var value struct {
		*plain
		Name        *string                `json:"name"`
		Type        *GraphStoreFieldType   `json:"type"`
		IndexConfig *GraphStoreIndexConfig `json:"indexConfig"`
	}
	var candidate GraphStoreFieldSchema
	value.plain = (*plain)(&candidate)
	if err := decodeGraphStoreSchema(data, &value); err != nil {
		return err
	}
	if value.Name == nil || value.Type == nil || value.IndexConfig == nil {
		return fmt.Errorf("missing or null GraphStoreFieldSchema field")
	}
	candidate.Name = *value.Name
	candidate.Type = *value.Type
	candidate.IndexConfig = *value.IndexConfig
	*s = candidate
	return nil
}

// GraphStoreCollectionKeyValue describes logical schema configuration, without physical storage metadata.
type GraphStoreCollectionKeyValue struct {
	// Required nonempty CK field name.
	Name string `json:"name"`
	// Required logical string constant; an empty string is allowed.
	Value string `json:"value"`
}

func (s *GraphStoreCollectionKeyValue) UnmarshalJSON(data []byte) error {
	type plain GraphStoreCollectionKeyValue
	var value struct {
		*plain
		Name  *string `json:"name"`
		Value *string `json:"value"`
	}
	var candidate GraphStoreCollectionKeyValue
	value.plain = (*plain)(&candidate)
	if err := decodeGraphStoreSchema(data, &value); err != nil {
		return err
	}
	if value.Name == nil || value.Value == nil {
		return fmt.Errorf("missing or null GraphStoreCollectionKeyValue field")
	}
	candidate.Name = *value.Name
	candidate.Value = *value.Value
	*s = candidate
	return nil
}

// GraphStoreEndpoint describes logical schema configuration, without physical storage metadata.
type GraphStoreEndpoint struct {
	// Required source or target label field name.
	Label string `json:"label"`
	// Required source or target ID field name.
	ID string `json:"id"`
}

func (s *GraphStoreEndpoint) UnmarshalJSON(data []byte) error {
	type plain GraphStoreEndpoint
	var value struct {
		*plain
		Label *string `json:"label"`
		ID    *string `json:"id"`
	}
	var candidate GraphStoreEndpoint
	value.plain = (*plain)(&candidate)
	if err := decodeGraphStoreSchema(data, &value); err != nil {
		return err
	}
	if value.Label == nil || value.ID == nil {
		return fmt.Errorf("missing or null GraphStoreEndpoint field")
	}
	candidate.Label = *value.Label
	candidate.ID = *value.ID
	*s = candidate
	return nil
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
	// Nil inherits BaseSchema; zero disables expiry; positive values are seconds.
	TTLSeconds *int64 `json:"ttlSeconds,omitempty"`
	// Required property array; an empty array is valid.
	Fields []GraphStoreFieldSchema `json:"fields"`
}

func (s *GraphStoreCollectionSchema) UnmarshalJSON(data []byte) error {
	type plain GraphStoreCollectionSchema
	var value struct {
		*plain
		Name                *string                         `json:"name"`
		Description         *string                         `json:"description"`
		GraphLayout         *GraphStoreLayoutType           `json:"graphLayout"`
		CollectionKeyValues *[]GraphStoreCollectionKeyValue `json:"collectionKeyValues"`
		Fields              *[]GraphStoreFieldSchema        `json:"fields"`
	}
	var candidate GraphStoreCollectionSchema
	value.plain = (*plain)(&candidate)
	if err := decodeGraphStoreSchema(data, &value); err != nil {
		return err
	}
	if value.Name == nil || value.Description == nil || value.GraphLayout == nil || value.CollectionKeyValues == nil || value.Fields == nil {
		return fmt.Errorf("missing or null GraphStoreCollectionSchema field")
	}
	candidate.Name = *value.Name
	candidate.Description = *value.Description
	candidate.GraphLayout = *value.GraphLayout
	candidate.CollectionKeyValues = *value.CollectionKeyValues
	candidate.Fields = *value.Fields
	if err := candidate.validate(); err != nil {
		return err
	}
	*s = candidate
	return nil
}

// GraphStoreCollectionSchemaUpdate describes logical schema configuration, without physical storage metadata.
type GraphStoreCollectionSchemaUpdate struct {
	// Required nonempty name, identical to the request path.
	Name string `json:"name"`
	// Required nonempty replacement description.
	Description string `json:"description"`
	// Nil restores inheritance; zero disables expiry; this is replacement, not patch.
	TTLSeconds *int64 `json:"ttlSeconds,omitempty"`
	// Required complete replacement property array; an empty array is valid.
	Fields []GraphStoreFieldSchema `json:"fields"`
}

func (s *GraphStoreCollectionSchemaUpdate) UnmarshalJSON(data []byte) error {
	type plain GraphStoreCollectionSchemaUpdate
	var value struct {
		*plain
		Name        *string                  `json:"name"`
		Description *string                  `json:"description"`
		Fields      *[]GraphStoreFieldSchema `json:"fields"`
	}
	var candidate GraphStoreCollectionSchemaUpdate
	value.plain = (*plain)(&candidate)
	if err := decodeGraphStoreSchema(data, &value); err != nil {
		return err
	}
	if value.Name == nil || value.Description == nil || value.Fields == nil {
		return fmt.Errorf("missing or null GraphStoreCollectionSchemaUpdate field")
	}
	candidate.Name = *value.Name
	candidate.Description = *value.Description
	candidate.Fields = *value.Fields
	if err := candidate.validate(); err != nil {
		return err
	}
	*s = candidate
	return nil
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
	TimeStateEnable bool
	// Required nonnegative seconds; zero disables expiry.
	TTLSeconds int64 // Nonnegative seconds; zero disables expiry.
	// Common property names; defaults to an empty array.
	CommonFieldNames []string // Nil defaults to an empty array.
	// Required global index setting; configuration combinations are validated by the service.
	IndexEnable bool
	// Optional global FullText configuration.
	FullTextIndex *GraphStoreFullTextIndex
	// Required vertex layout; encoded within graphLayouts.
	VertexLayout GraphStoreVertexLayout
	// Required edge layout; encoded within graphLayouts.
	EdgeLayout GraphStoreEdgeLayout
}

func (s GraphStoreBaseSchema) MarshalJSON() ([]byte, error) {
	fields := s.CommonFieldNames
	if fields == nil {
		fields = []string{}
	}
	return json.Marshal(struct {
		TimeStateEnable  bool                     `json:"timeStateEnable"`
		TTLSeconds       int64                    `json:"ttlSeconds"`
		CommonFieldNames []string                 `json:"commonFieldNames"`
		IndexEnable      bool                     `json:"indexEnable"`
		FullTextIndex    *GraphStoreFullTextIndex `json:"fullTextIndex,omitempty"`
		GraphLayouts     [2]interface{}           `json:"graphLayouts"`
	}{s.TimeStateEnable, s.TTLSeconds, fields, s.IndexEnable, s.FullTextIndex,
		[2]interface{}{s.VertexLayout, s.EdgeLayout}})
}

// Only this private wire object needs both alternatives. It decodes each layout once.
type graphStoreLayout struct {
	Name            *string                          `json:"name"`
	Label           *string                          `json:"label"`
	ID              graphStoreLayoutID               `json:"id"`
	Source          graphStoreLayoutEndpoint         `json:"source"`
	Target          graphStoreLayoutEndpoint         `json:"target"`
	CK              []string                         `json:"ck"`
	PK              []string                         `json:"pk"`
	RouteKey        []string                         `json:"routeKey"`
	KeyIndexConfigs map[string]GraphStoreIndexConfig `json:"keyIndexConfigs"`
}

// Presence distinguishes an unsupported union member from an omitted one, including null.
type graphStoreLayoutID struct {
	value   *string
	present bool
}

func (s *graphStoreLayoutID) UnmarshalJSON(data []byte) error {
	s.present = true
	return json.Unmarshal(data, &s.value)
}

type graphStoreLayoutEndpoint struct {
	value   GraphStoreEndpoint
	present bool
}

func (s *graphStoreLayoutEndpoint) UnmarshalJSON(data []byte) error {
	s.present = true
	return json.Unmarshal(data, &s.value)
}

func (s *graphStoreLayout) UnmarshalJSON(data []byte) error {
	type plain graphStoreLayout
	if err := decodeGraphStoreSchema(data, (*plain)(s)); err != nil {
		return err
	}
	if s.Name == nil || s.Label == nil {
		return fmt.Errorf("missing layout name/label")
	}
	switch *s.Name {
	case "vertex":
		if s.ID.value == nil || s.Source.present || s.Target.present {
			return fmt.Errorf("invalid vertex layout")
		}
	case "edge":
		if !s.Source.present || !s.Target.present || s.ID.present {
			return fmt.Errorf("invalid edge layout")
		}
	default:
		return fmt.Errorf("unsupported GraphStore layout %q", *s.Name)
	}
	return nil
}

func (s *GraphStoreBaseSchema) UnmarshalJSON(data []byte) error {
	var value struct {
		TimeStateEnable  *bool                    `json:"timeStateEnable"`
		TTLSeconds       *int64                   `json:"ttlSeconds"`
		CommonFieldNames []string                 `json:"commonFieldNames"`
		IndexEnable      *bool                    `json:"indexEnable"`
		FullTextIndex    *GraphStoreFullTextIndex `json:"fullTextIndex"`
		GraphLayouts     []graphStoreLayout       `json:"graphLayouts"`
	}
	if err := decodeGraphStoreSchema(data, &value); err != nil {
		return err
	}
	if value.TimeStateEnable == nil || value.TTLSeconds == nil || value.IndexEnable == nil || len(value.GraphLayouts) != 2 {
		return fmt.Errorf("incomplete GraphStoreBaseSchema")
	}
	candidate := GraphStoreBaseSchema{TimeStateEnable: *value.TimeStateEnable, TTLSeconds: *value.TTLSeconds,
		CommonFieldNames: value.CommonFieldNames, IndexEnable: *value.IndexEnable, FullTextIndex: value.FullTextIndex}
	var vertex, edge bool
	for _, layout := range value.GraphLayouts {
		if *layout.Name == "vertex" {
			if vertex {
				return fmt.Errorf("duplicate vertex layout")
			}
			vertex = true
			candidate.VertexLayout = GraphStoreVertexLayout{*layout.Label, *layout.ID.value, layout.CK, layout.PK, layout.RouteKey, layout.KeyIndexConfigs}
		} else {
			if edge {
				return fmt.Errorf("duplicate edge layout")
			}
			edge = true
			candidate.EdgeLayout = GraphStoreEdgeLayout{layout.Source.value, *layout.Label, layout.Target.value, layout.CK, layout.PK, layout.RouteKey, layout.KeyIndexConfigs}
		}
	}
	if candidate.CommonFieldNames == nil {
		candidate.CommonFieldNames = []string{}
	}
	if err := candidate.validate(); err != nil {
		return err
	}
	*s = candidate
	return nil
}

func graphStoreSchemaStrings(values []string) error {
	for _, value := range values {
		if value == "" {
			return fmt.Errorf("empty GraphStore schema field name")
		}
	}
	return nil
}
func (s GraphStoreFullTextIndex) validate() error {
	if s.Token == "" || s.OptionToken != nil && *s.OptionToken == "" {
		return fmt.Errorf("empty full-text token")
	}
	return nil
}
func (s GraphStoreIndexConfig) validate() error {
	if s.FullTextIndex != nil {
		return s.FullTextIndex.validate()
	}
	return nil
}
func (s GraphStoreEndpoint) validate() error {
	if s.Label == "" || s.ID == "" {
		return fmt.Errorf("endpoint label/id are required")
	}
	return nil
}
func validateGraphStoreKeys(ck, pk, route []string, indexes map[string]GraphStoreIndexConfig) error {
	for _, keys := range [][]string{ck, pk, route} {
		if err := graphStoreSchemaStrings(keys); err != nil {
			return err
		}
	}
	for name, index := range indexes {
		if name == "" {
			return fmt.Errorf("empty key index name")
		}
		if err := index.validate(); err != nil {
			return err
		}
	}
	return nil
}
func (s GraphStoreBaseSchema) validate() error {
	if s.TTLSeconds < 0 {
		return fmt.Errorf("negative BaseSchema TTL")
	}
	if s.VertexLayout.Label == "" || s.VertexLayout.ID == "" || s.EdgeLayout.Label == "" {
		return fmt.Errorf("layout field names are required")
	}
	if err := s.EdgeLayout.Source.validate(); err != nil {
		return err
	}
	if err := s.EdgeLayout.Target.validate(); err != nil {
		return err
	}
	if err := graphStoreSchemaStrings(s.CommonFieldNames); err != nil {
		return err
	}
	if err := validateGraphStoreKeys(s.VertexLayout.CK, s.VertexLayout.PK, s.VertexLayout.RouteKey, s.VertexLayout.KeyIndexConfigs); err != nil {
		return err
	}
	if err := validateGraphStoreKeys(s.EdgeLayout.CK, s.EdgeLayout.PK, s.EdgeLayout.RouteKey, s.EdgeLayout.KeyIndexConfigs); err != nil {
		return err
	}
	if s.FullTextIndex != nil {
		return s.FullTextIndex.validate()
	}
	return nil
}
func validateGraphStoreCollection(name, description string, ttl *int64, fields []GraphStoreFieldSchema) error {
	if name == "" || description == "" || fields == nil {
		return fmt.Errorf("collection name, description and fields are required")
	}
	if ttl != nil && *ttl < 0 {
		return fmt.Errorf("negative collection TTL")
	}
	for _, field := range fields {
		if field.Name == "" {
			return fmt.Errorf("empty collection field name")
		}
		switch field.Type {
		case GraphStoreFieldString, GraphStoreFieldInt, GraphStoreFieldDouble, GraphStoreFieldBool:
		default:
			return fmt.Errorf("unsupported GraphStore field type %q", field.Type)
		}
		if err := field.IndexConfig.validate(); err != nil {
			return err
		}
	}
	return nil
}
func (s GraphStoreCollectionSchema) validate() error {
	if s.GraphLayout != GraphStoreLayoutVertex && s.GraphLayout != GraphStoreLayoutEdge {
		return fmt.Errorf("unsupported collection layout %q", s.GraphLayout)
	}
	if s.CollectionKeyValues == nil {
		return fmt.Errorf("collectionKeyValues is required")
	}
	for _, value := range s.CollectionKeyValues {
		if value.Name == "" {
			return fmt.Errorf("empty CK name")
		}
	}
	return validateGraphStoreCollection(s.Name, s.Description, s.TTLSeconds, s.Fields)
}
func (s GraphStoreCollectionSchemaUpdate) validate() error {
	return validateGraphStoreCollection(s.Name, s.Description, s.TTLSeconds, s.Fields)
}
