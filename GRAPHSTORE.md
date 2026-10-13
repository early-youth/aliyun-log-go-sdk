# GraphStore SDK

Use the existing `sls.ClientInterface`, including `Client` and `TokenAutoUpdateClient`.
The following function accepts an existing client, project and store name:

```go
import sls "github.com/aliyun/aliyun-log-go-sdk"

func configureGraphStore(client sls.ClientInterface, project, store string) error {
    full := &sls.GraphStoreFullTextIndex{Token: " ,", CaseSensitive: false, Chn: false}
    base := sls.GraphStoreBaseSchema{
        TTLSeconds: 86400, IndexEnable: true, FullTextIndex: full,
        VertexLayout: sls.GraphStoreVertexLayout{Label: "vl", ID: "vid"},
        EdgeLayout: sls.GraphStoreEdgeLayout{Label: "el",
            Source: sls.GraphStoreEndpoint{Label: "sl", ID: "sid"},
            Target: sls.GraphStoreEndpoint{Label: "tl", ID: "tid"}},
    }
    if err := client.CreateGraphStore(project, &sls.CreateGraphStoreRequest{
        StoreName: store, ShardCount: 2, BaseSchema: base,
    }); err != nil { return err }
    fields := []sls.GraphStoreFieldSchema{
        {Name: "text", Type: sls.GraphStoreFieldString,
            IndexConfig: sls.GraphStoreIndexConfig{Enable: true, FullTextIndex: full}},
        {Name: "count", Type: sls.GraphStoreFieldInt,
            IndexConfig: sls.GraphStoreIndexConfig{Enable: true, RangeIndex: &sls.GraphStoreRangeIndex{}}},
        {Name: "exact", Type: sls.GraphStoreFieldBool,
            IndexConfig: sls.GraphStoreIndexConfig{Enable: true}},
    }
    for _, item := range []struct { name string; layout sls.GraphStoreLayoutType; ck []sls.GraphStoreCollectionKeyValue }{
        {"vertices", sls.GraphStoreLayoutVertex, []sls.GraphStoreCollectionKeyValue{{Name: "vl", Value: "person"}}},
        {"edges", sls.GraphStoreLayoutEdge, []sls.GraphStoreCollectionKeyValue{
            {Name: "sl", Value: "person"}, {Name: "el", Value: "knows"}, {Name: "tl", Value: "person"}}},
    } {
        schema := &sls.GraphStoreCollectionSchema{Name: item.name, Description: "example",
            GraphLayout: item.layout, CollectionKeyValues: item.ck, Fields: fields}
        if err := client.CreateGraphStoreCollectionSchema(project, store, schema); err != nil { return err }
    }
    // Nil TTL restores inheritance; [] replaces properties with an empty array.
    if err := client.UpdateGraphStoreCollectionSchema(project, store, "vertices",
        &sls.GraphStoreCollectionSchemaUpdate{Name: "vertices", Description: "updated", Fields: []sls.GraphStoreFieldSchema{}}); err != nil { return err }
    result, err := client.GetGraphStoreBaseSchema(project, store)
    if err != nil { return err }
    _ = result.BaseSchema.VertexLayout.ID
    page, err := client.ListGraphStoreCollectionSchemas(project, store, 100, "")
    if err != nil { return err }
    for _, schema := range page.CollectionSchemas { _ = schema.Fields }
    // Explicitly pass page.NextPageToken when requesting another page.
    return nil
}
```

## Logical schema contract

All schema inputs and Get/List results are concrete models. This changes source compatibility:
remove raw JSON arguments and migrate custom SDK interface implementations. There is no raw
compatibility overload. HTTP still uses `baseSchema`, `value`, `collectionSchema` and
`collectionSchemas`; BaseSchema's two typed layouts encode as `graphLayouts:[vertex,edge]`.

| Model | Fields and meaning |
| --- | --- |
| BaseSchema | Required timeStateEnable, nonnegative int64 ttlSeconds, indexEnable, vertexLayout and edgeLayout; commonFieldNames defaults to []; optional fullTextIndex. |
| VertexLayout | Fixed read-only name vertex; required label/id **field names**. Optional ck/pk/routeKey and keyIndexConfigs. |
| EdgeLayout | Fixed read-only name edge; required source/target Endpoint and label field name; the same optional keys/index map. |
| Endpoint | Required nonempty label/id field names. |
| CollectionSchema | Required nonempty name/description, graphLayout (vertex/edge), collectionKeyValues and fields arrays. Optional int64 ttlSeconds. |
| CollectionSchemaUpdate | Required name/description and complete replacement fields array; optional ttlSeconds. No layout or CK mutation fields. |
| CollectionKeyValue | Required nonempty name and string value; value may be empty. These are logical constants, not arbitrary binary PB values. |
| FieldSchema | Required nonempty name, type (STRING/INT/DOUBLE/BOOL) and IndexConfig. INT is signed int64. |
| IndexConfig | Required enable; optional FullTextIndex or empty RangeIndex. Enabled with neither selects Exact. |
| FullTextIndex | Required nonempty token string, caseSensitive and chn; optional nonempty optionToken string. |

