// Package sqllog 定时从 ProxySQL Advanced Events Logging 采集完整 SQL，
// 按 schemaname/hostgroup 映射到项目绑定库并存入 SQLite。
//
// Admin 通道：容器部署走 docker exec（Admin 只允许容器内本地连接），
// 二进制部署走 TCP 直连 Admin 端口。
package sqllog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dbgit/dbgit/internal/proxysql"
	"github.com/dbgit/dbgit/internal/store"
	_ "github.com/go-sql-driver/mysql"
)

// Options 采集器配置。
type Options struct {
	AdminAddr     string
	AdminUser     string
	AdminPassword string
	Interval      time.Duration
	// BufferSize eventslog 环形缓冲区大小（事件数）。
	BufferSize int
	// RetentionDays 变更记录保留天数。
	RetentionDays int
}

// Status 采集状态（供前端展示）。
type Status struct {
	Enabled         bool   `json:"enabled"`
	ProxyRunning    bool   `json:"proxy_running"`
	ProxyVersion    string `json:"proxy_version,omitempty"`
	Watermark       string `json:"watermark,omitempty"`
	LastCollectAt   string `json:"last_collect_at,omitempty"`
	LastScanned     int    `json:"last_scanned"`
	LastInserted    int64  `json:"last_inserted"`
	TotalStored     int64  `json:"total_stored"`
	DroppedEvents   *int64 `json:"dropped_events,omitempty"`
	LastError       string `json:"last_error,omitempty"`
	IntervalSeconds int    `json:"interval_seconds"`
	RetentionDays   int    `json:"retention_days"`
}

// Collector SQL 采集器。
type Collector struct {
	db   *store.DB
	opts Options

	mu     sync.Mutex
	status Status

	// watermark 已处理的最大事件时间（毫秒）；wmInit 为 false 时首轮从库里恢复。
	watermark int64
	wmInit    bool

	sampled bool
}

// New 创建采集器。
func New(db *store.DB, opts Options) *Collector {
	if opts.Interval <= 0 {
		opts.Interval = time.Minute
	}
	if opts.BufferSize <= 0 {
		opts.BufferSize = 200000
	}
	if opts.RetentionDays <= 0 {
		opts.RetentionDays = 30
	}
	return &Collector{
		db:   db,
		opts: opts,
		status: Status{
			IntervalSeconds: int(opts.Interval / time.Second),
			RetentionDays:   opts.RetentionDays,
		},
	}
}

// Status 返回当前采集状态（拷贝）。
func (c *Collector) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.status
	if c.watermark > 0 {
		s.Watermark = time.UnixMilli(c.watermark).UTC().Format(time.RFC3339)
	}
	return s
}

// Start 启动定时采集，直到 ctx 取消。
func (c *Collector) Start(ctx context.Context) {
	ticker := time.NewTicker(c.opts.Interval)
	defer ticker.Stop()
	if _, _, err := c.CollectOnce(ctx); err != nil {
		slog.Warn("SQL 采集初始执行失败", "err", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, _, err := c.CollectOnce(ctx); err != nil {
				slog.Warn("SQL 采集失败", "err", err)
			}
		}
	}
}

