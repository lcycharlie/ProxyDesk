package routing

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"proxydesk/internal/app"
	"proxydesk/internal/proxyparse"
)

const (
	DefaultPortStart = 10000
	DefaultPortEnd   = 10099
	MaxPortSpan      = 2000
)

type PortRange struct {
	Start int
	End   int
}

type ManualRouteInput struct {
	ListenHost       string
	PortText         string
	PortRange        PortRange
	UsedPorts        map[int]bool
	LocalProtocol    app.Protocol
	UpstreamProtocol app.Protocol
	ProxyLine        string
	Now              time.Time
}

type UpstreamRouteInput struct {
	ListenHost    string
	PortText      string
	PortRange     PortRange
	UsedPorts     map[int]bool
	LocalProtocol app.Protocol
	Upstream      app.UpstreamProxy
	Now           time.Time
}

func ValidatePortRange(portRange PortRange) error {
	if portRange.Start < 1 || portRange.Start > 65535 || portRange.End < 1 || portRange.End > 65535 {
		return fmt.Errorf("端口范围需要在 1-65535 之间")
	}
	if portRange.Start > portRange.End {
		return fmt.Errorf("端口起始不能大于端口结束")
	}
	if portRange.End-portRange.Start > MaxPortSpan {
		return fmt.Errorf("端口范围过大，请控制在 2000 个以内")
	}
	return nil
}

func PortOptions(portRange PortRange, usedPorts map[int]bool) []string {
	if portRange.Start > portRange.End {
		return []string{}
	}
	options := []string{}
	for port := portRange.Start; port <= portRange.End; port++ {
		if !usedPorts[port] {
			options = append(options, strconv.Itoa(port))
		}
	}
	return options
}

func BuildManualRoute(input ManualRouteInput) (app.PortRoute, error) {
	listenHost := strings.TrimSpace(input.ListenHost)
	if listenHost == "" {
		return app.PortRoute{}, fmt.Errorf("监听地址不能为空")
	}
	if listenHost != "0.0.0.0" && net.ParseIP(listenHost) == nil && listenHost != "localhost" {
		return app.PortRoute{}, fmt.Errorf("监听地址应为 127.0.0.1、本机内网 IP 或 0.0.0.0")
	}

	port, err := parsePort(input.PortText, input.PortRange, input.UsedPorts)
	if err != nil {
		return app.PortRoute{}, err
	}

	line := firstProxyLine(input.ProxyLine)
	upstream, err := proxyparse.ParseLine(line, input.UpstreamProtocol)
	if err != nil {
		return app.PortRoute{}, err
	}

	return buildRoute(listenHost, port, input.LocalProtocol, input.UpstreamProtocol, upstream, input.Now), nil
}

func BuildRouteFromUpstream(input UpstreamRouteInput) (app.PortRoute, error) {
	listenHost := strings.TrimSpace(input.ListenHost)
	if listenHost == "" {
		return app.PortRoute{}, fmt.Errorf("监听地址不能为空")
	}
	if listenHost != "0.0.0.0" && net.ParseIP(listenHost) == nil && listenHost != "localhost" {
		return app.PortRoute{}, fmt.Errorf("监听地址应为 127.0.0.1、本机内网 IP 或 0.0.0.0")
	}

	port, err := parsePort(input.PortText, input.PortRange, input.UsedPorts)
	if err != nil {
		return app.PortRoute{}, err
	}
	return buildRoute(listenHost, port, input.LocalProtocol, input.Upstream.Protocol, input.Upstream, input.Now), nil
}

func parsePort(portText string, portRange PortRange, usedPorts map[int]bool) (int, error) {
	if err := ValidatePortRange(portRange); err != nil {
		return 0, err
	}
	if strings.TrimSpace(portText) == "" {
		return 0, fmt.Errorf("当前端口范围内没有可用端口，请扩大范围或删除转发列表中的配置")
	}
	port, err := strconv.Atoi(strings.TrimSpace(portText))
	if err != nil || port < portRange.Start || port > portRange.End {
		return 0, fmt.Errorf("端口需要在 %d-%d 之间", portRange.Start, portRange.End)
	}
	if usedPorts[port] {
		return 0, fmt.Errorf("端口 %d 已被转发列表使用，请选择其他端口", port)
	}
	return port, nil
}

func firstProxyLine(value string) string {
	line := strings.TrimSpace(value)
	if strings.Contains(line, "\n") {
		line = strings.TrimSpace(strings.Split(line, "\n")[0])
	}
	return line
}

func buildRoute(listenHost string, port int, localProtocol app.Protocol, upstreamProtocol app.Protocol, upstream app.UpstreamProxy, now time.Time) app.PortRoute {
	if now.IsZero() {
		now = time.Now()
	}
	return app.PortRoute{
		ID:            "route-" + strconv.Itoa(port),
		Name:          "Port " + strconv.Itoa(port),
		LocalHost:     listenHost,
		LocalHTTPPort: port,
		LocalProtocol: localProtocol,
		Protocol:      upstreamProtocol,
		Upstream:      upstream,
		Enabled:       true,
		UpdatedAt:     now,
	}
}