Base TTL 0 disables expiry. Collection TTL unset/null inherits BaseSchema; 0 disables expiry;
positive values override in seconds. Update is replacement: unset/null TTL restores inheritance.
Required `fields` and `collectionKeyValues` arrays may be empty. Optional keys are omitted when
unset; explicit empty lists remain empty lists. Vertex defaults are ck=[label], pk=[id],
routeKey=[label]; Edge defaults are ck=[source.label,label,target.label],
pk=[source.id,target.id], routeKey=[source.label]. Defaults, valid key projections, index
combinations, token rules, physical IDs/roles/versions and logical-to-physical compilation belong
to the service. The SDK checks required values, empty elements, basic ranges and Update name equality.
Unknown schema fields/enums, wrong known-field types and integer overflow return a parameter or
BadResponse error instead of silently discarding configuration; upgrade the SDK to support new schema fields.

Collection Get/Update/Delete take a string name. Names and pagination tokens remain opaque;
SDK path checks allow safe ASCII characters (including `.`/`@` for collections) and reject `.`/`..`.
The service owns configurable naming limits, the 1000 collection create quota and the 1 MiB
normalized CollectionSchema JSON array cap (BaseSchema excluded).

Go optional nested objects/TTL use pointers. Optional key slices/maps use nil for omission;
non-nil empty slices/maps encode as []/{}. StoreID is int64; SplitGraphStoreShard's *uint64
separates omission from zero. ListGraphStores Offset/Size use *int; collection pageSize 0 omits it.
Missing/null nextPageToken means no next page. GetGraphStore reads signed-int32 shards without
rechecking their count, order or uniqueness. Ordinary response extensions are ignored.

## Write and query

```go
err := client.PostGraphStoreData(project, storeName, &sls.PostGraphStoreDataRequest{
    LogGroup: group, CompressType: sls.Compress_LZ4,
})
rows, err := client.GetGraphStoreData(project, storeName, &sls.GetGraphStoreDataRequest{
    Query: "MATCH (n) RETURN n", Start: 1700000000, End: 1700001000, Session: "opaque",
})
// rows.Data.Fields, rows.Data.Values ([][]*string), rows.Meta and rows.RequestID.
// Nil cell is JSON null; pointer to "" is an empty string. Handle err before reading rows.
```

Write returns error only. NONE/LZ4/Zstd retain native LogStore behavior; LZ4 is the default.
After compression validation, nil/zero-log groups are local no-ops; a nil request is invalid.
Actual writes serialize standard LogGroup once, preserving exact binary PB values. Management
uses c.request; data APIs retain request(convert(...)), RetryOnServerErrorEnabled, RetryTimeOut
and credential refresh. The retry timeout does not cancel an in-flight HTTP request.

Query sends int64 closed [start,end], 0 <= start <= end. NewGetGraphStoreDataRequest defaults
to [0,4294967295]; an explicit struct supports [0,0]. Session is optional and opaque.
Ordinary management/query bodies decode through encoding/json into typed fields. Required
outer/data/meta fields are checked; native type/overflow errors remain errors. Unknown result
members and unequal row widths are accepted. Native string decoding handles null string-list
elements and invalid UTF-8; no extra UTF-8 pass or cell coercion is added. Responses support
uncompressed/LZ4 through the existing decompressor. The raw-size header must fit int;
there is no GraphStore 256 MiB cap. Actual decompression/length errors remain BadResponse.
Management and query decode failures retain HTTP code, response headers and requestId without
parser retries. Service errors retain *sls.Error; mutations can have unknown results after timeout.
ACK does not promise query visibility/durability; retries may duplicate delivery. Callers own polling.

SDK validation uses `go test .`, targeted race tests and `go build .`.
All 13 operations stay on the existing interface. No dependency/module version change or release
is implied. E2E bridges are outside this SDK refinement and need separate migration before reuse.
