package uistate

import (
	"strings"
	"testing"

	core "proxydesk/internal/app"
	"proxydesk/internal/routeproxy"
)

func TestRouteDetailRunning(t *testing.T) {
	route := testRoute()
	view := RouteDetail(route, true)

	if view.Status != "运行中" || view.StatusTone != StatusToneRunning {
		t.Fatalf("unexpected status view: %+v", view)
	}
	if view.ExitDisplay != "75.82.36.185 United States California Chatsworth" {
		t.Fatalf("unexpected exit display: %q", view.ExitDisplay)
	}
	if view.LocalProtocol != "SOCKS5" {
		t.Fatalf("unexpected local protocol: %q", view.LocalProtocol)
	}
	if view.UpstreamProtocol != "HTTP" {
		t.Fatalf("unexpected upstream protocol: %q", view.UpstreamProtocol)
	}
	if !strings.Contains(view.UpstreamDisplay, "proxy.example.com:35001") {
		t.Fatalf("unexpected upstream display: %q", view.UpstreamDisplay)
	}
}

func TestRouteDetailIdleWithoutExit(t *testing.T) {
	route := testRoute()
	route.LastExitIP = ""
	view := RouteDetail(route, false)

	if view.Status != "未启动" || view.StatusTone != StatusToneIdle {
		t.Fatalf("unexpected idle view: %+v", view)
	}
	if view.ExitDisplay != "-" {
		t.Fatalf("unexpected empty exit display: %q", view.ExitDisplay)
	}
}

func TestRouteListItem(t *testing.T) {
	item := RouteListItem(routeproxy.Snapshot{
		Route:   testRoute(),
		Running: true,
	})

	for _, want := range []string{"[运行中]", "192.168.31.140:10001", "本地:SOCKS5", "上游:HTTP proxy.example.com:35001", "实际出口:75.82.36.185"} {
		if !strings.Contains(item, want) {
			t.Fatalf("expected list item to contain %q, got %q", want, item)
		}
	}
}

func TestChangedStatus(t *testing.T) {
	text, tone := ChangedStatus()
	if text != "配置已变更，需重启" || tone != StatusToneChanged {
		t.Fatalf("unexpected changed status: %q %q", text, tone)
	}
}

func testRoute() core.PortRoute {
	return core.PortRoute{
		LocalHost:       "192.168.31.140",
		LocalHTTPPort:   10001,
		LocalProtocol:   core.ProtocolSOCKS5,
		Protocol:        core.ProtocolHTTP,
		LastExitIP:      "75.82.36.185",
		LastExitCountry: "United States",
		LastExitRegion:  "California",
		LastExitCity:    "Chatsworth",
		Upstream: core.UpstreamProxy{
			Host:     "proxy.example.com",
			Port:     "35001",
			Username: "user",
			Password: "pass",
			Protocol: core.ProtocolHTTP,
		},
	}
}
