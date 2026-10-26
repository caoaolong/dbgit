package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/dbgit/dbgit/internal/store"
	"github.com/gin-gonic/gin"
)

func setupMergeTest(t *testing.T) (*store.DB, *SyncMerges, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := store.Open(filepath.Join(t.TempDir(), "merge_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	h := NewSyncMerges(db)
	r := gin.New()
	r.POST("/api/v1/bindings/:id/merges", h.CreateMerge)
	r.POST("/api/v1/bindings/:id/clone", h.CreateClone)
	r.GET("/api/v1/projects/:id/sync-graph", h.SyncGraph)
	sc := NewSyncConfig(db)
	r.GET("/api/v1/projects/:id/sync-config", sc.GetSyncConfig)
	r.PUT("/api/v1/projects/:id/sync-config", sc.UpdateSyncConfig)
	return db, h, r
}

func seedMergeProject(t *testing.T, db *store.DB) (projectID, devID, testID, devBindingID int64) {
	t.Helper()
	p, err := db.CreateProject("商城", "")
	if err != nil {
		t.Fatal(err)
	}
	dev, err := db.CreateEnv(p.ID, "开发", "")
	if err != nil {
		t.Fatal(err)
	}
	test, err := db.CreateEnv(p.ID, "测试", "")
	if err != nil {
		t.Fatal(err)
	}
	ins1, err := db.CreateInstance(store.Instance{Name: "dev-ins", DBType: "mysql", Host: "127.0.0.1", Port: 3306})
	if err != nil {
		t.Fatal(err)
	}
	ins2, err := db.CreateInstance(store.Instance{Name: "test-ins", DBType: "mysql", Host: "127.0.0.1", Port: 3307})
	if err != nil {
		t.Fatal(err)
	}
	bDev, err := db.CreateEnvDB(dev.ID, ins1.ID, "shop_dev", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateEnvDB(test.ID, ins2.ID, "shop_test", ""); err != nil {
		t.Fatal(err)
	}
	// 同名库：合并时应自动解析为目标绑定。
	if _, err := db.CreateEnvDB(test.ID, ins2.ID, "shop_dev", ""); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	n, err := db.InsertSQLChanges(
		[]store.SQLChange{
			{BindingID: bDev.ID, StmtType: "INSERT", SQLText: "INSERT INTO orders VALUES (1)", Username: "app", Schemaname: "shop_dev"},
			{BindingID: bDev.ID, StmtType: "UPDATE", SQLText: "UPDATE orders SET a=1", Username: "app", Schemaname: "shop_dev"},
		},
		[]string{"t1", "t2"},
		[]int64{now - 2000, now - 1000},
	)
	if err != nil || n != 2 {
		t.Fatalf("seed changes = %d, err = %v", n, err)
	}
	return p.ID, dev.ID, test.ID, bDev.ID
}

func doRequest(t *testing.T, r *gin.Engine, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, _ := http.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应不是 JSON: %s", w.Body.String())
	}
	return w.Code, out
}

// TestMergeDevToTest 用测试代码把开发环境的库合并到测试环境。
func TestMergeDevToTest(t *testing.T) {
	db, _, r := setupMergeTest(t)
	_, devID, testID, devBindingID := seedMergeProject(t, db)

	// 正常合并：开发 → 测试。
	code, out := doRequest(t, r, "POST",
		"/api/v1/bindings/"+itoa(devBindingID)+"/merges",
		map[string]any{"target_env_id": testID, "comment": "开发同步到测试"})
	if code != http.StatusOK || out["code"] != float64(0) {
		t.Fatalf("合并失败: http=%d body=%v", code, out)
	}
	data := out["data"].(map[string]any)
	if data["stmt_count"] != float64(2) {
		t.Fatalf("stmt_count = %v, want 2", data["stmt_count"])
	}
	if data["target_binding_id"] == float64(0) {
		t.Fatalf("同名库 shop_dev 应被解析为目标绑定，got 0")
	}
	if data["has_base"] != true {
		t.Fatalf("同名库合并应有共同基线: %v", data)
	}
	// 合并应在源分支和目标分支各落一条 MERGE 提交。
	srcChanges, err := db.ListSQLChanges(devBindingID, store.SQLChangeFilter{StmtType: "MERGE"})
	if err != nil || len(srcChanges) != 1 {
		t.Fatalf("源分支应有 1 条 MERGE 提交: %+v, err=%v", srcChanges, err)
	}
	tgtBindingID := int64(data["target_binding_id"].(float64))
	tgtChanges, err := db.ListSQLChanges(tgtBindingID, store.SQLChangeFilter{StmtType: "MERGE"})
	if err != nil || len(tgtChanges) != 1 {
		t.Fatalf("目标分支应有 1 条 MERGE 提交: %+v, err=%v", tgtChanges, err)
	}

	// 分支树应包含这次合并。
	_, out = doRequest(t, r, "GET", "/api/v1/projects/1/sync-graph?per_binding=10", nil)
	gdata := out["data"].(map[string]any)
	merges := gdata["merges"].([]any)
	if len(merges) != 1 {
		t.Fatalf("merges = %d, want 1", len(merges))
	}
	m := merges[0].(map[string]any)
	if m["comment"] != "开发同步到测试" || m["target_env_name"] != "测试" {
		t.Fatalf("merge = %v", m)
	}

	// 非法用例：目标与来源相同。
	_, out = doRequest(t, r, "POST",
		"/api/v1/bindings/"+itoa(devBindingID)+"/merges",
		map[string]any{"target_env_id": devID, "comment": "x"})
	if out["code"] == float64(0) {
		t.Fatalf("同环境合并应被拒绝")
	}

	// 非法用例：缺少说明。
	_, out = doRequest(t, r, "POST",
		"/api/v1/bindings/"+itoa(devBindingID)+"/merges",
		map[string]any{"target_env_id": testID, "comment": "  "})
	if out["code"] == float64(0) {
		t.Fatalf("空说明应被拒绝")
	}

	// 非法用例：跨项目环境。
	other, err := db.CreateProject("其他", "")
	if err != nil {
		t.Fatal(err)
	}
	otherEnv, err := db.CreateEnv(other.ID, "生产", "")
	if err != nil {
		t.Fatal(err)
	}
	_, out = doRequest(t, r, "POST",
		"/api/v1/bindings/"+itoa(devBindingID)+"/merges",
		map[string]any{"target_env_id": otherEnv.ID, "comment": "x"})
	if out["code"] == float64(0) {
		t.Fatalf("跨项目合并应被拒绝")
	}
}

// TestCloneDevToStaging 用测试代码把开发环境的库克隆到空环境，再合并验证共同基线。
func TestCloneDevToStaging(t *testing.T) {
	db, _, r := setupMergeTest(t)
	projectID, devID, testID, devBindingID := seedMergeProject(t, db)

	// 同名库直接覆盖（不再拒绝）。
	var sameNameID int64
	testDBs, err := db.ListEnvDBs(testID)
	if err != nil {
		t.Fatal(err)
	}
	var testInstanceID int64
	for _, b := range testDBs {
		testInstanceID = b.InstanceID
		if b.DBName == "shop_dev" {
			sameNameID = b.ID
		}
	}
	if sameNameID == 0 {
		t.Fatalf("测试环境应有 shop_dev")
	}
	code, out := doRequest(t, r, "POST",
		"/api/v1/bindings/"+itoa(devBindingID)+"/clone",
		map[string]any{"target_env_id": testID, "target_binding_id": sameNameID})
	if code != http.StatusOK || out["code"] != float64(0) {
		t.Fatalf("同名库覆盖式克隆应成功: http=%d body=%v", code, out)
	}
	if out["data"].(map[string]any)["created"] != false {
		t.Fatalf("覆盖式克隆不应新建绑定: %v", out["data"])
	}

	// 目标有多个库且无同名时，未明确指定目标应被拒绝。
	otherSrc, err := db.CreateEnvDB(devID, testInstanceID, "cms_dev", "")
	if err != nil {
		t.Fatal(err)
	}
	_, out = doRequest(t, r, "POST",
		"/api/v1/bindings/"+itoa(otherSrc.ID)+"/clone",
		map[string]any{"target_env_id": testID})
	if out["code"] == float64(0) {
		t.Fatalf("目标有多个库且未明确指定时应被拒绝")
	}
	_ = otherSrc

	// 新建空环境（预发布），把开发库克隆过去。
	staging, err := db.CreateEnv(projectID, "预发布", "")
	if err != nil {
		t.Fatal(err)
	}
	code, out2 := doRequest(t, r, "POST",
		"/api/v1/bindings/"+itoa(devBindingID)+"/clone",
		map[string]any{"target_env_id": staging.ID, "comment": "建基线"})
	if code != http.StatusOK || out2["code"] != float64(0) {
		t.Fatalf("克隆失败: http=%d body=%v", code, out2)
	}
	cdata := out2["data"].(map[string]any)
	if cdata["target_binding_id"] == float64(0) {
		t.Fatalf("克隆应创建目标绑定")
	}
	tgtBindingID := int64(cdata["target_binding_id"].(float64))
	// 克隆应在目标分支落一条 CLONE 提交记录。
	changes, err := db.ListSQLChanges(tgtBindingID, store.SQLChangeFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].StmtType != "CLONE" {
		t.Fatalf("目标分支应有 1 条 CLONE 提交，got %+v", changes)
	}
	// 目标环境应多出同名库。
	found := false
	if dbs, err := db.ListEnvDBs(staging.ID); err != nil {
		t.Fatal(err)
	} else {
		for _, b := range dbs {
			if b.DBName == "shop_dev" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("目标环境缺少克隆出的同名库")
	}

	// 基于克隆基线合并：开发 → 预发布，应标记有共同基线。
	_, out = doRequest(t, r, "POST",
		"/api/v1/bindings/"+itoa(devBindingID)+"/merges",
		map[string]any{"target_env_id": staging.ID, "comment": "基线合并"})
	if out["code"] != float64(0) {
		t.Fatalf("合并失败: %v", out)
	}
	if out["data"].(map[string]any)["has_base"] != true {
		t.Fatalf("克隆后合并应有共同基线: %v", out["data"])
	}

	// 分支树应包含这次克隆。
	_, out = doRequest(t, r, "GET", "/api/v1/projects/1/sync-graph?per_binding=10", nil)
	gdata := out["data"].(map[string]any)
	clones, ok := gdata["clones"].([]any)
	if !ok || len(clones) != 2 {
		t.Fatalf("clones = %v", gdata["clones"])
	}
	foundStaging := false
	for _, cc := range clones {
		if cc.(map[string]any)["target_env_name"] == "预发布" {
			foundStaging = true
		}
	}
	if !foundStaging {
		t.Fatalf("分支树缺少预发布的克隆记录: %v", clones)
	}

	// 非法用例：跨项目克隆。
	other, err := db.CreateProject("其他", "")
	if err != nil {
		t.Fatal(err)
	}
	otherEnv, err := db.CreateEnv(other.ID, "生产", "")
	if err != nil {
		t.Fatal(err)
	}
	_, out = doRequest(t, r, "POST",
		"/api/v1/bindings/"+itoa(devBindingID)+"/clone",
		map[string]any{"target_env_id": otherEnv.ID})
	if out["code"] == float64(0) {
		t.Fatalf("跨项目克隆应被拒绝")
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// TestSyncConfigRevision 保存同步配置时追加版本记录，分支树据此生成提交。
func TestSyncConfigRevision(t *testing.T) {
	db, _, r := setupMergeTest(t)
	seedMergeProject(t, db)

	code, out := doRequest(t, r, "PUT", "/api/v1/projects/1/sync-config",
		map[string]any{"content": "orders\ntmp_*:DELETE", "comment": "首版忽略"})
	if code != http.StatusOK || out["code"] != float64(0) {
		t.Fatalf("保存失败: http=%d body=%v", code, out)
	}
	if out["data"].(map[string]any)["revision_id"] == float64(0) {
		t.Fatalf("应返回 revision_id: %v", out)
	}
	code, out = doRequest(t, r, "PUT", "/api/v1/projects/1/sync-config",
		map[string]any{"content": "orders"})
	if code != http.StatusOK || out["code"] != float64(0) {
		t.Fatalf("保存失败: http=%d body=%v", code, out)
	}

	_, out = doRequest(t, r, "GET", "/api/v1/projects/1/sync-graph?per_binding=1", nil)
	cc, ok := out["data"].(map[string]any)["config_changes"].([]any)
	if !ok || len(cc) != 2 {
		t.Fatalf("config_changes = %v", out["data"])
	}
	if cc[0].(map[string]any)["rule_count"] != float64(2) {
		t.Fatalf("第一版应有 2 条规则: %v", cc[0])
	}
}
