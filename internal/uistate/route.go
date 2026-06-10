package uistate

import (
	"fmt"
	"strings"

	core "proxydesk/internal/app"
	"proxydesk/internal/proxyparse"
	"proxydesk/internal/routeproxy"
)

type StatusTone string

const (
	StatusToneRunning StatusTone = "running"
	StatusToneIdle    StatusTone = "idle"
	StatusToneChanged StatusTone = "changed"
)

type RouteView struct {
	Status           string
	StatusTone       StatusTone
	ExitDisplay      string
	UpstreamDisplay  string
	ErrorDisplay     string
	LocalProtocol    string
	UpstreamProtocol string
}

func LocalProtocolDisplay(protocol core.Protocol) string {
	if protocol == core.ProtocolSOCKS5 {
		return "SOCKS5"
	}
	return "HTTP/HTTPS"
}

func UpstreamProtocolDisplay(protocol core.Protocol) string {
	if protocol == "" {
		return "-"
	}
	return string(protocol)
}

func ExitDisplay(route core.PortRoute) string {
	if strings.TrimSpace(route.LastExitIP) == "" {
		return "-"
	}
	return exitDisplay(route.LastExitIP, route.LastExitCountry, route.LastExitRegion, route.LastExitCity)
}

func RouteListItem(snapshot routeproxy.Snapshot) string {
	status := "未启动"
	if snapshot.Running {
		status = "运行中"
	}
	route := snapshot.Route
	return fmt.Sprintf("[%s] %s:%d  本地:%s  上游:%s %s  实际出口:%s",
		status,
		route.LocalHost,
		route.LocalHTTPPort,
		route.LocalProtocol,
		route.Upstream.Protocol,
		route.Upstream.Address(),
		ExitDisplay(route),
	)
}

func RouteDetail(route core.PortRoute, running bool) RouteView {
	view := RouteView{
		Status:           "未启动",
		StatusTone:       StatusToneIdle,
		ExitDisplay:      ExitDisplay(route),
		UpstreamDisplay:  proxyparse.Format(route.Upstream),
		ErrorDisplay:     "-",
		LocalProtocol:    LocalProtocolDisplay(route.LocalProtocol),
		UpstreamProtocol: UpstreamProtocolDisplay(route.Protocol),
	}
	if running {
		view.Status = "运行中"
		view.StatusTone = StatusToneRunning
	}
	return view
}

func ChangedStatus() (string, StatusTone) {
	return "配置已变更，需重启", StatusToneChanged
}

func exitDisplay(ip, country, region, city string) string {
	parts := []string{}
	if ip = strings.TrimSpace(ip); ip != "" {
		parts = append(parts, ip)
	}
	for _, part := range []string{country, region, city} {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " ")
}