// CollectOnce 执行一次采集，返回（扫描行数，入库行数）。
func (c *Collector) CollectOnce(ctx context.Context) (scanned int, inserted int64, err error) {
	st := proxysql.Detect()
	c.mu.Lock()
	c.status.ProxyRunning = st.Running
	c.status.ProxyVersion = st.Version
	c.mu.Unlock()
	if !st.Installed || !st.Running {
		err = fmt.Errorf("ProxySQL 未运行，已跳过采集")
		c.setError(err.Error())
		return 0, 0, err
	}
	if st.Version != "" && !versionAtLeast(st.Version, 2, 7, 2) {
		err = fmt.Errorf("ProxySQL 版本过低（当前 %s），Advanced Events Logging 需要 ≥ 2.7.2", st.Version)
		c.setError(err.Error())
		return 0, 0, err
	}

	ch, err := c.openChannel(st)
	if err != nil {
		c.setError(err.Error())
		return 0, 0, err
	}
	if closer, ok := ch.(interface{ close() error }); ok {
		defer closer.close()
	}

	// 首轮从库里恢复水位（空表则为 0，本轮全量拉取内存表存量）。
	if !c.wmInit {
		if maxEt, err := c.db.MaxSQLChangeEventTime(); err == nil {
			c.watermark = maxEt
		}
		c.wmInit = true
	}

	if err := ch.ensureLogging(ctx, c.opts.BufferSize); err != nil {
		c.setError(err.Error())
		return 0, 0, err
	}
	if err := ch.dumpEvents(ctx); err != nil {
		c.setError(err.Error())
		return 0, 0, err
	}

	rows, err := ch.fetchEvents(ctx, c.windowSinceUs())
	if err != nil {
		c.setError(err.Error())
		return 0, 0, err
	}
	scanned = len(rows)
	if scanned == 0 {
		c.setSuccess(0, 0)
		c.cleanup()
		return 0, 0, nil
	}

	bindings, hgMap, err := c.bindingIndex()
	if err != nil {
		c.setError("读取绑定关系失败: " + err.Error())
		return scanned, 0, err
	}

	now := time.Now()
	changes := make([]store.SQLChange, 0, len(rows))
	dedups := make([]string, 0, len(rows))
	eventMs := make([]int64, 0, len(rows))
	maxEt := c.watermark
	for _, r := range rows {
		et := rowEventTime(r, now)
		if et > maxEt {
			maxEt = et
		}
		chg, ok := c.translate(r, bindings, hgMap, et)
		if !ok {
			continue
		}
		changes = append(changes, chg)
		dedups = append(dedups, dedupKey(et, r))
		eventMs = append(eventMs, et)
	}
	// 水位按本轮见到的最大事件时间推进（无论是否入库成功映射），
	// 漏网靠重叠窗口 + 哈希去重兜底，保证 at-least-once。
	if maxEt > c.watermark {
		c.watermark = maxEt
	}

	inserted, err = c.db.InsertSQLChanges(changes, dedups, eventMs)
	if err != nil {
		c.setError("变更记录入库失败: " + err.Error())
		return scanned, 0, err
	}
	c.setSuccess(scanned, inserted)
	c.cleanup()
	if n, ok := ch.fetchDropped(ctx); ok {
		c.mu.Lock()
		c.status.DroppedEvents = &n
		c.mu.Unlock()
	}
	return scanned, inserted, nil
}

func (c *Collector) setError(msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status.LastError = msg
}

func (c *Collector) setSuccess(scanned int, inserted int64) {
	total, err := c.db.CountSQLStatements()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status.Enabled = true
	c.status.LastCollectAt = time.Now().UTC().Format(time.RFC3339)
	c.status.LastScanned = scanned
	c.status.LastInserted = inserted
	c.status.LastError = ""
	if err == nil {
		c.status.TotalStored = total
	}
}

// cleanup 按保留期清理过期记录（失败只记日志）。
func (c *Collector) cleanup() {
	if c.opts.RetentionDays <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -c.opts.RetentionDays).UnixMilli()
	if n, err := c.db.DeleteSQLChangesBefore(cutoff); err != nil {
		slog.Warn("清理过期变更记录失败", "err", err)
	} else if n > 0 {
		slog.Info("清理过期变更记录", "rows", n)
	}
}

// windowSinceUs 本轮拉取下限（微秒）：水位回退一个重叠窗口防压线遗漏。
// 重叠取 max(60秒, 2倍采集间隔)，0 水位表示全量拉取内存表存量。
func (c *Collector) windowSinceUs() int64 {
	if c.watermark <= 0 {
		return 0
	}
	overlapMs := int64(60000)
	if iv := int64(c.opts.Interval / time.Millisecond); iv*2 > overlapMs {
		overlapMs = iv * 2
	}
	floorMs := c.watermark - overlapMs
	if floorMs < 0 {
		floorMs = 0
	}
	return floorMs * 1000
}

