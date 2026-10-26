package proxysql

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// 默认参数。
const (
	DefaultContainerName = "dbgit-proxysql"
	DefaultVersion       = "latest"
	DefaultAdminPort     = 6032
	DefaultMySQLPort     = 6033
	DockerImage          = "proxysql/proxysql"
)

// InstallMethod 安装方式。
type InstallMethod string

const (
	MethodDocker  InstallMethod = "docker"
	MethodPackage InstallMethod = "package"
)

// Options 安装参数。
type Options struct {
	Method        InstallMethod `json:"method"`
	Version       string        `json:"version"`
	ContainerName string        `json:"container_name"`
	AdminPort     int           `json:"admin_port"`
	MySQLPort     int           `json:"mysql_port"`
	AdminUser     string        `json:"admin_user"`
	AdminPassword string        `json:"admin_password"`
}

var (
	nameRe    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
	versionRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
)

// ApplyDefaults 填充默认值。
func (o *Options) ApplyDefaults() {
	if o.Method == "" {
		o.Method = MethodDocker
	}
	if o.Version == "" {
		o.Version = DefaultVersion
	}
	if o.ContainerName == "" {
		o.ContainerName = DefaultContainerName
	}
	if o.AdminPort == 0 {
		o.AdminPort = DefaultAdminPort
	}
	if o.MySQLPort == 0 {
		o.MySQLPort = DefaultMySQLPort
	}
}

// Validate 校验参数（密码只做非空要求，不记入日志）。
func (o *Options) Validate() error {
	o.ApplyDefaults()
	switch o.Method {
	case MethodDocker, MethodPackage:
	default:
		return fmt.Errorf("不支持的安装方式: %q", o.Method)
	}
	if !versionRe.MatchString(o.Version) || len(o.Version) > 32 {
		return fmt.Errorf("非法版本: %q", o.Version)
	}
	if o.Method == MethodDocker {
		if !nameRe.MatchString(o.ContainerName) || len(o.ContainerName) > 64 {
			return fmt.Errorf("非法容器名: %q", o.ContainerName)
		}
	}
	for _, p := range []int{o.AdminPort, o.MySQLPort} {
		if p < 1 || p > 65535 {
			return fmt.Errorf("非法端口: %d", p)
		}
	}
	if o.AdminPort == o.MySQLPort {
		return fmt.Errorf("管理端口与 MySQL 端口不能相同")
	}
	return nil
}

// Status 本机 ProxySQL 状态。
type Status struct {
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	DockerAvailable bool   `json:"docker_available"`
	Installed       bool   `json:"installed"`
	Method          string `json:"method"` // docker | package | ""（未安装）
	Version         string `json:"version"`
	ContainerName   string `json:"container_name,omitempty"`
	Image           string `json:"image,omitempty"`
	AdminPort       int    `json:"admin_port,omitempty"`
	MySQLPort       int    `json:"mysql_port,omitempty"`
	Running         bool   `json:"running"`
	Detail          string `json:"detail"`
}

// Detect 探测本机状态：容器或二进制任一存在即视为已安装。
// 端口优先取容器实时映射；取不到时回退到上次成功安装时记录的参数。
func Detect() Status {
	st := Status{OS: runtime.GOOS, Arch: runtime.GOARCH}
	st.DockerAvailable = commandExists("docker")

	detectDocker(&st)
	detectBinary(&st)

	if saved, ok := LoadInstallState(); ok {
		if st.Method == "" {
			st.Method = string(saved.Method)
		}
		if st.AdminPort == 0 {
			st.AdminPort = saved.AdminPort
		}
		if st.MySQLPort == 0 {
			st.MySQLPort = saved.MySQLPort
		}
		if st.Version == "" {
			st.Version = saved.Version
		}
		if st.ContainerName == "" {
			st.ContainerName = saved.ContainerName
		}
	}
	return st
}

