// 在已有 Project 中演示 GraphStore CRUD；配置和运行方式见根目录 GRAPHSTORE.md。
package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	sls "github.com/aliyun/aliyun-log-go-sdk"
	"github.com/gogo/protobuf/proto"
)

const (
	// ID 是非空字符串，无需固定长度；边的 sid/tid 须与对应顶点 ID 一致。
	aliceID     = "alice"
	bobID       = "bob"
	personQuery = "MATCH (n:person) RETURN n.name AS name, n.age AS age ORDER BY n.name"
	edgeQuery   = "MATCH (a:person)-[:knows]->(b:person) RETURN a.name AS source, b.name AS target"
	waitTimeout = 120 * time.Second
)

type sample struct {
	client         sls.ClientInterface
	project, store string
}

func main() {
	if err := runExample(); err != nil {
		fmt.Fprintln(os.Stderr, failure(err))
		fmt.Fprintln(os.Stderr, "If Create or cleanup had an unknown result, check the Store printed above.")
		os.Exit(1)
	}
}

func runExample() error {
	// 凭据只从环境变量读取，避免写入源码或命令行参数。
	for _, name := range []string{"GRAPHSTORE_ENDPOINT", "GRAPHSTORE_PROJECT", "ALIBABA_CLOUD_ACCESS_KEY_ID", "ALIBABA_CLOUD_ACCESS_KEY_SECRET"} {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			return fmt.Errorf("set environment variable %s", name)
		}
	}
	version := os.Getenv("GRAPHSTORE_SIGNATURE_VERSION")
	if version == "" {
		version = "v1"
	}
	if version != "v1" && version != "v4" {
		return fmt.Errorf("GRAPHSTORE_SIGNATURE_VERSION must be v1 or v4")
	}
	region := os.Getenv("GRAPHSTORE_REGION")
	if version == "v4" && strings.TrimSpace(region) == "" {
		return fmt.Errorf("set GRAPHSTORE_REGION for V4 signing")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	provider := sls.NewStaticCredentialsProvider(os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID"), os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET"), "")
	client := sls.CreateNormalInterfaceV2(os.Getenv("GRAPHSTORE_ENDPOINT"), provider)
	// V4 的 region 是签名输入，不能根据域名猜测；全流程复用原生 Client 和连接池。
	client.SetAuthVersion(sls.AuthVersionType(version))
	if version == "v4" {
		client.SetRegion(region)
	}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	client.SetHTTPClient(httpClient)
	defer httpClient.CloseIdleConnections()
	defer client.Close()
	s := sample{client: client, project: os.Getenv("GRAPHSTORE_PROJECT"), store: fmt.Sprintf("graphstore-go-sample-%x", id)}
	// Create 超时时结果可能未知，提前输出唯一资源名，供人工核对。
	fmt.Printf("Sample resources: project=%s, store=%s\n", s.project, s.store)
	fmt.Printf("Signing: version=%s, region=%s\n", version, region)
	if err := s.run(); err != nil {
		return err
	}
	fmt.Println("GraphStore sample passed; Store deletion confirmed.")
	return nil
}

