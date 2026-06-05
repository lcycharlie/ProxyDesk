package proxyparse

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"proxydesk/internal/app"
)

func ParseLine(line string, protocol app.Protocol) (app.UpstreamProxy, error) {
	line = strings.Trim(strings.TrimSpace(line), `"'`)
	if line == "" {
		return app.UpstreamProxy{}, fmt.Errorf("proxy is empty")
	}

	if before, after, ok := strings.Cut(line, "://"); ok {
		scheme := strings.ToLower(before)
		if scheme == "http" || scheme == "https" || scheme == "socks" || scheme == "socks5" {
			line = after
		}
	}

	if strings.Contains(line, "@") {
		return parseUserInfoURL(line, protocol)
	}

	parts := strings.Split(line, ":")
	if len(parts) < 2 {
		return app.UpstreamProxy{}, fmt.Errorf("proxy must be host:port or host:port:user:pass")
	}
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return app.UpstreamProxy{}, fmt.Errorf("invalid proxy port %q", parts[1])
	}
	if parts[0] == "" {
		return app.UpstreamProxy{}, fmt.Errorf("proxy host is empty")
	}

	p := app.UpstreamProxy{
		Host:     parts[0],
		Port:     parts[1],
		Protocol: protocol,
	}
	if len(parts) >= 3 {
		p.Username = parts[2]
	}
	if len(parts) >= 4 {
		p.Password = strings.Join(parts[3:], ":")
	}
	return p, nil
}

func parseUserInfoURL(line string, protocol app.Protocol) (app.UpstreamProxy, error) {
	u, err := url.Parse("http://" + line)
	if err != nil {
		return app.UpstreamProxy{}, err
	}
	if u.Hostname() == "" {
		return app.UpstreamProxy{}, fmt.Errorf("proxy host is empty")
	}
	if u.Port() == "" {
		return app.UpstreamProxy{}, fmt.Errorf("proxy port is empty")
	}
	if _, err := strconv.Atoi(u.Port()); err != nil {
		return app.UpstreamProxy{}, fmt.Errorf("invalid proxy port %q", u.Port())
	}

	p := app.UpstreamProxy{
		Host:     u.Hostname(),
		Port:     u.Port(),
		Protocol: protocol,
	}
	if u.User != nil {
		p.Username = u.User.Username()
		p.Password, _ = u.User.Password()
	}
	return p, nil
}

func Format(p app.UpstreamProxy) string {
	if p.Username == "" && p.Password == "" {
		return p.Address()
	}
	return p.Address() + ":" + p.Username + ":" + p.Password
}