// detectDocker 查找镜像名含 proxysql 的容器（不限于本工具创建的）。
func detectDocker(st *Status) {
	if !st.DockerAvailable {
		return
	}
	out, err := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}\t{{.Image}}\t{{.Status}}").Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) != 3 || !strings.Contains(strings.ToLower(parts[1]), "proxysql") {
			continue
		}
		name, image, status := parts[0], parts[1], parts[2]
		st.Installed = true
		st.Method = string(MethodDocker)
		st.ContainerName = name
		st.Image = image
		st.Version = imageTag(image)
		st.Running = strings.HasPrefix(status, "Up")
		st.AdminPort, st.MySQLPort = dockerPorts(name)
		st.Detail = fmt.Sprintf("docker 容器 %s（%s）", name, status)
		return
	}
}

// detectBinary 检测 proxysql 二进制与进程。
func detectBinary(st *Status) {
	out, err := exec.Command("proxysql", "--version").CombinedOutput()
	if err != nil && !commandExists("proxysql") {
		return
	}
	st.Installed = true
	if st.Method == "" {
		st.Method = string(MethodPackage)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		st.Version = s
	}
	// 进程是否在跑（pgrep 不可用时视为未知，保持 false）。
	if err := exec.Command("pgrep", "-x", "proxysql").Run(); err == nil {
		st.Running = true
	}
	if st.Detail != "" {
		st.Detail += "; "
	}
	st.Detail += "系统包安装"
}

func imageTag(image string) string {
	if i := strings.LastIndex(image, ":"); i >= 0 && i < len(image)-1 {
		return image[i+1:]
	}
	return image
}

// dockerPorts 解析容器 6032/6033 的宿主机映射端口。
func dockerPorts(name string) (admin, mysql int) {
	out, err := exec.Command("docker", "port", name).Output()
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		// 形如 "6032/tcp -> 0.0.0.0:6032"
		halves := strings.Split(line, "->")
		if len(halves) != 2 {
			continue
		}
		containerPart := strings.TrimSpace(halves[0])
		hostPart := strings.TrimSpace(halves[1])
		port := 0
		if idx := strings.LastIndex(hostPart, ":"); idx >= 0 {
			fmt.Sscanf(hostPart[idx+1:], "%d", &port)
		}
		switch {
		case strings.HasPrefix(containerPart, "6032/"):
			admin = port
		case strings.HasPrefix(containerPart, "6033/"):
			mysql = port
		}
	}
	return admin, mysql
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// SavedState 上次成功安装时记录的参数（重启不丢失，供状态回显）。
type SavedState struct {
	Method        InstallMethod `json:"method"`
	Version       string        `json:"version"`
	ContainerName string        `json:"container_name"`
	AdminPort     int           `json:"admin_port"`
	MySQLPort     int           `json:"mysql_port"`
}

// stateFilePath 安装记录文件。可用 DBGIT_DATA_DIR 覆盖目录（默认相对后端工作目录的 data/）。
func stateFilePath() string {
	if dir := os.Getenv("DBGIT_DATA_DIR"); dir != "" {
		return filepath.Join(dir, "proxysql_install.json")
	}
	return filepath.Join("data", "proxysql_install.json")
}