func (s sample) run() (err error) {
	// TTL 必须为正数；vl/vid 等是字段名，person/knows 才是数据中的标签值。
	base := sls.GraphStoreBaseSchema{
		TTLSeconds:   86400,
		VertexLayout: sls.GraphStoreVertexLayout{Label: "vl", ID: "vid"},
		EdgeLayout: sls.GraphStoreEdgeLayout{Label: "el",
			Source: sls.GraphStoreEndpoint{Label: "sl", ID: "sid"},
			Target: sls.GraphStoreEndpoint{Label: "tl", ID: "tid"}},
	}
	if err = s.client.CreateGraphStore(s.project, &sls.CreateGraphStoreRequest{StoreName: s.store, ShardCount: 1, BaseSchema: base}); err != nil {
		return err
	}
	fmt.Println("CreateGraphStore: OK")
	// 只有明确创建成功才自动清理；中途失败也只删除本例自己的 Store，保留最初错误。
	defer func() {
		if cleanupErr := s.deleteStore(); cleanupErr != nil {
			if err == nil {
				err = cleanupErr
			} else {
				fmt.Fprintln(os.Stderr, "Cleanup failure:", failure(cleanupErr))
			}
		}
	}()
	if err = s.waitUntilReady(); err != nil {
		return err
	}
	size := 100
	stores, err := s.client.ListGraphStores(s.project, &sls.ListGraphStoresRequest{Size: &size})
	if err != nil {
		return err
	}
	// Project 可以已有其他 Store，不假设列表为空或本例资源位于第一页。
	fmt.Printf("ListGraphStores: page count=%d, total=%d\n", stores.Count, stores.Total)
	baseResponse, err := s.client.GetGraphStoreBaseSchema(s.project, s.store)
	if err != nil {
		return err
	}
	if baseResponse.BaseSchema.TTLSeconds != 86400 {
		return fmt.Errorf("unexpected Base TTL: %d", baseResponse.BaseSchema.TTLSeconds)
	}
	fmt.Printf("GetGraphStoreBaseSchema: TTL=%d\n", baseResponse.BaseSchema.TTLSeconds)
	person := sls.GraphStoreCollectionSchema{
		Name: "person", Description: "People", GraphLayout: sls.GraphStoreLayoutVertex,
		CollectionKeyValues: []sls.GraphStoreCollectionKeyValue{{Name: "vl", Value: "person"}},
		Fields:              []sls.GraphStoreFieldSchema{{Name: "name", Type: sls.GraphStoreFieldString}, {Name: "age", Type: sls.GraphStoreFieldInt}},
	}
	// 非 DELETE 边事件携带真实属性，满足当前 Worker 的非空属性要求；两 Collection 初始继承 Base TTL。
	knows := sls.GraphStoreCollectionSchema{
		Name: "knows", Description: "Person knows person", GraphLayout: sls.GraphStoreLayoutEdge,
		CollectionKeyValues: []sls.GraphStoreCollectionKeyValue{{Name: "sl", Value: "person"}, {Name: "el", Value: "knows"}, {Name: "tl", Value: "person"}},
		Fields:              []sls.GraphStoreFieldSchema{{Name: "weight", Type: sls.GraphStoreFieldDouble}},
	}
	for _, schema := range []*sls.GraphStoreCollectionSchema{&person, &knows} {
		if err = s.client.CreateGraphStoreCollectionSchema(s.project, s.store, schema); err != nil {
			return err
		}
		fmt.Println("CreateGraphStoreCollectionSchema:", schema.Name)
	}
	personResponse, err := s.client.GetGraphStoreCollectionSchema(s.project, s.store, "person")
	if err != nil {
		return err
	}
	fmt.Println("GetGraphStoreCollectionSchema:", personResponse.CollectionSchema.Name)
	// 分页 token 直接透传，直到服务端返回空 token；设置总时限，防止异常 token 导致无限循环。
	deadline := time.Now().Add(waitTimeout)
	for token := ""; ; {
		if !time.Now().Before(deadline) {
			return fmt.Errorf("timed out listing CollectionSchemas")
		}
		page, err := s.client.ListGraphStoreCollectionSchemas(s.project, s.store, 1, token)
		if err != nil {
			return err
		}
		for _, schema := range page.CollectionSchemas {
			fmt.Println("ListGraphStoreCollectionSchemas:", schema.Name)
		}
		token = page.NextPageToken
		if token == "" {
			break
		}
	}
	// shortcut: 当前 stg 在 READY 后仍需等待路由/Schema 缓存收敛；有公开可写保证后替换固定等待。
	fmt.Println("Waiting 65 seconds before the first write for stg cache convergence.")
	time.Sleep(65 * time.Second)
	if err = s.writeAndQuery(); err != nil {
		return err
	}
	// Update 是完整替换，必须保留原有字段；这里增加 email，TTL 从继承改为显式 3600 秒。
	fields := append(personResponse.CollectionSchema.Fields, sls.GraphStoreFieldSchema{Name: "email", Type: sls.GraphStoreFieldString})
	update := &sls.GraphStoreCollectionSchemaUpdate{Name: "person", Description: "People with email", TTLSeconds: proto.Int64(3600), Fields: fields}
	if err = s.client.UpdateGraphStoreCollectionSchema(s.project, s.store, "person", update); err != nil {
		return err
	}
	updated, err := s.client.GetGraphStoreCollectionSchema(s.project, s.store, "person")
	if err != nil {
		return err
	}
	got := updated.CollectionSchema
	fieldNames := map[string]bool{}
	for _, field := range got.Fields {
		fieldNames[field.Name] = true
	}
	if got.Description != update.Description || got.TTLSeconds == nil || *got.TTLSeconds != 3600 || len(got.Fields) != 3 || !fieldNames["name"] || !fieldNames["age"] || !fieldNames["email"] {
		return fmt.Errorf("CollectionSchema update readback mismatch")
	}
	fmt.Println("UpdateGraphStoreCollectionSchema: fields=name,age,email, TTL=3600 confirmed")
	for _, name := range []string{"knows", "person"} {
		if err = s.client.DeleteGraphStoreCollectionSchema(s.project, s.store, name); err != nil {
			return err
		}
		fmt.Println("DeleteGraphStoreCollectionSchema:", name)
	}
	return nil
}

