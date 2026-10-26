// Package syncignore 解析与匹配同步忽略规则。
//
// 规则格式（类 .gitignore，每行一条）：
//
//	# 以 # 开头的行为注释，空行忽略
//	orders                  # 忽略 orders 表的所有同步操作
//	orders:INSERT,UPDATE    # 忽略 orders 表的指定操作（逗号分隔，不区分大小写）
//	*:DELETE                # 所有表忽略 DELETE（慎用）
//
// 操作取值：INSERT UPDATE DELETE REPLACE CREATE ALTER DROP TRUNCATE，
// DDL 是 CREATE/ALTER/DROP/TRUNCATE 的简写，ALL 表示全部操作。
package syncignore

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// 有效操作（含简写，解析时展开）。
var validOps = map[string]bool{
	"INSERT": true, "UPDATE": true, "DELETE": true, "REPLACE": true,
	"CREATE": true, "ALTER": true, "DROP": true, "TRUNCATE": true,
}

var ddlOps = []string{"CREATE", "ALTER", "DROP", "TRUNCATE"}

var tableRe = regexp.MustCompile(`^[\p{L}\p{N}_$*]+$`)

const (
	// MaxLines 单份配置最多行数。
	MaxLines = 2000
	// MaxBytes 单份配置最大字节数。
	MaxBytes = 64 * 1024
)

// Rule 单条忽略规则；Ops 为空表示忽略该表全部操作。
type Rule struct {
	Table  string
	Ops    map[string]bool
	AllOps bool
}

// Rules 规则集（已按表合并）。
type Rules []Rule

// Parse 解析规则文本，返回合并后的规则集；非法行返回带行号的错误。
func Parse(content string) (Rules, error) {
	if len(content) > MaxBytes {
		return nil, fmt.Errorf("配置过大（最多 %d 字节）", MaxBytes)
	}
	merged := make(map[string]*Rule)
	var order []string
	lines := strings.Split(content, "\n")
	if len(lines) > MaxLines+1 {
		return nil, fmt.Errorf("配置行数过多（最多 %d 行）", MaxLines)
	}
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		table, opsPart, hasOps := strings.Cut(line, ":")
		table = strings.TrimSpace(table)
		if !tableRe.MatchString(table) {
			return nil, fmt.Errorf("第 %d 行：表名非法 %q（仅支持字母、数字、下划线、$ 与 * 通配）", i+1, table)
		}
		r := merged[table]
		if r == nil {
			r = &Rule{Table: table, Ops: make(map[string]bool)}
			merged[table] = r
			order = append(order, table)
		}
		if r.AllOps {
			continue
		}
		if !hasOps {
			r.AllOps = true
			r.Ops = make(map[string]bool)
			continue
		}
		ops := strings.Split(opsPart, ",")
		empty := true
		for _, o := range ops {
			o = strings.ToUpper(strings.TrimSpace(o))
			if o == "" {
				continue
			}
			empty = false
			switch o {
			case "ALL":
				r.AllOps = true
				r.Ops = make(map[string]bool)
			case "DDL":
				for _, d := range ddlOps {
					r.Ops[d] = true
				}
			default:
				if !validOps[o] {
					return nil, fmt.Errorf("第 %d 行：未知操作 %q（支持 INSERT/UPDATE/DELETE/REPLACE/CREATE/ALTER/DROP/TRUNCATE/DDL/ALL）", i+1, o)
				}
				r.Ops[o] = true
			}
			if r.AllOps {
				break
			}
		}
		if empty {
			return nil, fmt.Errorf("第 %d 行：冒号后操作为空", i+1)
		}
	}
	out := make(Rules, 0, len(order))
	for _, t := range order {
		out = append(out, *merged[t])
	}
	return out, nil
}

// Ignore 判断某表某操作是否被忽略（大小写不敏感，支持 * 通配）。
func (rs Rules) Ignore(table, op string) bool {
	op = strings.ToUpper(strings.TrimSpace(op))
	for _, r := range rs {
		if !matchTable(r.Table, table) {
			continue
		}
		if r.AllOps {
			return true
		}
		if r.Ops[op] {
			return true
		}
	}
	return false
}

func matchTable(pattern, table string) bool {
	if !strings.Contains(pattern, "*") {
		return strings.EqualFold(pattern, table)
	}
	ok, err := path.Match(strings.ToLower(pattern), strings.ToLower(table))
	return err == nil && ok
}
