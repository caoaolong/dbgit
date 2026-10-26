package proxysql

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// 可直连的远端地址原样使用；本机地址需换算为容器可达地址。
func isLocalHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}

// resolveBackendTarget 将后端地址换算为 ProxySQL 容器可达的 (hostname, port)：
// 本机地址若对应宿主机上某个容器的发布端口，则用该容器名 + 容器内端口（并确保与 ProxySQL 同网）；
// 否则原样返回（远端库直连；本机无对应容器时回退 host.docker.internal，原端口）。
func resolveBackendTarget(ctx context.Context, proxyContainer, host string, port int) (string, int) {
	if !isLocalHost(host) {
		return strings.TrimSpace(host), port
	}
	name, internal := findPublisher(ctx, port)
	if name == "" {
		return "host.docker.internal", port
	}
	ensureSharedNetwork(ctx, proxyContainer, name)
	return name, internal
}

// findPublisher 查找发布了指定宿主机端口的容器名与其容器内端口。
func findPublisher(ctx context.Context, port int) (string, int) {
	out, err := exec.CommandContext(ctx, "docker", "ps", "--format", "{{.Names}}|{{.Ports}}").Output()
	if err != nil {
		return "", 0
	}
	needle := fmt.Sprintf(":%d->", port)
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 || !strings.Contains(parts[1], needle) {
			continue
		}
		name := strings.Trim(strings.TrimSpace(parts[0]), "/")
		if name == "" {
			continue
		}
		// 端口段形如 "0.0.0.0:3307->3306/tcp"，取箭头后的容器内端口。
		internal := 0
		if i := strings.Index(parts[1], needle); i >= 0 {
			rest := parts[1][i+len(needle):]
			fmt.Sscanf(rest, "%d", &internal)
		}
		if internal == 0 {
			internal = port
		}
		return name, internal
	}
	return "", 0
}

// containerNetworks 容器所接的网络名。
func containerNetworks(ctx context.Context, container string) []string {
	out, err := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{json .NetworkSettings.Networks}}", container).Output()
	if err != nil {
		return nil
	}
	var nets map[string]any
	if err := json.Unmarshal(out, &nets); err != nil {
		return nil
	}
	names := make([]string, 0, len(nets))
	for n := range nets {
		names = append(names, n)
	}
	return names
}

func userDefined(nets []string) []string {
	out := make([]string, 0, len(nets))
	for _, n := range nets {
		switch n {
		case "bridge", "host", "none", "":
		default:
			out = append(out, n)
		}
	}
	return out
}

// ensureSharedNetwork 确保目标容器与 ProxySQL 容器有共同的自定义网络，没有则尝试接入。
func ensureSharedNetwork(ctx context.Context, proxyContainer, target string) {
	proxyNets := userDefined(containerNetworks(ctx, proxyContainer))
	if len(proxyNets) == 0 {
		return
	}
	for _, n := range userDefined(containerNetworks(ctx, target)) {
		for _, p := range proxyNets {
			if n == p {
				return
			}
		}
	}
	// 尽力接入代理所在的第一个自定义网络。
	_ = exec.CommandContext(ctx, "docker", "network", "connect", proxyNets[0], target).Run()
}