// adminChannel 抽象两种 Admin 访问方式：容器部署走 docker exec，二进制部署走 TCP。
type adminChannel interface {
	ensureLogging(ctx context.Context, bufferSize int) error
	dumpEvents(ctx context.Context) error
	// fetchEvents 拉取内存 events 表全部事件（列名统一小写）。
	// sinceUs > 0 时仅拉取 start_time 之后的行（水位增量）。
	fetchEvents(ctx context.Context, sinceUs int64) ([]map[string]string, error)
	// fetchDropped 读取环形缓冲区丢弃计数（不支持时返回 ok=false）。
	fetchDropped(ctx context.Context) (int64, bool)
}

func (c *Collector) openChannel(st proxysql.Status) (adminChannel, error) {
	if container := proxysql.ContainerName(); container != "" && st.Running {
		return &execChannel{container: container, user: c.opts.AdminUser, pass: c.opts.AdminPassword}, nil
	}
	addr := c.opts.AdminAddr
	if addr == "" {
		addr = "127.0.0.1:6032"
	}
	if st.AdminPort > 0 {
		addr = fmt.Sprintf("127.0.0.1:%d", st.AdminPort)
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/?timeout=5s&readTimeout=15s&writeTimeout=5s",
		url.QueryEscape(c.opts.AdminUser), url.QueryEscape(c.opts.AdminPassword), addr)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("连接 ProxySQL Admin 失败: %w", err)
	}
	return &tcpChannel{db: db}, nil
}

func ensureStmts(bufferSize int) []string {
	return []string{
		fmt.Sprintf(`SET mysql-eventslog_buffer_history_size=%d`, bufferSize),
		`SET mysql-eventslog_default_log=1`,
		`LOAD MYSQL VARIABLES TO RUNTIME`,
	}
}

// eventsTables 内存 events 表候选位置（版本差异兼容）。
func eventsTables() []string {
	return []string{
		`stats.stats_mysql_query_events`,
		`stats_mysql_query_events`,
	}
}

// buildHexSelect 构造 HEX 取数查询：SQL 文本常含制表符/换行，HEX 后再按 TSV 解析才安全。
func buildHexSelect(table string, cols []string) string {
	var b strings.Builder
	b.WriteString("SELECT ")
	for i, col := range cols {
		if i > 0 {
			b.WriteString(", ")
		}
		name := strings.ReplaceAll(col, `"`, `""`)
		fmt.Fprintf(&b, `COALESCE(HEX("%s"),'')`, name)
	}
	fmt.Fprintf(&b, " FROM %s", table)
	return b.String()
}

func splitTSV(out string) [][]string {
	var rows [][]string
	for _, line := range strings.Split(out, "\n") {
		// 去掉行尾 \r（mysql 客户端输出 \r\n）；防御性丢弃 WARNING 行。
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, "WARNING:") {
			continue
		}
		rows = append(rows, strings.Split(line, "\t"))
	}
	return rows
}

func decodeHexRow(cells []string) ([]string, error) {
	out := make([]string, len(cells))
	for i, h := range cells {
		if h == "" {
			continue
		}
		b, err := hex.DecodeString(h)
		if err != nil {
			return nil, fmt.Errorf("HEX 解码失败: %w", err)
		}
		out[i] = string(b)
	}
	return out, nil
}

// execChannel 经 docker exec 在容器内执行 admin SQL。
type execChannel struct {
	container string
	user      string
	pass      string
}

func (ch *execChannel) ensureLogging(ctx context.Context, bufferSize int) error {
	if err := proxysql.ExecAdmin(ctx, ch.container, ch.user, ch.pass, "", ensureStmts(bufferSize)...); err != nil {
		return fmt.Errorf("开启 eventslog 失败: %w", err)
	}
	return nil
}

