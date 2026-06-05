package proxyparse

import (
	"fmt"
	"strconv"
	"strings"

	"proxydesk/internal/app"
)

func ParseLine(line string, protocol app.Protocol) (app.UpstreamProxy, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return app.UpstreamProxy{}, fmt.Errorf("proxy is empty")
	}

	parts := strings.Split(line, ":")
	if len(parts) < 2 {
		return app.UpstreamProxy{}, fmt.Errorf("proxy must be host:port or host:port:user:pass")
	}
	if len(parts) > 4 {
		return app.UpstreamProxy{}, fmt.Errorf("proxy contains too many ':' separators")
	}
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return app.UpstreamProxy{}, fmt.Errorf("invalid proxy port %q", parts[1])
	}

	p := app.UpstreamProxy{
		Host:     parts[0],
		Port:     parts[1],
		Protocol: protocol,
	}
	if len(parts) >= 3 {
		p.Username = parts[2]
	}
	if len(parts) == 4 {
		p.Password = parts[3]
	}
	return p, nil
}

func Format(p app.UpstreamProxy) string {
	if p.Username == "" && p.Password == "" {
		return p.Address()
	}
	return p.Address() + ":" + p.Username + ":" + p.Password
}