// SaveInstallState 持久化成功安装的参数。
func SaveInstallState(opts Options) error {
	p := stateFilePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(SavedState{
		Method:        opts.Method,
		Version:       opts.Version,
		ContainerName: opts.ContainerName,
		AdminPort:     opts.AdminPort,
		MySQLPort:     opts.MySQLPort,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// LoadInstallState 读取安装记录，不存在返回 false。
func LoadInstallState() (SavedState, bool) {
	var s SavedState
	data, err := os.ReadFile(stateFilePath())
	if err != nil {
		return s, false
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, false
	}
	return s, true
}

// LogFunc 追加任务日志（实现方保证并发安全）。
type LogFunc func(format string, args ...any)

// Install 按方式分发安装。
func Install(ctx context.Context, opts Options, logf LogFunc) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	switch opts.Method {
	case MethodDocker:
		return installDocker(ctx, opts, logf)
	case MethodPackage:
		return installPackage(ctx, opts, logf)
	default:
		return fmt.Errorf("不支持的安装方式: %q", opts.Method)
	}
}

func runCmd(ctx context.Context, logf LogFunc, name string, args ...string) error {
	logf("$ %s %s", name, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if s := strings.TrimSpace(string(out)); s != "" {
		// 截断超长输出，避免日志爆炸。
		if len(s) > 4000 {
			s = s[:4000] + "\n...（输出已截断）"
		}
		logf("%s", s)
	}
	if err != nil {
		return fmt.Errorf("%s 执行失败: %w", name, err)
	}
	return nil
}

func installDocker(ctx context.Context, opts Options, logf LogFunc) error {
	if !commandExists("docker") {
		return fmt.Errorf("未检测到 docker，请先安装 Docker 或改用系统包方式")
	}
	image := fmt.Sprintf("%s:%s", DockerImage, opts.Version)

	if err := runCmd(ctx, logf, "docker", "--version"); err != nil {
		return err
	}
	// 已有同名容器则先移除，保证幂等。
	if out, err := exec.CommandContext(ctx, "docker", "ps", "-a", "--filter", "name=^"+opts.ContainerName+"$", "--format", "{{.Names}}").Output(); err == nil {
		if strings.TrimSpace(string(out)) != "" {
			logf("检测到同名容器 %q，先移除", opts.ContainerName)
			if err := runCmd(ctx, logf, "docker", "rm", "-f", opts.ContainerName); err != nil {
				return err
			}
		}
	}
	if err := runCmd(ctx, logf, "docker", "pull", image); err != nil {
		return err
	}
	if err := runCmd(ctx, logf, "docker", "run", "-d",
		"--name", opts.ContainerName,
		"--restart", "unless-stopped",
		"-p", fmt.Sprintf("%d:6032", opts.AdminPort),
		"-p", fmt.Sprintf("%d:6033", opts.MySQLPort),
		image); err != nil {
		return err
	}
	logf("容器已启动，等待管理端口 %d 就绪…", opts.AdminPort)
	if err := waitTCP(ctx, "127.0.0.1", opts.AdminPort, 30*time.Second); err != nil {
		return fmt.Errorf("容器已启动但管理端口未就绪: %w", err)
	}
	logf("ProxySQL（Docker）安装成功，管理端口 %d，MySQL 端口 %d", opts.AdminPort, opts.MySQLPort)
	return nil
}

func installPackage(ctx context.Context, opts Options, logf LogFunc) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("系统包安装仅支持 Linux（当前: %s），macOS/Windows 请用 Docker 方式", runtime.GOOS)
	}
	logf("系统包安装（版本参数仅 Docker 方式使用，当前忽略 %q）", opts.Version)
	switch {
	case commandExists("apt-get"):
		if err := runCmd(ctx, logf, "sudo", "apt-get", "update"); err != nil {
			return err
		}
		if err := runCmd(ctx, logf, "sudo", "apt-get", "install", "-y", "proxysql"); err != nil {
			return fmt.Errorf("apt 安装失败（如官方源缺失需先配置 ProxySQL 仓库）: %w", err)
		}
	case commandExists("dnf"):
		if err := runCmd(ctx, logf, "sudo", "dnf", "install", "-y", "proxysql"); err != nil {
			return err
		}
	case commandExists("yum"):
		if err := runCmd(ctx, logf, "sudo", "yum", "install", "-y", "proxysql"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("未检测到 apt-get/dnf/yum，不支持的 Linux 发行版")
	}
	// 尝试注册为系统服务，失败仅警告（容器/WSL 等环境可能无 systemd）。
	if err := runCmd(ctx, logf, "sudo", "systemctl", "enable", "--now", "proxysql"); err != nil {
		logf("警告：systemctl 启动服务失败，可手动启动 proxysql（%v）", err)
	}
	if out, err := exec.CommandContext(ctx, "proxysql", "--version").CombinedOutput(); err == nil {
		logf("ProxySQL（系统包）安装成功：%s", strings.TrimSpace(string(out)))
		return nil
	}
	logf("安装命令已执行，但未检测到 proxysql --version，请检查上方日志")
	return nil
}

func waitTCP(ctx context.Context, host string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 2*time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("等待 %s:%d 超时（%s）", host, port, timeout)
}