func (ch *execChannel) dumpEvents(ctx context.Context) error {
	if err := proxysql.ExecAdmin(ctx, ch.container, ch.user, ch.pass, "", `DUMP EVENTSLOG FROM BUFFER TO MEMORY`); err != nil {
		return fmt.Errorf("DUMP EVENTSLOG 失败（需要 ProxySQL ≥ 2.7.2）: %w", err)
	}
	return nil
}

func (ch *execChannel) fetchEvents(ctx context.Context, sinceUs int64) ([]map[string]string, error) {
	var lastErr error
	for _, table := range eventsTables() {
		cols, rows, err := ch.fetchTable(ctx, table, sinceUs)
		if err != nil {
			lastErr = err
			continue
		}
		out := make([]map[string]string, 0, len(rows))
		for _, r := range rows {
			m := make(map[string]string, len(cols))
			for i, col := range cols {
				if i < len(r) {
					m[strings.ToLower(col)] = r[i]
				}
			}
			out = append(out, m)
		}
		return out, nil
	}
	return nil, fmt.Errorf("eventslog 事件表不可用（需要 ProxySQL ≥ 2.7.2 且已开启 eventslog）: %v", lastErr)
}

// windowWhere 增量窗口条件（列不存在的老版本回退全量，由调用方处理错误）。
func windowWhere(sinceUs int64) string {
	if sinceUs <= 0 {
		return ""
	}
	return fmt.Sprintf(" WHERE start_time > %d", sinceUs)
}

func (ch *execChannel) fetchTable(ctx context.Context, table string, sinceUs int64) ([]string, [][]string, error) {
	// 注意：经此链路 LIMIT 0 不返回表头，用 LIMIT 1 取表头；窗口内无行直接返回空。
	head, err := proxysql.QueryAdminRaw(ctx, ch.container, ch.user, ch.pass, "",
		`SELECT * FROM `+table+windowWhere(sinceUs)+` LIMIT 1`, false)
	if err != nil && sinceUs > 0 {
		// 疑似无 start_time 列的老版本：回退全量拉取。
		sinceUs = 0
		head, err = proxysql.QueryAdminRaw(ctx, ch.container, ch.user, ch.pass, "",
			`SELECT * FROM `+table+` LIMIT 1`, false)
	}
	if err != nil {
		return nil, nil, err
	}
	header := splitTSV(head)
	if len(header) == 0 {
		return nil, nil, nil // 窗口内无事件
	}
	cols := header[0]
	data, err := proxysql.QueryAdminRaw(ctx, ch.container, ch.user, ch.pass, "",
		buildHexSelect(table+windowWhere(sinceUs), cols), true)
	if err != nil {
		return nil, nil, err
	}
	var rows [][]string
	for _, cells := range splitTSV(data) {
		dec, err := decodeHexRow(cells)
		if err != nil {
			return nil, nil, err
		}
		rows = append(rows, dec)
	}
	return cols, rows, nil
}

func (ch *execChannel) fetchDropped(ctx context.Context) (int64, bool) {
	for _, table := range []string{`stats.stats_mysql_global`, `stats_mysql_global`} {
		out, err := proxysql.QueryAdminRaw(ctx, ch.container, ch.user, ch.pass, "",
			`SELECT Variable_Value FROM `+table+` WHERE Variable_Name = 'MySQL_Logger-circularBufferEventsDroppedCount'`, true)
		if err != nil {
			continue
		}
		if lines := splitTSV(out); len(lines) > 0 && len(lines[0]) > 0 {
			if n, err := strconv.ParseInt(strings.TrimSpace(lines[0][0]), 10, 64); err == nil {
				return n, true
			}
		}
	}
	return 0, false
}

// tcpChannel 直连 Admin 端口（仅二进制部署可用；容器部署的 Admin 只允许本地连接）。
type tcpChannel struct {
	db *sql.DB
}