func (s sample) writeAndQuery() error {
	if err := s.write(
		event("vertex", "create", "vl", "person", "vid", aliceID, "name", "Alice", "age", "30"),
		event("vertex", "create", "vl", "person", "vid", bobID, "name", "Bob", "age", "28"),
		event("edge", "create", "sl", "person", "sid", aliceID, "el", "knows", "tl", "person", "tid", bobID, "weight", "1.0"),
	); err != nil {
		return err
	}
	if err := s.waitForRows(personQuery, []string{"name", "age"}, [][]interface{}{{typedValue("String", "Alice"), typedValue("Integer", "30")}, {typedValue("String", "Bob"), typedValue("Integer", "28")}}); err != nil {
		return err
	}
	if err := s.waitForRows(edgeQuery, []string{"source", "target"}, [][]interface{}{{typedValue("String", "Alice"), typedValue("String", "Bob")}}); err != nil {
		return err
	}
	// UPDATE 保留完整属性；DELETE 只需标识目标的键字段。
	if err := s.write(event("vertex", "update", "vl", "person", "vid", aliceID, "name", "Alice", "age", "31")); err != nil {
		return err
	}
	if err := s.waitForRows(personQuery, []string{"name", "age"}, [][]interface{}{{typedValue("String", "Alice"), typedValue("Integer", "31")}, {typedValue("String", "Bob"), typedValue("Integer", "28")}}); err != nil {
		return err
	}
	if err := s.write(event("edge", "delete", "sl", "person", "sid", aliceID, "el", "knows", "tl", "person", "tid", bobID)); err != nil {
		return err
	}
	if err := s.waitForRows(edgeQuery, []string{"source", "target"}, [][]interface{}{}); err != nil {
		return err
	}
	if err := s.write(event("vertex", "delete", "vl", "person", "vid", aliceID), event("vertex", "delete", "vl", "person", "vid", bobID)); err != nil {
		return err
	}
	return s.waitForRows(personQuery, []string{"name", "age"}, [][]interface{}{})
}

func event(layout, method string, fields ...string) *sls.Log {
	log := &sls.Log{Time: proto.Uint32(uint32(time.Now().Unix()))}
	values := append([]string{"__layout__", layout, "__method__", method}, fields...)
	for i := 0; i < len(values); i += 2 {
		log.Contents = append(log.Contents, &sls.LogContent{Key: proto.String(values[i]), Value: proto.String(values[i+1])})
	}
	return log
}

