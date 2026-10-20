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
| BaseSchema | Required timeStateEnable, positive int64 ttlSeconds, indexEnable, vertexLayout and edgeLayout; commonFieldNames defaults to []; optional fullTextIndex. |
| VertexLayout | Fixed read-only name vertex; required label/id **field names**. Optional ck/pk/routeKey and keyIndexConfigs. |
| EdgeLayout | Fixed read-only name edge; required source/target Endpoint and label field name; the same optional keys/index map. |
| Endpoint | Required nonempty label/id field names. |
| CollectionSchema | Required nonempty name/description, graphLayout (vertex/edge), collectionKeyValues and fields arrays. Optional positive int64 ttlSeconds. |
| CollectionSchemaUpdate | Required name/description and complete replacement fields array; optional positive ttlSeconds. No layout or CK mutation fields. |
| CollectionKeyValue | Required nonempty name and string value; value may be empty. These are logical constants, not arbitrary binary PB values. |
| FieldSchema | Required nonempty name, type (STRING/INT/DOUBLE/BOOL) and IndexConfig. INT is signed int64. |
| IndexConfig | Required enable; optional FullTextIndex or empty RangeIndex. Enabled with neither selects Exact. |
| FullTextIndex | Required nonempty token string, caseSensitive and chn; optional nonempty optionToken string. |

Base TTL must be positive. Collection TTL unset/null inherits BaseSchema; an explicit value
must be positive seconds. Zero/negative TTL values are invalid. Update is replacement:
unset/null TTL restores inheritance. The service owns these business checks.
Required `fields` and `collectionKeyValues` arrays may be empty. Optional keys are omitted when
unset; explicit empty lists remain empty lists. Vertex defaults are ck=[label], pk=[id],
routeKey=[label]; Edge defaults are ck=[source.label,label,target.label],
pk=[source.id,target.id], routeKey=[source.label]. Defaults, valid key projections, index
combinations, token rules, physical IDs/roles/versions and logical-to-physical compilation belong
to the service. Like LogStore indexes, ordinary schema fields use encoding/json and their JSON tags;
missing/null members use native zero/nil values and unknown members are ignored. String-backed enum
values are preserved. The SDK does not repeat required/nonempty/TTL business checks. BaseSchema only
adapts graphLayouts, and omitted commonFieldNames uses the service default. Known-field type and integer
overflow errors retain BadResponse context; path safety, envelope metadata and Update name equality remain checked.

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
// rows.Data.Fields, rows.Data.Values ([][]interface{}), rows.Meta and rows.RequestID.
// nil is JSON null; numbers are json.Number; strings, booleans, maps and lists remain native. Handle err first.
```

Write returns error only. NONE/LZ4/Zstd retain native LogStore behavior; LZ4 is the default.
After compression validation, nil/zero-log groups are local no-ops; a nil request is invalid.
Actual writes serialize standard LogGroup once, preserving exact binary PB values. Management
uses c.request; data APIs retain request(convert(...)), RetryOnServerErrorEnabled, RetryTimeOut
and credential refresh. The retry timeout does not cancel an in-flight HTTP request.

Query sends int64 closed [start,end], 0 <= start <= end. NewGetGraphStoreDataRequest defaults
to [0,4294967295]; an explicit struct supports [0,0]. Session is optional and opaque.
Management bodies decode through encoding/json into typed fields; query uses json.Decoder.UseNumber
and rejects trailing input. Required
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
is implied. The E2E bridge decodes fixture JSON into these typed models before calling the SDK.

## Runnable CRUD sample

[example/graphstore/main.go](example/graphstore/main.go) 可直接运行，包含中文注释。
在 Go SDK 仓库根目录执行，先配置环境变量（AK/SK 从本机安全凭据源注入，不写入命令历史）：

```bash
export GRAPHSTORE_ENDPOINT='https://cn-wulanchabu-stg-pg2-share.log.aliyuncs.com'
export GRAPHSTORE_PROJECT='luyao-test-wulan-pg2'
# 同时设置 ALIBABA_CLOUD_ACCESS_KEY_ID 和 ALIBABA_CLOUD_ACCESS_KEY_SECRET
go run ./example/graphstore
```

示例使用已有 Project，不要求为空，也不创建/删除 Project。每次创建唯一名称的 Store，
演示 Store Create/Get/List/Delete、BaseSchema 读取、CollectionSchema CRUD/分页，以及
两个 `person` 顶点和一条 `knows` 边的写入、查询、更新和删除。ID 使用 `alice` / `bob`，无需 32 位。
Base TTL 为 86400 秒，Collection 初始继承 Base，更新时完整提交字段并设置 TTL=3600 秒。
不演示 Split、Project 管理或故障恢复。

全程复用一个原生 SDK Client，每次 HTTP 请求超时 30 秒。
`GRAPHSTORE_SIGNATURE_VERSION` 默认 `v1`，只接受 `v1` / `v4`；使用 V4 必须显式设置
`GRAPHSTORE_REGION`，不从域名推断。无需修改源码即可分别运行：

```bash
GRAPHSTORE_SIGNATURE_VERSION=v1 go run ./example/graphstore
GRAPHSTORE_SIGNATURE_VERSION=v4 GRAPHSTORE_REGION=cn-wulanchabu-stg-pg2 go run ./example/graphstore
```

READY/Schema 完成后，针对当前 stg 缓存收敛固定等待 65 秒再首写；READY、查询读回和
删除终态各轮询最多 120 秒，另加最后一次在途 SDK 调用耗时。
五次读回严格比较列名、行值和 `truncated=false`，ACK 不代表已持久化或立即可查询。
示例不自行重放失败写入；原生 SDK 的 retry/credential refresh 行为保持不变，超时可能结果未知，
原生重试也可能重复投递。

只有 Create 明确成功后才自动清理本次 Store，业务中途失败也会清理；Create 失败或清理失败时
根据启动输出的 Project/Store 名核对资源。成功标记为
`GraphStore sample passed; Store deletion confirmed.`，同时确认精确的 `404/GraphStoreNotExist`。
管理和写入 API 原生只返回 error，查询成功响应提供 request ID；失败诊断不输出原始响应正文。

查询结果保留 JSON 原生类型：数字、布尔、数组、对象、null 和字符串；路径为数组，实体为对象。
业务属性中的 JSON 文本仍是字符串，不递归解析。Go 的数字为 `json.Number`，可调用 `Int64()` 或 `Float64()`，避免大整数精度丢失。