func (ch *tcpChannel) close() error { return ch.db.Close() }

func (ch *tcpChannel) ensureLogging(ctx context.Context, bufferSize int) error {
	for _, s := range ensureStmts(bufferSize) {
		if _, err := ch.db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("开启 eventslog 失败（%s）: %w", s, err)
		}
	}
	return nil
}

func (ch *tcpChannel) dumpEvents(ctx context.Context) error {
	if _, err := ch.db.ExecContext(ctx, `DUMP EVENTSLOG FROM BUFFER TO MEMORY`); err != nil {
		return fmt.Errorf("DUMP EVENTSLOG 失败（需要 ProxySQL ≥ 2.7.2）: %w", err)
	}
	return nil
}

func (ch *tcpChannel) fetchEvents(ctx context.Context, sinceUs int64) ([]map[string]string, error) {
	var lastErr error
	for _, table := range eventsTables() {
		cols, hexRows, err := ch.fetchTable(ctx, table, sinceUs)
		if err != nil {
			lastErr = err
			continue
		}
		out := make([]map[string]string, 0, len(hexRows))
		for _, r := range hexRows {
			m := make(map[string]string, len(cols))
			for i, col := range cols {
				if i < len(r) {
					m[strings.ToLower(col)] = r[i]
				}
			}
			out = append(out, m)
		}
		return out, nil
	}
	return nil, fmt.Errorf("eventslog 事件表不可用（需要 ProxySQL ≥ 2.7.2 且已开启 eventslog）: %v", lastErr)
}

func (ch *tcpChannel) fetchTable(ctx context.Context, table string, sinceUs int64) ([]string, [][]string, error) {
	rows, err := ch.db.QueryContext(ctx, `SELECT * FROM `+table+` LIMIT 0`)
	if err != nil {
		return nil, nil, err
	}
	cols, err := rows.Columns()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	if len(cols) == 0 {
		return nil, nil, fmt.Errorf("无法识别事件表结构: %s", table)
	}
	query := buildHexSelect(table, cols)
	if sinceUs > 0 {
		// WHERE 下推失败（疑似无 start_time 列）则回退全量。
		if _, err := ch.db.ExecContext(ctx, `SELECT 1 FROM `+table+` WHERE start_time > 0 LIMIT 0`); err == nil {
			query = buildHexSelect(table+windowWhere(sinceUs), cols)
		}
	}
	data, err := ch.db.QueryContext(ctx, query)
	if err != nil {
		return nil, nil, err
	}
	defer data.Close()
	var out [][]string
	n := len(cols)
	for data.Next() {
		vals := make([]any, n)
		ptrs := make([]any, n)
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := data.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		row := make([]string, n)
		for i, v := range vals {
			row[i] = asCellString(v)
		}
		dec, err := decodeHexRow(row)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, dec)
	}
	return cols, out, data.Err()
}

func (ch *tcpChannel) fetchDropped(ctx context.Context) (int64, bool) {
	for _, q := range []string{
		`SELECT Variable_Value FROM stats.stats_mysql_global WHERE Variable_Name = 'MySQL_Logger-circularBufferEventsDroppedCount'`,
		`SELECT Variable_Value FROM stats_mysql_global WHERE Variable_Name = 'MySQL_Logger-circularBufferEventsDroppedCount'`,
	} {
		var v string
		if err := ch.db.QueryRowContext(ctx, q).Scan(&v); err != nil {
			continue
		}
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return n, true
		}
	}
	return 0, false
}

// asCellString 将驱动返回的单元格转为字符串（HEX 查询结果为文本/字节）。
func asCellString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprintf("%v", t)
	}
}

// ----------------------------------------------------------------------------
// 行转换与映射
// ----------------------------------------------------------------------------