func (s sample) write(logs ...*sls.Log) error {
	// 标准 PB LogGroup 交给原生 SDK 序列化、LZ4 压缩和签名；ACK 后仍须查询验证。
	if err := s.client.PostGraphStoreData(s.project, s.store, &sls.PostGraphStoreDataRequest{LogGroup: &sls.LogGroup{Logs: logs}, CompressType: sls.Compress_LZ4}); err != nil {
		return err
	}
	fmt.Printf("PostGraphStoreData: %d events acknowledged\n", len(logs))
	return nil
}

func (s sample) waitUntilReady() error {
	for deadline := time.Now().Add(waitTimeout); time.Now().Before(deadline); time.Sleep(time.Second) {
		response, err := s.client.GetGraphStore(s.project, s.store)
		if err != nil {
			return err
		}
		fmt.Printf("GetGraphStore: state=%s, shards=%d\n", response.State, len(response.Shards))
		if response.State == "READY" {
			return nil
		}
	}
	return fmt.Errorf("timed out waiting for Store READY")
}

func typedValue(kind string, value interface{}) map[string]interface{} {
	return map[string]interface{}{"$type": kind, "_value": value}
}

func (s sample) waitForRows(query string, fields []string, rows [][]interface{}) error {
	// ACK 不保证立即可读。比较完整 Typed cell，同时核对类型标签和值。
	request := sls.NewGetGraphStoreDataRequest(query)
	request.ResultFormat = "typed_json"
	for deadline := time.Now().Add(waitTimeout); time.Now().Before(deadline); time.Sleep(time.Second) {
		response, err := s.client.GetGraphStoreData(s.project, s.store, request)
		if err != nil {
			return err
		}
		fmt.Printf("GetGraphStoreData: requestId=%s, rows=%d, status=%s\n", response.RequestID, len(response.Data.Values), response.Status)
		if len(response.Meta) != 0 {
			fmt.Printf("Meta: %s\n", response.Meta)
		}
		if response.Status == "success" && reflect.DeepEqual(response.Data.Fields, fields) && reflect.DeepEqual(response.Data.Values, rows) {
			values, _ := json.Marshal(response.Data.Values)
			fmt.Printf("Query: %s\nRows: %s\n", query, values)
			return nil
		}
	}
	return fmt.Errorf("timed out waiting for query readback: %s", query)
}

func (s sample) deleteStore() error {
	if err := s.client.DeleteGraphStore(s.project, s.store); err != nil {
		return err
	}
	fmt.Println("DeleteGraphStore: OK")
	for deadline := time.Now().Add(waitTimeout); time.Now().Before(deadline); time.Sleep(time.Second) {
		_, err := s.client.GetGraphStore(s.project, s.store)
		if err != nil {
			var serviceError *sls.Error
			// 只有明确的 Store 不存在才表示清理完成，其他 404 或权限错误仍返回失败。
			if errors.As(err, &serviceError) && serviceError.HTTPCode == 404 && serviceError.Code == "GraphStoreNotExist" {
				fmt.Println("Store absent; requestId=" + serviceError.RequestID)
				return nil
			}
			return err
		}
	}
	return fmt.Errorf("timed out waiting for Store deletion")
}

func failure(err error) string {
	// SDK 错误仅打印状态、错误码和 request ID，不输出原始响应正文或认证信息。
	var serviceError *sls.Error
	if errors.As(err, &serviceError) {
		return fmt.Sprintf("SDK failure: httpStatus=%d, errorCode=%s, requestId=%s", serviceError.HTTPCode, serviceError.Code, serviceError.RequestID)
	}
	var badResponse *sls.BadResponseError
	if errors.As(err, &badResponse) {
		return fmt.Sprintf("SDK bad response: httpStatus=%d, requestId=%s", badResponse.HTTPCode, http.Header(badResponse.RespHeader).Get(sls.RequestIDHeader))
	}
	return fmt.Sprintf("Sample failure: %v", err)
}
