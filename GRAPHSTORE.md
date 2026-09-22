# GraphStore SDK

Use the existing SLS ClientInterface, including CreateNormalInterface/CreateNormalInterfaceV2
and TokenAutoUpdateClient. No separate GraphStore client is required.

```go
store, err := client.GetGraphStore(project, storeName)
if err != nil { return err }
_ = store.CurrentShardCount

page, err := client.ListGraphStoreCollectionSchemas(project, storeName, 100, "")
if err != nil { return err }
// Explicitly use page.NextPageToken for a subsequent page.
_ = page.CollectionSchemas
```

CreateGraphStore takes *CreateGraphStoreRequest (StoreName, ShardCount, BaseSchema).
Schemas are complete logical object values in json.RawMessage; Create/UpdateCollectionSchema
wrap them as a JSON object, never as an escaped string. Returned schemas stay opaque RawMessage.
Collection Get/Update/Delete take collectionSchemaName string, not an ID; numeric-looking
names remain strings. Get/List schemas require name and have no added ID. Update values
must include the same name as the path; the service enforces immutable name/layout/keys.
Collection pages use raw last-name tokens with byte ordering, default size 100/max 1000;
the SDK passes tokens through without interpretation. The service owns configurable name
validation (default lowercase/digits/interior '-'/'_', length 2..63), uniqueness and the
10240 logical-schema create quota. SDK path validation only prevents unsafe path segments.
StoreID stays int64; SplitGraphStoreShard's *uint64 distinguishes nil from a pointer to zero.
ListGraphStores accepts optional *int Offset/Size; collection pageSize 0 omits the parameter.

Mutations return error only. A service *sls.Error retains HTTPCode and RequestID.
Management methods add no retry; TokenAutoUpdateClient retains credential-refresh behavior.
Timeouts may leave mutation results unknown.
No automatic pagination or lifecycle polling.

Compatibility: ClientInterface adds 13 GraphStore methods (11 management, write and query). Third-party implementations must
add those methods; callers using the SDK's implementations keep using the existing constructors.
No dependency or module version changes; these local changes are not a public release.

## Write and query

```go
err := client.PostGraphStoreData(project, storeName, &sls.PostGraphStoreDataRequest{
    LogGroup: group, // *sls.LogGroup; standard protobuf, preserving binary values.
    CompressType: sls.Compress_LZ4,
})
if err != nil { return err }
rows, err := client.GetGraphStoreData(project, storeName, &sls.GetGraphStoreDataRequest{
    Query: "MATCH (n) RETURN n",
    From: 1700000000, To: 1700001000, Session: "opaque",
})
if err != nil { return err }
// rows.Data.Fields, rows.Data.Values ([][]*string), rows.Meta and rows.RequestID.
// A nil cell is JSON null; a pointer to "" is an empty string.
```

Write returns only error. Compression defaults to LZ4 (zero); NONE and Zstd are also supported,
exactly as in LogStore. After compression validation, a nil/zero-log group is a local no-op:
no request and no server ACK. A nil request is invalid. No local size threshold is added.
Actual writes serialize the standard LogGroup once, then use the existing LogStore request,
signing and retry path. Data methods retain RetryOnServerErrorEnabled, RetryTimeOut and
credential refresh; retry timeout does not cancel an already-running HTTP request.
An ACK does not imply query visibility or durability. Timeout results may be unknown and
retrying can duplicate delivery. Callers own query convergence polling.

GetGraphStore returns ordered unique signed-int32 shards; List keeps summary items.
Query requires uint32 From < To with [from,to) semantics and accepts an optional opaque Session.
The public response has data.fields/values and only meta.truncated/elapsedMs/warnings;
elapsedMs is signed int64. Cells stay string/null, including complex graph values.
Old header/rowCount/peakMemoryBytes/resultBytes are not accepted. Missing, null or mistyped required fields, row width
mismatches, invalid JSON/UTF-8 and bad compressed responses fail without partial rows or
parser retries. Responses support uncompressed or LZ4 only, with raw size checked against
256 MiB before LZ4 allocation. Existing LogStore response behavior is unchanged.

Validation: go test ./...; go test -race -run GraphStore ./...
The e2e-tagged internal/graphstore-e2e bridge runs one native SDK operation for the shared
GraphStore E2E Runner. Build with:
```sh
go build -tags=e2e -o /path/to/graphstore-go-e2e ./internal/graphstore-e2e
```
Credentials come only from GRAPHSTORE_E2E_ACCESS_KEY_ID/GRAPHSTORE_E2E_ACCESS_KEY_SECRET.
The Runner owns scenarios, deadlines and resource cleanup. The bridge exposes all 13 management,
write and query operations.
