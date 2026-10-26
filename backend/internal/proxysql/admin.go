package proxysql

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ContainerName 发现 ProxySQL 容器：安装记录 > 默认名 > 任意 proxysql 镜像容器。
func ContainerName() string {
	if s, ok := LoadInstallState(); ok && s.ContainerName != "" && containerRunning(s.ContainerName) {
		return s.ContainerName
	}
	if containerRunning(DefaultContainerName) {
		return DefaultContainerName
	}
	return anyProxySQLContainer()
}

func containerRunning(name string) bool {
	out, err := exec.Command("docker", "ps", "--format", "{{.Names}}").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

func anyProxySQLContainer() string {
	out, err := exec.Command("docker", "ps", "--format", "{{.Names}}\t{{.Image}}").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) == 2 && strings.Contains(strings.ToLower(parts[1]), "proxysql") {
			return strings.TrimSpace(parts[0])
		}
	}
	return ""
}

// MySQLPort 代理对外 MySQL 端口：安装记录 > 默认 6033。
func MySQLPort() int {
	if s, ok := LoadInstallState(); ok && s.MySQLPort > 0 {
		return s.MySQLPort
	}
	return DefaultMySQLPort
}

func escSQL(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// ExecAdmin 经 docker exec 在容器内执行 admin SQL（admin 只允许本地连接）。
// SQL 通过 stdin 传入，避免 shell 转义问题；密码走 MYSQL_PWD 环境变量。
// dockerHost 非空时透传给 docker -H（如 ssh://user@host），用于管理远端主机上的容器。
func ExecAdmin(ctx context.Context, container, adminUser, adminPass, dockerHost string, stmts ...string) error {
	_, err := QueryAdminRaw(ctx, container, adminUser, adminPass, dockerHost, joinStmts(stmts), false)
	return err
}

func joinStmts(stmts []string) string {
	var b strings.Builder
	for _, s := range stmts {
		if strings.TrimSpace(s) == "" {
			continue
		}
		b.WriteString(s)
		if !strings.HasSuffix(strings.TrimSpace(s), ";") {
			b.WriteString(";")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// QueryAdminRaw 经 docker exec 在容器内执行 admin SQL 并返回原始 stdout。
// noHeader=true 时输出 TSV 无表头（-N -B），否则首行为表头。
// 注意：stdout 与 stderr 分开采集，容器内 mysql 客户端打到 stderr 的 WARNING
// 行不会污染 TSV 解析。dockerHost 非空时透传给 docker -H。
func QueryAdminRaw(ctx context.Context, container, adminUser, adminPass, dockerHost, sqlText string, noHeader bool) (string, error) {
	if container == "" {
		return "", fmt.Errorf("未发现运行中的 ProxySQL 容器")
	}
	args := []string{"exec", "-i", "-e", "MYSQL_PWD=" + adminPass}
	if dockerHost != "" {
		args = append([]string{"-H", dockerHost}, args...)
	}
	args = append(args, container,
		"mysql", "-h127.0.0.1", "-P6032", "-u"+adminUser, "--protocol=TCP", "-B")
	if noHeader {
		args = append(args, "-N")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// SQL 必须以 ;\n 结尾：经 SSH 通道的 docker exec -i 里，
	// 缺了语句结束符 mysql 客户端会一直等待输入直到超时。
	sqlText = strings.TrimSpace(sqlText)
	if sqlText != "" && !strings.HasSuffix(sqlText, ";") {
		sqlText += ";"
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = strings.NewReader(sqlText + "\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return "", fmt.Errorf("proxysql admin 执行失败: %s: %w", msg, err)
	}
	return stdout.String(), nil
}