// bindingIndex 构建映射索引：hostgroup→instance，以及全部绑定列表。
func (c *Collector) bindingIndex() ([]store.EnvDB, map[int]int64, error) {
	tree, err := c.db.GetProjectTree()
	if err != nil {
		return nil, nil, err
	}
	var bindings []store.EnvDB
	for _, p := range tree {
		for _, e := range p.Envs {
			bindings = append(bindings, e.DBs...)
		}
	}
	hgMap := make(map[int]int64)
	views, err := c.db.ListAllProxyBackendViews()
	if err != nil {
		return nil, nil, err
	}
	for _, v := range views {
		if v.HostgroupID != nil {
			hgMap[*v.HostgroupID] = v.InstanceID
		}
	}
	return bindings, hgMap, nil
}

// rowEventTime 提取行的事件时间（毫秒），缺失时回退为当前时间。
func rowEventTime(r map[string]string, now time.Time) int64 {
	return parseEventTime(firstNonEmpty(r,
		"event_time", "timestamp", "timestamp_us", "timestamp_ms", "time_us", "time_ms", "event_timestamp",
		"start_time", "start"), now)
}

// translate 将一条 eventslog 行转为入库记录；不符合收录条件的返回 ok=false。
func (c *Collector) translate(
	r map[string]string,
	bindings []store.EnvDB,
	hgMap map[int]int64,
	eventTime int64,
) (store.SQLChange, bool) {
	sqlText := strings.TrimSpace(firstNonEmpty(r, "query", "sql_text", "digest_text", "sql", "statement", "query_text"))
	if sqlText == "" {
		return store.SQLChange{}, false
	}
	stmtType := classifyStmt(sqlText)
	if stmtType == "" {
		return store.SQLChange{}, false // 只收录变更语句
	}
	schema := firstNonEmpty(r, "schemaname", "schema", "db", "database")
	hg := firstInt(r, "hostgroup", "hostgroup_id", "hg_id", "hid")

	bindingID := int64(0)
	if instID, ok := hgMap[hg]; ok && hg > 0 {
		for _, b := range bindings {
			if b.InstanceID == instID && strings.EqualFold(b.DBName, schema) {
				bindingID = b.ID
				break
			}
		}
	}
	if bindingID == 0 && schema != "" {
		var matched []store.EnvDB
		for _, b := range bindings {
			if strings.EqualFold(b.DBName, schema) {
				matched = append(matched, b)
			}
		}
		// 库名全局唯一时才能无歧义归属。
		if len(matched) == 1 {
			bindingID = matched[0].ID
		}
	}
	if bindingID == 0 {
		return store.SQLChange{}, false
	}

	duration := firstInt64(r, "duration_us", "duration", "exec_time_us", "query_time_us")
	if duration == 0 {
		// Advanced Events Logging 用 start_time/end_time（微秒）表达耗时。
		if s, e := firstInt64(r, "start_time", "start"), firstInt64(r, "end_time", "end"); s > 0 && e >= s && e-s < 3_600_000_000 {
			duration = e - s
		}
	}

	if !c.sampled {
		c.sampled = true
		keys := make([]string, 0, len(r))
		for k := range r {
			keys = append(keys, k)
		}
		slog.Debug("eventslog 首行列", "columns", strings.Join(keys, ","))
	}

	return store.SQLChange{
		BindingID:    bindingID,
		StmtType:     stmtType,
		SQLText:      sqlText,
		Username:     firstNonEmpty(r, "username", "user", "client_username"),
		ClientAddr:   firstNonEmpty(r, "client_address", "client_addr", "client_host", "client_ip", "client"),
		Schemaname:   schema,
		HostgroupID:  hg,
		DurationUs:   duration,
		RowsAffected: firstInt64(r, "rows_affected", "affected_rows", "rows_sent"),
	}, true
}

func firstNonEmpty(r map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(r[k]); v != "" {
			return v
		}
	}
	return ""
}

func firstInt(r map[string]string, keys ...string) int {
	for _, k := range keys {
		if v := strings.TrimSpace(r[k]); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				return n
			}
		}
	}
	return 0
}

