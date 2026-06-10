package routeproxy

import (
	"context"
	"testing"

	"proxydesk/internal/app"
)

func TestManagerAddSelectAndUsedPorts(t *testing.T) {
	manager := NewManager(nil)

	first := testRoute(10000)
	second := testRoute(10001)

	if idx := manager.Add(first); idx != 0 {
		t.Fatalf("first index = %d, want 0", idx)
	}
	if idx := manager.Add(second); idx != 1 {
		t.Fatalf("second index = %d, want 1", idx)
	}
	if manager.Selected() != 1 {
		t.Fatalf("selected = %d, want 1", manager.Selected())
	}

	used := manager.UsedPorts(0)
	if !used[10000] || !used[10001] {
		t.Fatalf("used ports = %#v, want 10000 and 10001", used)
	}

	used = manager.UsedPorts(10001)
	if !used[10000] || used[10001] {
		t.Fatalf("used ports with keep = %#v, want only 10000", used)
	}
}

func TestManagerDeleteUpdatesSelection(t *testing.T) {
	manager := NewManager(nil)
	manager.Add(testRoute(10000))
	manager.Add(testRoute(10001))
	manager.Add(testRoute(10002))

	deleted, ok := manager.Delete(2, context.Background())
	if !ok {
		t.Fatal("Delete returned false")
	}
	if deleted.LocalHTTPPort != 10002 {
		t.Fatalf("deleted port = %d, want 10002", deleted.LocalHTTPPort)
	}
	if manager.Selected() != 1 {
		t.Fatalf("selected after deleting last = %d, want 1", manager.Selected())
	}

	deleted, ok = manager.Delete(0, context.Background())
	if !ok {
		t.Fatal("Delete returned false")
	}
	if deleted.LocalHTTPPort != 10000 {
		t.Fatalf("deleted port = %d, want 10000", deleted.LocalHTTPPort)
	}
	if manager.Selected() != 0 {
		t.Fatalf("selected after deleting first = %d, want 0", manager.Selected())
	}
}

func testRoute(port int) app.PortRoute {
	return app.PortRoute{
		ID:            "test",
		Name:          "test",
		LocalHost:     "127.0.0.1",
		LocalHTTPPort: port,
		LocalProtocol: app.ProtocolHTTP,
		Protocol:      app.ProtocolHTTP,
		Upstream: app.UpstreamProxy{
			Host:     "127.0.0.1",
			Port:     "8080",
			Protocol: app.ProtocolHTTP,
		},
	}
}
