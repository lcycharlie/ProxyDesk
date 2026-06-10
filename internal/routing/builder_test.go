package routing

import (
	"testing"
	"time"

	"proxydesk/internal/app"
)

func TestValidatePortRange(t *testing.T) {
	tests := []struct {
		name      string
		portRange PortRange
		wantErr   bool
	}{
		{name: "valid", portRange: PortRange{Start: 10000, End: 10099}},
		{name: "start too low", portRange: PortRange{Start: 0, End: 10099}, wantErr: true},
		{name: "end too high", portRange: PortRange{Start: 10000, End: 70000}, wantErr: true},
		{name: "reversed", portRange: PortRange{Start: 10099, End: 10000}, wantErr: true},
		{name: "too wide", portRange: PortRange{Start: 10000, End: 13001}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePortRange(tt.portRange)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidatePortRange() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPortOptionsSkipsUsedPorts(t *testing.T) {
	options := PortOptions(PortRange{Start: 10000, End: 10003}, map[int]bool{10001: true})
	want := []string{"10000", "10002", "10003"}
	if len(options) != len(want) {
		t.Fatalf("options = %#v, want %#v", options, want)
	}
	for i := range want {
		if options[i] != want[i] {
			t.Fatalf("options = %#v, want %#v", options, want)
		}
	}
}

func TestBuildManualRoute(t *testing.T) {
	now := time.Date(2026, 6, 10, 1, 2, 3, 0, time.UTC)
	route, err := BuildManualRoute(ManualRouteInput{
		ListenHost:       "192.168.31.140",
		PortText:         "10000",
		PortRange:        PortRange{Start: 10000, End: 10099},
		UsedPorts:        map[int]bool{},
		LocalProtocol:    app.ProtocolSOCKS5,
		UpstreamProtocol: app.ProtocolHTTP,
		ProxyLine:        "global.rpip.lokiproxy.com:35001:user:pass\nignored",
		Now:              now,
	})
	if err != nil {
		t.Fatalf("BuildManualRoute returned error: %v", err)
	}
	if route.LocalHost != "192.168.31.140" {
		t.Fatalf("LocalHost = %q", route.LocalHost)
	}
	if route.LocalHTTPPort != 10000 {
		t.Fatalf("LocalHTTPPort = %d", route.LocalHTTPPort)
	}
	if route.LocalProtocol != app.ProtocolSOCKS5 {
		t.Fatalf("LocalProtocol = %q", route.LocalProtocol)
	}
	if route.Upstream.Address() != "global.rpip.lokiproxy.com:35001" {
		t.Fatalf("Upstream address = %q", route.Upstream.Address())
	}
	if route.Upstream.Username != "user" || route.Upstream.Password != "pass" {
		t.Fatalf("Upstream auth = %q/%q", route.Upstream.Username, route.Upstream.Password)
	}
	if !route.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %v, want %v", route.UpdatedAt, now)
	}
}

func TestBuildRouteFromUpstreamRejectsUsedPort(t *testing.T) {
	_, err := BuildRouteFromUpstream(UpstreamRouteInput{
		ListenHost:    "192.168.31.140",
		PortText:      "10000",
		PortRange:     PortRange{Start: 10000, End: 10099},
		UsedPorts:     map[int]bool{10000: true},
		LocalProtocol: app.ProtocolHTTP,
		Upstream: app.UpstreamProxy{
			Host:     "1.2.3.4",
			Port:     "8080",
			Protocol: app.ProtocolSOCKS5,
		},
	})
	if err == nil {
		t.Fatal("BuildRouteFromUpstream returned nil error for used port")
	}
}