func firstInt64(r map[string]string, keys ...string) int64 {
	for _, k := range keys {
		if v := strings.TrimSpace(r[k]); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				return n
			}
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return int64(f)
			}
		}
	}
	return 0
}

// parseEventTime 兼容多种时间表示，统一转为毫秒时间戳。
func parseEventTime(s string, now time.Time) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return now.UnixMilli()
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return normalizeEpoch(n)
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return normalizeEpoch(int64(f))
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
		time.RFC3339,
	} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t.UnixMilli()
		}
	}
	return now.UnixMilli()
}

// normalizeEpoch 按量级判断秒/毫秒/微秒/纳秒并转为毫秒。
func normalizeEpoch(n int64) int64 {
	switch {
	case n >= 1e17:
		return n / 1e6 // 纳秒
	case n >= 1e14:
		return n / 1e3 // 微秒
	case n >= 1e11:
		return n // 毫秒
	case n >= 1e9:
		return n * 1e3 // 秒
	default:
		return time.Now().UnixMilli()
	}
}

// classifyStmt 判定语句类型；返回 "" 表示非变更语句（不收录）。
func classifyStmt(sqlText string) string {
	s := strings.TrimSpace(sqlText)
	// 跳过前导注释与括号。
	for {
		upper := strings.ToUpper(s)
		switch {
		case strings.HasPrefix(upper, "/*"):
			if i := strings.Index(s, "*/"); i >= 0 {
				s = strings.TrimSpace(s[i+2:])
				continue
			}
			return ""
		case strings.HasPrefix(upper, "--"):
			if i := strings.Index(s, "\n"); i >= 0 {
				s = strings.TrimSpace(s[i+1:])
				continue
			}
			return ""
		case strings.HasPrefix(s, "#"):
			if i := strings.Index(s, "\n"); i >= 0 {
				s = strings.TrimSpace(s[i+1:])
				continue
			}
			return ""
		case strings.HasPrefix(s, "("):
			s = strings.TrimSpace(s[1:])
			continue
		}
		break
	}
	fields := strings.Fields(strings.ToUpper(s))
	if len(fields) == 0 {
		return ""
	}
	first := fields[0]
	// 只读 / 会话控制语句不收录。
	switch first {
	case "SELECT", "SHOW", "DESCRIBE", "DESC", "EXPLAIN", "SET", "USE",
		"BEGIN", "START", "COMMIT", "ROLLBACK", "SAVEPOINT", "RELEASE",
		"LOCK", "UNLOCK", "KILL", "DO", "HANDLER", "HELP", "RESET", "ANALYZE":
		return ""
	}
	if len(first) > 16 {
		return "OTHER"
	}
	return first
}

func dedupKey(eventTime int64, r map[string]string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%d|%s|%s|%s|%s|%s",
		eventTime,
		r["username"], r["schemaname"], r["hostgroup"],
		firstNonEmpty(r, "query", "sql_text", "digest_text", "sql"),
		firstNonEmpty(r, "duration_us", "duration"),
	)
	return hex.EncodeToString(h.Sum(nil))
}

// versionAtLeast 比较 X.Y.Z 版本号；解析失败时返回 true（交给表探测决定）。
func versionAtLeast(v string, major, minor, patch int) bool {
	v = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(v), "v"))
	parts := strings.Split(v, ".")
	if len(parts) < 3 {
		return true
	}
	nums := make([]int, 3)
	for i := 0; i < 3; i++ {
		digits := ""
		for _, ch := range parts[i] {
			if ch < '0' || ch > '9' {
				break
			}
			digits += string(ch)
		}
		if digits == "" {
			return true
		}
		n, err := strconv.Atoi(digits)
		if err != nil {
			return true
		}
		nums[i] = n
	}
	want := []int{major, minor, patch}
	for i := 0; i < 3; i++ {
		if nums[i] != want[i] {
			return nums[i] > want[i]
		}
	}
	return true
}
