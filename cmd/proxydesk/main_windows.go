//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"golang.org/x/net/proxy"

	core "proxydesk/internal/app"
	"proxydesk/internal/localproxy"
	"proxydesk/internal/provider"
	"proxydesk/internal/proxyparse"
	"proxydesk/internal/systemproxy"
)

type runtimeState struct {
	routes   []routeRuntime
	selected int
}

type routeRuntime struct {
	route  core.PortRoute
	server interface {
		Stop(context.Context) error
	}
}

func (r routeRuntime) running() bool {
	return r.server != nil
}

type publicIPInfo struct {
	IP          string
	Country     string
	CountryCode string
	Region      string
	City        string
}

func (i publicIPInfo) Display() string {
	parts := []string{}
	if i.IP != "" {
		parts = append(parts, i.IP)
	}
	for _, part := range []string{i.Country, i.Region, i.City} {
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

func main() {
	state := &runtimeState{}
	state.selected = -1
	countries := allCountries()
	defaultCountry := defaultCountryIndex(countries, "US")
	filteredCountries := append([]string{}, countries...)
	detectedLANIP := detectLANIP()
	pageBackground := walk.RGB(244, 247, 249)
	panelBackground := walk.RGB(255, 255, 255)
	headerBackground := walk.RGB(238, 252, 248)
	headerCardBackground := walk.RGB(255, 255, 255)
	sidebarBackground := walk.RGB(248, 250, 252)
	contentBackground := walk.RGB(250, 252, 252)
	primaryText := walk.RGB(15, 23, 42)
	mutedText := walk.RGB(100, 116, 139)
	accentText := walk.RGB(13, 148, 136)
	activeButton := walk.RGB(204, 251, 241)
	ctaButton := walk.RGB(20, 184, 166)
	dangerText := walk.RGB(185, 28, 28)

	var mw *walk.MainWindow
	var countryCB, localProtocolCB, protocolCB, portCB *walk.ComboBox
	var apiLocalProtocolCB, apiProtocolCB, apiPortCB *walk.ComboBox
	var countrySearchEdit, listenHostEdit, portStartEdit, portEndEdit, apiEndpoint, apiCountryParam, apiJSONKey *walk.LineEdit
	var upstreamEdit, logBox *walk.TextEdit
	var routeList *walk.ListBox
	var contentTitle *walk.Label
	var dashboardPage, configPage, routePage, settingsPage *walk.Composite
	var settingsPortPage, settingsAPIPage, settingsLogPage *walk.Composite
	var navDashboardBtn, navConfigBtn, navRouteBtn, navSettingsBtn *walk.PushButton
	var settingsPortBtn, settingsAPIBtn, settingsLogBtn *walk.PushButton
	var statusLabel, exitIPLabel, upstreamLabel, errorLabel, localProtocolLabel, upstreamProtocolLabel *walk.Label
	var envExitLabel, localIPLabel *walk.Label
	var actualExitLabel *walk.Label
	loadingRoute := false

	appendLogDirect := func(format string, args ...any) {
		if logBox == nil {
			return
		}
		line := time.Now().Format("15:04:05") + "  " + fmt.Sprintf(format, args...)
		if strings.TrimSpace(logBox.Text()) != "" {
			logBox.AppendText("\r\n")
		}
		logBox.AppendText(line)
		logBox.ScrollToCaret()
	}
	appendLog := func(format string, args ...any) {
		if mw != nil {
			mw.Synchronize(func() {
				appendLogDirect(format, args...)
			})
			return
		}
		appendLogDirect(format, args...)
	}

	selectedCountry := func() string {
		text := strings.TrimSpace(countryCB.Text())
		if text != "" {
			for _, country := range countries {
				if strings.EqualFold(country, text) {
					return country
				}
			}
			if strings.Contains(text, " - ") {
				return text
			}
		}
		idx := countryCB.CurrentIndex()
		if idx >= 0 && idx < len(filteredCountries) {
			return filteredCountries[idx]
		}
		if defaultCountry >= 0 && defaultCountry < len(countries) {
			return countries[defaultCountry]
		}
		if len(countries) == 0 {
			return ""
		}
		return countries[0]
	}
	refreshCountryOptions := func() {
		if countryCB == nil || countrySearchEdit == nil {
			return
		}
		current := selectedCountry()
		filteredCountries = filterCountries(countries, countrySearchEdit.Text())
		_ = countryCB.SetModel(filteredCountries)
		idx := countryIndex(filteredCountries, current)
		if idx < 0 {
			idx = 0
		}
		if len(filteredCountries) > 0 {
			_ = countryCB.SetCurrentIndex(idx)
			return
		}
		_ = countryCB.SetText("")
	}
	selectedLocalProtocol := func() core.Protocol {
		if localProtocolCB.CurrentIndex() == 1 {
			return core.ProtocolSOCKS5
		}
		return core.ProtocolHTTP
	}
	selectedUpstreamProtocol := func() core.Protocol {
		if protocolCB.CurrentIndex() == 1 {
			return core.ProtocolSOCKS5
		}
		return core.ProtocolHTTP
	}
	selectedAPILocalProtocol := func() core.Protocol {
		if apiLocalProtocolCB != nil && apiLocalProtocolCB.CurrentIndex() == 1 {
			return core.ProtocolSOCKS5
		}
		return core.ProtocolHTTP
	}
	selectedAPIUpstreamProtocol := func() core.Protocol {
		if apiProtocolCB != nil && apiProtocolCB.CurrentIndex() == 1 {
			return core.ProtocolSOCKS5
		}
		return core.ProtocolHTTP
	}
	currentPortRange := func() (int, int) {
		start := 10000
		end := 10099
		if portStartEdit != nil {
			if value, err := strconv.Atoi(strings.TrimSpace(portStartEdit.Text())); err == nil {
				start = value
			}
		}
		if portEndEdit != nil {
			if value, err := strconv.Atoi(strings.TrimSpace(portEndEdit.Text())); err == nil {
				end = value
			}
		}
		return start, end
	}
	validatePortRange := func() (int, int, error) {
		start, end := currentPortRange()
		if start < 1 || start > 65535 || end < 1 || end > 65535 {
			return 0, 0, fmt.Errorf("端口范围需要在 1-65535 之间")
		}
		if start > end {
			return 0, 0, fmt.Errorf("端口起始不能大于端口结束")
		}
		if end-start > 2000 {
			return 0, 0, fmt.Errorf("端口范围过大，请控制在 2000 个以内")
		}
		return start, end, nil
	}
	portOptions := func(keepPort int) []string {
		start, end := currentPortRange()
		if start > end {
			return []string{}
		}
		used := map[int]bool{}
		for _, rt := range state.routes {
			if rt.route.LocalHTTPPort != keepPort {
				used[rt.route.LocalHTTPPort] = true
			}
		}
		options := []string{}
		for port := start; port <= end; port++ {
			if !used[port] {
				options = append(options, strconv.Itoa(port))
			}
		}
		return options
	}
	refreshPortOptions := func(keepPort int) {
		options := portOptions(keepPort)
		refreshOnePortCombo := func(combo *walk.ComboBox) {
			if combo == nil {
				return
			}
			_ = combo.SetModel(options)
			if keepPort > 0 {
				_ = combo.SetText(strconv.Itoa(keepPort))
				return
			}
			if len(options) > 0 {
				_ = combo.SetCurrentIndex(0)
				return
			}
			_ = combo.SetText("")
		}
		refreshOnePortCombo(portCB)
		refreshOnePortCombo(apiPortCB)
	}
	updateRunningProtocolLabels := func(route core.PortRoute) {
		if localProtocolLabel != nil {
			if route.LocalProtocol == core.ProtocolSOCKS5 {
				_ = localProtocolLabel.SetText("SOCKS5")
			} else {
				_ = localProtocolLabel.SetText("HTTP/HTTPS")
			}
		}
		if upstreamProtocolLabel != nil {
			_ = upstreamProtocolLabel.SetText(string(route.Protocol))
		}
	}
	markConfigChanged := func() {
		if loadingRoute {
			return
		}
		if state.selected < 0 || state.selected >= len(state.routes) || !state.routes[state.selected].running() || statusLabel == nil {
			return
		}
		_ = statusLabel.SetText("配置已变更，需重启")
		statusLabel.SetTextColor(walk.RGB(185, 100, 0))
	}
	portRangeChanged := func() {
		refreshPortOptions(0)
		markConfigChanged()
	}
	routeDisplay := func(rt routeRuntime) string {
		status := "未启动"
		if rt.running() {
			status = "运行中"
		}
		exit := "-"
		if rt.route.LastExitIP != "" {
			exit = publicIPInfo{
				IP:      rt.route.LastExitIP,
				Country: rt.route.LastExitCountry,
				Region:  rt.route.LastExitRegion,
				City:    rt.route.LastExitCity,
			}.Display()
		}
		return fmt.Sprintf("[%s] %s:%d  本地:%s  上游:%s %s  实际出口:%s",
			status,
			rt.route.LocalHost,
			rt.route.LocalHTTPPort,
			rt.route.LocalProtocol,
			rt.route.Upstream.Protocol,
			rt.route.Upstream.Address(),
			exit,
		)
	}
	refreshRouteList := func() {
		if routeList == nil {
			return
		}
		items := make([]string, len(state.routes))
		for i, rt := range state.routes {
			items[i] = routeDisplay(rt)
		}
		_ = routeList.SetModel(items)
		if len(items) == 0 {
			state.selected = -1
			return
		}
		if state.selected < 0 || state.selected >= len(items) {
			state.selected = 0
		}
		_ = routeList.SetCurrentIndex(state.selected)
	}
	showRoute := func(route core.PortRoute, running bool) {
		if running {
			_ = statusLabel.SetText("运行中")
			statusLabel.SetTextColor(walk.RGB(22, 120, 75))
		} else {
			_ = statusLabel.SetText("未启动")
			statusLabel.SetTextColor(walk.RGB(123, 94, 0))
		}
		updateRunningProtocolLabels(route)
		_ = upstreamLabel.SetText(proxyparse.Format(route.Upstream))
		exitDisplay := "-"
		if route.LastExitIP != "" {
			exitDisplay = publicIPInfo{
				IP:      route.LastExitIP,
				Country: route.LastExitCountry,
				Region:  route.LastExitRegion,
				City:    route.LastExitCity,
			}.Display()
		}
		_ = exitIPLabel.SetText(exitDisplay)
		if actualExitLabel != nil {
			_ = actualExitLabel.SetText(exitDisplay)
		}
		_ = errorLabel.SetText("-")
	}
	loadSelectedRoute := func() {
		if routeList == nil {
			return
		}
		idx := routeList.CurrentIndex()
		if idx < 0 || idx >= len(state.routes) {
			return
		}
		loadingRoute = true
		defer func() {
			loadingRoute = false
		}()
		state.selected = idx
		route := state.routes[idx].route
		_ = listenHostEdit.SetText(route.LocalHost)
		refreshPortOptions(route.LocalHTTPPort)
		if route.LocalProtocol == core.ProtocolSOCKS5 {
			_ = localProtocolCB.SetCurrentIndex(1)
		} else {
			_ = localProtocolCB.SetCurrentIndex(0)
		}
		if route.Protocol == core.ProtocolSOCKS5 {
			_ = protocolCB.SetCurrentIndex(1)
		} else {
			_ = protocolCB.SetCurrentIndex(0)
		}
		_ = upstreamEdit.SetText(proxyparse.Format(route.Upstream))
		showRoute(route, state.routes[idx].running())
	}

	buildRoute := func() (core.PortRoute, error) {
		listenHost := strings.TrimSpace(listenHostEdit.Text())
		if listenHost == "" {
			listenHost = detectedLANIP
		}
		if listenHost != "0.0.0.0" && net.ParseIP(listenHost) == nil && listenHost != "localhost" {
			return core.PortRoute{}, fmt.Errorf("监听地址应为 127.0.0.1、本机内网 IP 或 0.0.0.0")
		}
		startPort, endPort, err := validatePortRange()
		if err != nil {
			return core.PortRoute{}, err
		}
		if strings.TrimSpace(portCB.Text()) == "" {
			return core.PortRoute{}, fmt.Errorf("当前端口范围内没有可用端口，请扩大范围或删除转发列表中的配置")
		}
		port, err := strconv.Atoi(strings.TrimSpace(portCB.Text()))
		if err != nil || port < startPort || port > endPort {
			return core.PortRoute{}, fmt.Errorf("端口需要在 %d-%d 之间", startPort, endPort)
		}
		for _, rt := range state.routes {
			if rt.route.LocalHTTPPort == port {
				return core.PortRoute{}, fmt.Errorf("端口 %d 已被转发列表使用，请选择其他端口", port)
			}
		}
		localProtocol := selectedLocalProtocol()
		upstreamProtocol := selectedUpstreamProtocol()

		line := strings.TrimSpace(upstreamEdit.Text())
		if strings.Contains(line, "\n") {
			line = strings.TrimSpace(strings.Split(line, "\n")[0])
		}
		upstream, err := proxyparse.ParseLine(line, upstreamProtocol)
		if err != nil {
			return core.PortRoute{}, err
		}

		return core.PortRoute{
			ID:            "route-" + strconv.Itoa(port),
			Name:          "Port " + strconv.Itoa(port),
			CountryCode:   "",
			CountryName:   "",
			LocalHost:     listenHost,
			LocalHTTPPort: port,
			LocalProtocol: localProtocol,
			Protocol:      upstreamProtocol,
			Upstream:      upstream,
			Enabled:       true,
			UpdatedAt:     time.Now(),
		}, nil
	}

	buildRouteFromUpstream := func(portText string, localProtocol core.Protocol, upstream core.UpstreamProxy) (core.PortRoute, error) {
		startPort, endPort, err := validatePortRange()
		if err != nil {
			return core.PortRoute{}, err
		}
		if strings.TrimSpace(portText) == "" {
			return core.PortRoute{}, fmt.Errorf("当前端口范围内没有可用端口，请扩大范围或删除转发列表中的配置")
		}
		port, err := strconv.Atoi(strings.TrimSpace(portText))
		if err != nil || port < startPort || port > endPort {
			return core.PortRoute{}, fmt.Errorf("端口需要在 %d-%d 之间", startPort, endPort)
		}
		for _, rt := range state.routes {
			if rt.route.LocalHTTPPort == port {
				return core.PortRoute{}, fmt.Errorf("端口 %d 已被转发列表使用，请选择其他端口", port)
			}
		}
		return core.PortRoute{
			ID:            "route-" + strconv.Itoa(port),
			Name:          "Port " + strconv.Itoa(port),
			LocalHost:     detectedLANIP,
			LocalHTTPPort: port,
			LocalProtocol: localProtocol,
			Protocol:      upstream.Protocol,
			Upstream:      upstream,
			Enabled:       true,
			UpdatedAt:     time.Now(),
		}, nil
	}

	addRouteToList := func(route core.PortRoute, source string) {
		state.routes = append(state.routes, routeRuntime{route: route})
		state.selected = len(state.routes) - 1
		appendLog("已新增%s转发配置：%s:%d -> %s", source, route.LocalHost, route.LocalHTTPPort, route.Upstream.Address())
		refreshRouteList()
		showRoute(route, false)
		refreshPortOptions(0)
	}

	newServer := func(route core.PortRoute) (interface {
		Start() error
		Stop(context.Context) error
	}, error) {
		switch route.LocalProtocol {
		case core.ProtocolHTTP:
			httpServer := localproxy.NewHTTPServer(route)
			httpServer.OnLog = appendLog
			return httpServer, nil
		case core.ProtocolSOCKS5:
			socksServer := localproxy.NewSOCKS5Server(route)
			socksServer.OnLog = appendLog
			return socksServer, nil
		default:
			return nil, fmt.Errorf("不支持的本地协议：%s", route.LocalProtocol)
		}
	}
	addRoute := func() {
		route, err := buildRoute()
		if err != nil {
			walk.MsgBox(mw, "配置无效", err.Error(), walk.MsgBoxIconError)
			return
		}
		addRouteToList(route, "")
	}
	startRoute := func() {
		idx := state.selected
		if routeList != nil && routeList.CurrentIndex() >= 0 {
			idx = routeList.CurrentIndex()
		}
		if idx < 0 || idx >= len(state.routes) {
			walk.MsgBox(mw, "提示", "请先在线路配置中新增配置，再到转发列表启动", walk.MsgBoxIconInformation)
			return
		}
		if state.routes[idx].running() {
			_ = state.routes[idx].server.Stop(context.Background())
			state.routes[idx].server = nil
		}
		route := state.routes[idx].route
		server, err := newServer(route)
		if err != nil {
			walk.MsgBox(mw, "启动失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		if err := server.Start(); err != nil {
			_ = errorLabel.SetText(err.Error())
			walk.MsgBox(mw, "启动失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		state.routes[idx].server = server
		state.selected = idx
		showRoute(route, true)
		refreshRouteList()
		appendLog("已启动本地 %s 代理 %s:%d -> %s 上游 %s", route.LocalProtocol, route.LocalHost, route.LocalHTTPPort, route.Upstream.Protocol, route.Upstream.Address())
		if route.LocalHost == "0.0.0.0" {
			appendLog("局域网设备请使用这台 Windows 电脑的内网 IP:%d 作为 %s 代理", route.LocalHTTPPort, route.LocalProtocol)
		}
	}

	stopRoute := func() {
		idx := state.selected
		if routeList != nil && routeList.CurrentIndex() >= 0 {
			idx = routeList.CurrentIndex()
		}
		if idx < 0 || idx >= len(state.routes) || !state.routes[idx].running() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := state.routes[idx].server.Stop(ctx); err != nil {
			walk.MsgBox(mw, "停止失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		state.routes[idx].server = nil
		showRoute(state.routes[idx].route, false)
		refreshRouteList()
		appendLog("已停止本地转发：%s:%d", state.routes[idx].route.LocalHost, state.routes[idx].route.LocalHTTPPort)
	}
	deleteRoute := func() {
		idx := state.selected
		if routeList != nil && routeList.CurrentIndex() >= 0 {
			idx = routeList.CurrentIndex()
		}
		if idx < 0 || idx >= len(state.routes) {
			return
		}
		if state.routes[idx].running() {
			_ = state.routes[idx].server.Stop(context.Background())
		}
		appendLog("已删除转发配置：%s:%d", state.routes[idx].route.LocalHost, state.routes[idx].route.LocalHTTPPort)
		state.routes = append(state.routes[:idx], state.routes[idx+1:]...)
		if idx >= len(state.routes) {
			idx = len(state.routes) - 1
		}
		state.selected = idx
		refreshRouteList()
		refreshPortOptions(0)
		if idx >= 0 {
			loadSelectedRoute()
		}
	}
	stopAllRoutes := func() {
		for i := range state.routes {
			if state.routes[i].running() {
				_ = state.routes[i].server.Stop(context.Background())
				state.routes[i].server = nil
			}
		}
		refreshRouteList()
		appendLog("已停止全部转发")
	}

	testExitIP := func() {
		idx := state.selected
		if routeList != nil && routeList.CurrentIndex() >= 0 {
			idx = routeList.CurrentIndex()
		}
		if idx < 0 || idx >= len(state.routes) || !state.routes[idx].running() {
			walk.MsgBox(mw, "提示", "请先启动本地转发", walk.MsgBoxIconInformation)
			return
		}
		info, err := checkIP(state.routes[idx].route)
		if err != nil {
			_ = errorLabel.SetText(err.Error())
			appendLog("选中转发出口检测失败：%v", err)
			return
		}
		state.routes[idx].route.LastExitIP = info.IP
		state.routes[idx].route.LastExitCountry = info.Country
		state.routes[idx].route.LastExitRegion = info.Region
		state.routes[idx].route.LastExitCity = info.City
		refreshRouteList()
		_ = exitIPLabel.SetText(info.Display())
		_ = errorLabel.SetText("-")
		appendLog("选中转发出口检测成功：%s", info.Display())
	}

	fetchAPI := func() {
		countryCode, _ := splitCountry(selectedCountry())
		upstreamProtocol := selectedAPIUpstreamProtocol()
		client := provider.Client{
			Config: core.APIConfig{
				Endpoint:        strings.TrimSpace(apiEndpoint.Text()),
				Method:          http.MethodGet,
				CountryParam:    strings.TrimSpace(apiCountryParam.Text()),
				ResponseJSONKey: strings.TrimSpace(apiJSONKey.Text()),
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		upstream, err := client.Fetch(ctx, countryCode, upstreamProtocol)
		if err != nil {
			_ = errorLabel.SetText(err.Error())
			appendLog("API 获取失败：%v", err)
			walk.MsgBox(mw, "API 获取失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		route, err := buildRouteFromUpstream(apiPortCB.Text(), selectedAPILocalProtocol(), upstream)
		if err != nil {
			_ = errorLabel.SetText(err.Error())
			appendLog("API 新增转发失败：%v", err)
			walk.MsgBox(mw, "API 新增转发失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		addRouteToList(route, " API")
		_ = upstreamEdit.SetText(proxyparse.Format(upstream))
		_ = errorLabel.SetText("-")
		appendLog("API 获取成功：%s %s，已加入转发列表端口 %d", countryCode, upstream.Address(), route.LocalHTTPPort)
		walk.MsgBox(
			mw,
			"API 获取成功",
			fmt.Sprintf("已提取 IP 并加入转发列表。\n\n本地代理：%s:%d\n上游代理：%s\n\n请到“转发列表”查看和启动。", route.LocalHost, route.LocalHTTPPort, upstream.Address()),
			walk.MsgBoxIconInformation,
		)
	}

	enableSystemProxy := func() {
		idx := state.selected
		if routeList != nil && routeList.CurrentIndex() >= 0 {
			idx = routeList.CurrentIndex()
		}
		if idx < 0 || idx >= len(state.routes) {
			walk.MsgBox(mw, "系统代理失败", "请先在转发列表中选择一条配置", walk.MsgBoxIconError)
			return
		}
		route := state.routes[idx].route
		host := localConnectHost(route)
		if err := systemproxy.EnableProxy(host, route.LocalHTTPPort, string(route.LocalProtocol)); err != nil {
			walk.MsgBox(mw, "系统代理失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		appendLog("已开启 Windows %s 系统代理：%s:%d", route.LocalProtocol, host, route.LocalHTTPPort)
	}

	disableSystemProxy := func() {
		if err := systemproxy.DisableHTTPProxy(); err != nil {
			walk.MsgBox(mw, "系统代理失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		appendLog("已关闭 Windows 系统代理")
	}
	clearLogs := func() {
		if logBox != nil {
			_ = logBox.SetText("")
		}
	}
	setButtonBackground := func(button *walk.PushButton, color walk.Color) {
		if button == nil {
			return
		}
		brush, err := walk.NewSolidColorBrush(color)
		if err != nil {
			return
		}
		button.SetBackground(brush)
	}
	pageNames := []string{"工作台", "线路配置", "转发列表", "设置"}
	openPage := func(index int) func() {
		return func() {
			pages := []*walk.Composite{dashboardPage, configPage, routePage, settingsPage}
			buttons := []*walk.PushButton{navDashboardBtn, navConfigBtn, navRouteBtn, navSettingsBtn}
			if index < 0 || index >= len(pages) {
				return
			}
			for i, page := range pages {
				if page != nil {
					page.SetVisible(i == index)
				}
				if i < len(buttons) && buttons[i] != nil {
					if i == index {
						setButtonBackground(buttons[i], activeButton)
						continue
					}
					setButtonBackground(buttons[i], walk.RGB(255, 255, 255))
				}
			}
			if contentTitle != nil {
				_ = contentTitle.SetText(pageNames[index])
			}
		}
	}
	openSettingsSection := func(index int) func() {
		return func() {
			pages := []*walk.Composite{settingsPortPage, settingsAPIPage, settingsLogPage}
			buttons := []*walk.PushButton{settingsPortBtn, settingsAPIBtn, settingsLogBtn}
			if index < 0 || index >= len(pages) {
				return
			}
			for i, page := range pages {
				if page != nil {
					page.SetVisible(i == index)
				}
				if i < len(buttons) && buttons[i] != nil {
					if i == index {
						setButtonBackground(buttons[i], activeButton)
						continue
					}
					setButtonBackground(buttons[i], walk.RGB(255, 255, 255))
				}
			}
		}
	}
	updateEnvironmentExit := func() {
		for i := 0; i < 20; i++ {
			if envExitLabel != nil && mw != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if envExitLabel == nil || mw == nil {
			return
		}
		client := &http.Client{Timeout: 12 * time.Second}
		info, err := fetchPublicIPInfo(client)
		mw.Synchronize(func() {
			if err != nil {
				_ = envExitLabel.SetText("检测失败")
				return
			}
			_ = envExitLabel.SetText(environmentCountryDisplay(info))
		})
	}
	refreshEnvironmentExit := func() {
		if envExitLabel != nil {
			_ = envExitLabel.SetText("检测中...")
		}
		go updateEnvironmentExit()
	}
	time.AfterFunc(300*time.Millisecond, updateEnvironmentExit)

	mainWindow := MainWindow{
		AssignTo:   &mw,
		Title:      "ProxyDesk",
		Icon:       1,
		MinSize:    Size{Width: 1040, Height: 700},
		Size:       Size{Width: 1180, Height: 760},
		Font:       Font{Family: "Microsoft YaHei UI", PointSize: 9},
		Background: SolidColorBrush{Color: pageBackground},
		Layout:     VBox{Margins: Margins{Left: 18, Top: 16, Right: 18, Bottom: 16}, Spacing: 14},
		Children: []Widget{
			GradientComposite{
				Background: SolidColorBrush{Color: headerBackground},
				Color1:     headerBackground,
				Color2:     walk.RGB(250, 255, 253),
				Vertical:   false,
				Layout:     VBox{Margins: Margins{Left: 22, Top: 16, Right: 22, Bottom: 14}, Spacing: 10},
				Children: []Widget{
					Composite{
						Layout: HBox{MarginsZero: true, Spacing: 14},
						Children: []Widget{
							Composite{
								Layout: VBox{MarginsZero: true, Spacing: 2},
								Children: []Widget{
									Label{
										Text:      "ProxyDesk",
										Font:      Font{Family: "Microsoft YaHei UI", PointSize: 18, Bold: true},
										TextColor: primaryText,
									},
									Label{
										Text:      "国家住宅 IP 本地端口转发器",
										TextColor: mutedText,
									},
								},
							},
							HSpacer{},
							Composite{
								MinSize:    Size{Width: 230, Height: 56},
								MaxSize:    Size{Width: 230, Height: 56},
								Layout:     VBox{Margins: Margins{Left: 14, Top: 8, Right: 14, Bottom: 8}, Spacing: 2},
								Background: SolidColorBrush{Color: headerCardBackground},
								Children: []Widget{
									Composite{
										Layout: HBox{MarginsZero: true, Spacing: 6},
										Children: []Widget{
											Label{Text: "当前环境出口", TextColor: mutedText},
											HSpacer{},
											LinkLabel{
												Text:    `<a id="refresh">↻</a>`,
												MinSize: Size{Width: 20, Height: 20},
												OnLinkActivated: func(link *walk.LinkLabelLink) {
													refreshEnvironmentExit()
												},
											},
										},
									},
									Label{
										AssignTo:     &envExitLabel,
										Text:         "检测中...",
										Font:         Font{Family: "Consolas", PointSize: 12, Bold: true},
										TextColor:    accentText,
										EllipsisMode: EllipsisEnd,
									},
								},
							},
							Composite{
								MinSize:    Size{Width: 230, Height: 56},
								MaxSize:    Size{Width: 230, Height: 56},
								Layout:     VBox{Margins: Margins{Left: 14, Top: 8, Right: 14, Bottom: 8}, Spacing: 2},
								Background: SolidColorBrush{Color: headerCardBackground},
								Children: []Widget{
									Label{Text: "本地 IP", TextColor: mutedText},
									Label{
										AssignTo:  &localIPLabel,
										Text:      detectedLANIP,
										Font:      Font{Family: "Consolas", PointSize: 12, Bold: true},
										TextColor: accentText,
									},
								},
							},
						},
					},
					Composite{
						Layout: HBox{MarginsZero: true, Spacing: 16},
						Children: []Widget{
							Label{Text: "状态", TextColor: mutedText},
							Label{
								AssignTo:  &statusLabel,
								Text:      "未启动",
								Font:      Font{Family: "Microsoft YaHei UI", PointSize: 10, Bold: true},
								TextColor: walk.RGB(123, 94, 0),
							},
							Label{Text: "出口 IP", TextColor: mutedText},
							Label{AssignTo: &exitIPLabel, Text: "-", TextColor: accentText},
							Label{Text: "运行本地协议", TextColor: mutedText},
							Label{AssignTo: &localProtocolLabel, Text: "HTTP/HTTPS", TextColor: accentText},
							Label{Text: "运行上游协议", TextColor: mutedText},
							Label{AssignTo: &upstreamProtocolLabel, Text: "HTTP", TextColor: accentText},
						},
					},
				},
			},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 12},
				Children: []Widget{
					Composite{
						MinSize:    Size{Width: 164, Height: 520},
						MaxSize:    Size{Width: 164},
						Background: SolidColorBrush{Color: sidebarBackground},
						Layout:     VBox{Margins: Margins{Left: 12, Top: 18, Right: 12, Bottom: 18}, Spacing: 10},
						Children: []Widget{
							Label{Text: "ProxyDesk", Font: Font{Family: "Microsoft YaHei UI", PointSize: 12, Bold: true}, TextColor: primaryText},
							Label{Text: "端口转发控制台", TextColor: mutedText},
							VSpacer{Size: 12},
							PushButton{AssignTo: &navDashboardBtn, Text: "概览", MinSize: Size{Height: 38}, Background: SolidColorBrush{Color: activeButton}, OnClicked: openPage(0)},
							PushButton{AssignTo: &navConfigBtn, Text: "线路配置", MinSize: Size{Height: 38}, Background: SolidColorBrush{Color: walk.RGB(255, 255, 255)}, OnClicked: openPage(1)},
							PushButton{AssignTo: &navRouteBtn, Text: "转发列表", MinSize: Size{Height: 38}, Background: SolidColorBrush{Color: walk.RGB(255, 255, 255)}, OnClicked: openPage(2)},
							PushButton{AssignTo: &navSettingsBtn, Text: "设置", MinSize: Size{Height: 38}, Background: SolidColorBrush{Color: walk.RGB(255, 255, 255)}, OnClicked: openPage(3)},
							VSpacer{},
							Label{Text: "实际国家看出口检测", TextColor: mutedText},
						},
					},
					Composite{
						MinSize:       Size{Width: 760, Height: 520},
						StretchFactor: 1,
						Background:    SolidColorBrush{Color: contentBackground},
						Layout:        VBox{Margins: Margins{Left: 16, Top: 14, Right: 16, Bottom: 14}, Spacing: 12},
						Children: []Widget{
							Label{AssignTo: &contentTitle, Text: "工作台", Font: Font{Family: "Microsoft YaHei UI", PointSize: 15, Bold: true}, TextColor: primaryText},
							Composite{
								AssignTo: &dashboardPage,
								Layout:   VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}, Spacing: 12},
								Children: []Widget{
									GroupBox{
										Title:      "当前连接",
										Layout:     VBox{Margins: Margins{Left: 16, Top: 12, Right: 16, Bottom: 12}, Spacing: 10},
										Background: SolidColorBrush{Color: panelBackground},
										Children: []Widget{
											Composite{
												Layout: Grid{Columns: 2, MarginsZero: true, Spacing: 8},
												Children: []Widget{
													Label{Text: "实际出口", TextColor: mutedText},
													Label{AssignTo: &actualExitLabel, Text: "-", TextColor: accentText, EllipsisMode: EllipsisEnd},
													Label{Text: "上游代理", TextColor: mutedText},
													Label{AssignTo: &upstreamLabel, Text: "-", TextColor: primaryText, EllipsisMode: EllipsisEnd},
													Label{Text: "最近错误", TextColor: mutedText},
													Label{AssignTo: &errorLabel, Text: "-", TextColor: dangerText, EllipsisMode: EllipsisEnd},
												},
											},
											Composite{
												MinSize:    Size{Height: 1},
												MaxSize:    Size{Height: 1},
												Background: SolidColorBrush{Color: walk.RGB(226, 232, 240)},
											},
											Label{Text: "系统代理", Font: Font{Family: "Microsoft YaHei UI", PointSize: 10, Bold: true}, TextColor: primaryText},
											Label{Text: "需要让浏览器或多数桌面软件直接走代理时，可开启 Windows 系统代理。", TextColor: mutedText},
											Composite{
												Layout: HBox{MarginsZero: true, Spacing: 8},
												Children: []Widget{
													PushButton{Text: "开启系统代理", MinSize: Size{Width: 130, Height: 34}, Background: SolidColorBrush{Color: ctaButton}, OnClicked: enableSystemProxy},
													PushButton{Text: "关闭系统代理", MinSize: Size{Width: 130, Height: 32}, Background: SolidColorBrush{Color: walk.RGB(255, 255, 255)}, OnClicked: disableSystemProxy},
													HSpacer{},
												},
											},
										},
									},
									GroupBox{
										Title:      "使用提示",
										Layout:     VBox{Margins: Margins{Left: 16, Top: 12, Right: 16, Bottom: 12}, Spacing: 8},
										Background: SolidColorBrush{Color: panelBackground},
										Children: []Widget{
											Label{Text: "其他设备使用“本地 IP”加对应端口；工具需要 SOCKS5 时，本地协议请选择 SOCKS5。", TextColor: accentText},
											Label{Text: "多条运行中的端口可以同时给不同浏览器、指纹浏览器或桌面工具使用。", TextColor: accentText},
										},
									},
									VSpacer{},
								},
							},
							Composite{
								AssignTo: &configPage,
								Visible:  false,
								Layout:   VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}, Spacing: 10},
								Children: []Widget{
									GroupBox{
										Title:      "线路配置",
										Layout:     VBox{Margins: Margins{Left: 14, Top: 12, Right: 14, Bottom: 12}, Spacing: 10},
										Background: SolidColorBrush{Color: panelBackground},
										Children: []Widget{
											Composite{
												Layout: Grid{Columns: 2, MarginsZero: true, Spacing: 8},
												Children: []Widget{
													Label{Text: "本地协议", TextColor: mutedText},
													ComboBox{AssignTo: &localProtocolCB, Model: []string{"HTTP/HTTPS", "SOCKS5"}, CurrentIndex: 0, MinSize: Size{Height: 26}, OnCurrentIndexChanged: markConfigChanged},
													Label{Text: "上游协议", TextColor: mutedText},
													ComboBox{AssignTo: &protocolCB, Model: []string{"HTTP", "SOCKS5"}, CurrentIndex: 0, MinSize: Size{Height: 26}, OnCurrentIndexChanged: markConfigChanged},
													Label{Text: "监听地址", TextColor: mutedText},
													LineEdit{AssignTo: &listenHostEdit, Text: detectedLANIP, ReadOnly: true, MinSize: Size{Height: 26}},
													Label{Text: "本地端口", TextColor: mutedText},
													ComboBox{AssignTo: &portCB, Model: portOptions(0), CurrentIndex: 0, MinSize: Size{Height: 26}, OnCurrentIndexChanged: markConfigChanged},
												},
											},
											Label{Text: "上游代理", TextColor: mutedText},
											TextEdit{AssignTo: &upstreamEdit, MinSize: Size{Height: 170}, OnTextChanged: markConfigChanged},
											Label{Text: "监听地址固定为本机内网 IP；端口范围可在设置中调整。", TextColor: mutedText},
											Composite{
												Layout: HBox{MarginsZero: true, Spacing: 8},
												Children: []Widget{
													PushButton{Text: "新增配置", MinSize: Size{Width: 100, Height: 34}, Background: SolidColorBrush{Color: ctaButton}, OnClicked: addRoute},
													HSpacer{},
												},
											},
										},
									},
								},
							},
							Composite{
								AssignTo: &routePage,
								Visible:  false,
								Layout:   VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}, Spacing: 10},
								Children: []Widget{
									GroupBox{
										Title:      "转发列表",
										Layout:     VBox{Margins: Margins{Left: 12, Top: 10, Right: 12, Bottom: 10}, Spacing: 8},
										Background: SolidColorBrush{Color: panelBackground},
										Children: []Widget{
											Label{Text: "选中哪一条，测试选中出口、开启系统代理就针对哪一条。列表中的“实际出口”会显示检测到的国家/地区。", TextColor: accentText},
											ListBox{
												AssignTo:              &routeList,
												Model:                 []string{},
												MinSize:               Size{Height: 280},
												OnCurrentIndexChanged: loadSelectedRoute,
											},
											Composite{
												Layout: HBox{MarginsZero: true, Spacing: 8},
												Children: []Widget{
													PushButton{Text: "启动选中", MinSize: Size{Width: 110, Height: 32}, Background: SolidColorBrush{Color: ctaButton}, OnClicked: startRoute},
													PushButton{Text: "停止选中", MinSize: Size{Width: 110, Height: 30}, Background: SolidColorBrush{Color: walk.RGB(255, 255, 255)}, OnClicked: stopRoute},
													PushButton{Text: "测试选中出口", MinSize: Size{Width: 120, Height: 30}, Background: SolidColorBrush{Color: walk.RGB(255, 255, 255)}, OnClicked: testExitIP},
													PushButton{Text: "删除选中", MinSize: Size{Width: 110, Height: 30}, Background: SolidColorBrush{Color: walk.RGB(255, 255, 255)}, OnClicked: deleteRoute},
													PushButton{Text: "停止全部", MinSize: Size{Width: 110, Height: 30}, Background: SolidColorBrush{Color: walk.RGB(255, 255, 255)}, OnClicked: stopAllRoutes},
													HSpacer{},
												},
											},
										},
									},
								},
							},
							Composite{
								AssignTo: &settingsPage,
								Visible:  false,
								Layout:   HBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}, Spacing: 12},
								Children: []Widget{
									Composite{
										MinSize:    Size{Width: 150, Height: 420},
										MaxSize:    Size{Width: 150},
										Layout:     VBox{Margins: Margins{Left: 12, Top: 14, Right: 12, Bottom: 14}, Spacing: 10},
										Background: SolidColorBrush{Color: panelBackground},
										Children: []Widget{
											Label{Text: "设置模块", Font: Font{Family: "Microsoft YaHei UI", PointSize: 10, Bold: true}, TextColor: primaryText},
											PushButton{AssignTo: &settingsPortBtn, Text: "端口范围", MinSize: Size{Height: 36}, Background: SolidColorBrush{Color: activeButton}, OnClicked: openSettingsSection(0)},
											PushButton{AssignTo: &settingsAPIBtn, Text: "供应商 API", MinSize: Size{Height: 36}, Background: SolidColorBrush{Color: walk.RGB(255, 255, 255)}, OnClicked: openSettingsSection(1)},
											PushButton{AssignTo: &settingsLogBtn, Text: "运行日志", MinSize: Size{Height: 36}, Background: SolidColorBrush{Color: walk.RGB(255, 255, 255)}, OnClicked: openSettingsSection(2)},
											VSpacer{},
											Label{Text: "端口和 API 共用", TextColor: accentText},
											Label{Text: "同一个可用端口池。", TextColor: accentText},
										},
									},
									Composite{
										StretchFactor: 1,
										Layout:        VBox{MarginsZero: true},
										Children: []Widget{
											Composite{
												AssignTo: &settingsPortPage,
												Layout:   VBox{MarginsZero: true, Spacing: 10},
												Children: []Widget{
													Composite{
														Layout:     VBox{Margins: Margins{Left: 14, Top: 12, Right: 14, Bottom: 12}, Spacing: 10},
														Background: SolidColorBrush{Color: panelBackground},
														Children: []Widget{
															Label{Text: "端口范围", Font: Font{Family: "Microsoft YaHei UI", PointSize: 10, Bold: true}, TextColor: primaryText},
															Composite{
																Layout: Grid{Columns: 2, MarginsZero: true, Spacing: 8},
																Children: []Widget{
																	Label{Text: "端口起始", TextColor: mutedText},
																	LineEdit{AssignTo: &portStartEdit, Text: "10000", MinSize: Size{Height: 26}, OnTextChanged: portRangeChanged},
																	Label{Text: "端口结束", TextColor: mutedText},
																	LineEdit{AssignTo: &portEndEdit, Text: "10099", MinSize: Size{Height: 26}, OnTextChanged: portRangeChanged},
																},
															},
															Label{Text: "本地端口下拉会按这个范围生成，并自动排除转发列表里已占用的端口。", TextColor: mutedText},
														},
													},
													VSpacer{},
												},
											},
											Composite{
												AssignTo: &settingsAPIPage,
												Visible:  false,
												Layout:   VBox{MarginsZero: true, Spacing: 10},
												Children: []Widget{
													Composite{
														Layout:     VBox{Margins: Margins{Left: 14, Top: 12, Right: 14, Bottom: 12}, Spacing: 10},
														Background: SolidColorBrush{Color: panelBackground},
														Children: []Widget{
															Label{Text: "供应商 API", Font: Font{Family: "Microsoft YaHei UI", PointSize: 10, Bold: true}, TextColor: primaryText},
															Composite{
																Layout: Grid{Columns: 2, MarginsZero: true, Spacing: 8},
																Children: []Widget{
																	Label{Text: "国家搜索"},
																	LineEdit{AssignTo: &countrySearchEdit, MinSize: Size{Height: 26}, OnTextChanged: refreshCountryOptions},
																	Label{Text: "国家/地区"},
																	ComboBox{AssignTo: &countryCB, Model: filteredCountries, CurrentIndex: defaultCountry, MinSize: Size{Height: 26}},
																	Label{Text: "本地协议"},
																	ComboBox{AssignTo: &apiLocalProtocolCB, Model: []string{"HTTP/HTTPS", "SOCKS5"}, CurrentIndex: 0, MinSize: Size{Height: 26}},
																	Label{Text: "上游协议"},
																	ComboBox{AssignTo: &apiProtocolCB, Model: []string{"HTTP", "SOCKS5"}, CurrentIndex: 0, MinSize: Size{Height: 26}},
																	Label{Text: "本地端口"},
																	ComboBox{AssignTo: &apiPortCB, Model: portOptions(0), CurrentIndex: 0, MinSize: Size{Height: 26}},
																	Label{Text: "API 地址"},
																	LineEdit{AssignTo: &apiEndpoint, Text: "http://gen.lokiproxy.com/gen?ptype=13&count=1&proto=http&stype=text&split=rn", MinSize: Size{Height: 26}},
																	Label{Text: "国家参数"},
																	LineEdit{AssignTo: &apiCountryParam, Text: "region", MinSize: Size{Height: 26}},
																	Label{Text: "JSON 字段"},
																	LineEdit{AssignTo: &apiJSONKey, MinSize: Size{Height: 26}},
																},
															},
															Composite{
																Layout: HBox{MarginsZero: true},
																Children: []Widget{
																	HSpacer{},
																	PushButton{Text: "按国家获取 IP", MinSize: Size{Width: 150, Height: 34}, Background: SolidColorBrush{Color: ctaButton}, OnClicked: fetchAPI},
																},
															},
														},
													},
													VSpacer{},
												},
											},
											Composite{
												AssignTo: &settingsLogPage,
												Visible:  false,
												Layout:   VBox{MarginsZero: true, Spacing: 10},
												Children: []Widget{
													Composite{
														Layout:     VBox{Margins: Margins{Left: 14, Top: 12, Right: 14, Bottom: 12}, Spacing: 8},
														Background: SolidColorBrush{Color: panelBackground},
														Children: []Widget{
															Label{Text: "运行日志", Font: Font{Family: "Microsoft YaHei UI", PointSize: 10, Bold: true}, TextColor: primaryText},
															Composite{
																Layout: HBox{MarginsZero: true},
																Children: []Widget{
																	Label{Text: "运行日志会自动滚动到底部，可手动滑动查看历史。", TextColor: accentText},
																	HSpacer{},
																	PushButton{Text: "清理日志", MinSize: Size{Width: 100, Height: 30}, Background: SolidColorBrush{Color: walk.RGB(255, 255, 255)}, OnClicked: clearLogs},
																},
															},
															TextEdit{
																AssignTo: &logBox,
																ReadOnly: true,
																MinSize:  Size{Height: 360},
																Font:     Font{Family: "Consolas", PointSize: 9},
																VScroll:  true,
																HScroll:  true,
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	if err := mainWindow.Create(); err != nil {
		writeStartupError(err)
		walk.MsgBox(nil, "ProxyDesk 启动失败", err.Error(), walk.MsgBoxIconError)
		os.Exit(1)
	}

	forceExit := false
	trayHintShown := false
	showMainWindow := func() {
		if mw == nil {
			return
		}
		mw.SetVisible(true)
		_ = mw.Activate()
	}
	quitApp := func() {
		forceExit = true
		if mw != nil {
			_ = mw.Close()
		}
	}

	notifyIcon, err := setupTrayIcon(mw, showMainWindow, quitApp)
	if err != nil {
		appendLog("托盘图标初始化失败：%v", err)
	} else {
		defer notifyIcon.Dispose()
		mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
			if forceExit {
				return
			}
			*canceled = true
			mw.SetVisible(false)
			if !trayHintShown {
				trayHintShown = true
				_ = notifyIcon.ShowInfo("ProxyDesk", "ProxyDesk 已最小化到系统托盘，右键托盘图标可以退出。")
			}
		})
	}

	exitCode := mw.Run()
	os.Exit(exitCode)
}

func setupTrayIcon(mw *walk.MainWindow, showMainWindow func(), quitApp func()) (*walk.NotifyIcon, error) {
	icon, err := walk.Resources.Icon("1")
	if err != nil {
		return nil, fmt.Errorf("load tray icon: %w", err)
	}
	notifyIcon, err := walk.NewNotifyIcon(mw)
	if err != nil {
		return nil, fmt.Errorf("create tray icon: %w", err)
	}
	if err := notifyIcon.SetIcon(icon); err != nil {
		notifyIcon.Dispose()
		return nil, fmt.Errorf("set tray icon: %w", err)
	}
	if err := notifyIcon.SetToolTip("ProxyDesk 正在运行"); err != nil {
		notifyIcon.Dispose()
		return nil, fmt.Errorf("set tray tooltip: %w", err)
	}

	notifyIcon.MouseUp().Attach(func(x, y int, button walk.MouseButton) {
		if button == walk.LeftButton {
			showMainWindow()
		}
	})

	showAction := walk.NewAction()
	if err := showAction.SetText("显示主窗口"); err != nil {
		notifyIcon.Dispose()
		return nil, fmt.Errorf("set tray show action: %w", err)
	}
	showAction.Triggered().Attach(showMainWindow)

	exitAction := walk.NewAction()
	if err := exitAction.SetText("退出 ProxyDesk"); err != nil {
		notifyIcon.Dispose()
		return nil, fmt.Errorf("set tray exit action: %w", err)
	}
	exitAction.Triggered().Attach(quitApp)

	actions := notifyIcon.ContextMenu().Actions()
	if err := actions.Add(showAction); err != nil {
		notifyIcon.Dispose()
		return nil, fmt.Errorf("add tray show action: %w", err)
	}
	if err := actions.Add(walk.NewSeparatorAction()); err != nil {
		notifyIcon.Dispose()
		return nil, fmt.Errorf("add tray separator: %w", err)
	}
	if err := actions.Add(exitAction); err != nil {
		notifyIcon.Dispose()
		return nil, fmt.Errorf("add tray exit action: %w", err)
	}
	if err := notifyIcon.SetVisible(true); err != nil {
		notifyIcon.Dispose()
		return nil, fmt.Errorf("show tray icon: %w", err)
	}
	return notifyIcon, nil
}

func writeStartupError(err error) {
	exe, exeErr := os.Executable()
	if exeErr != nil {
		exe = "ProxyDesk.exe"
	}
	logPath := filepath.Join(filepath.Dir(exe), "proxydesk-error.log")
	message := time.Now().Format(time.RFC3339) + "\r\n" + err.Error() + "\r\n"
	_ = os.WriteFile(logPath, []byte(message), 0644)
}

func splitCountry(value string) (string, string) {
	parts := strings.SplitN(value, " - ", 2)
	if len(parts) != 2 {
		return value, value
	}
	return parts[0], parts[1]
}

func defaultCountryIndex(countries []string, code string) int {
	prefix := code + " - "
	for i, country := range countries {
		if strings.HasPrefix(country, prefix) {
			return i
		}
	}
	return 0
}

func countryIndex(countries []string, value string) int {
	for i, country := range countries {
		if strings.EqualFold(country, value) {
			return i
		}
	}
	return -1
}

func filterCountries(countries []string, query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return append([]string{}, countries...)
	}
	filtered := []string{}
	for _, country := range countries {
		if strings.Contains(strings.ToLower(country), query) {
			filtered = append(filtered, country)
		}
	}
	return filtered
}

func environmentCountryDisplay(info publicIPInfo) string {
	ip := strings.TrimSpace(info.IP)
	countryCode := strings.TrimSpace(info.CountryCode)
	if countryCode == "" {
		country := strings.TrimSpace(info.Country)
		if len(country) == 2 {
			countryCode = strings.ToUpper(country)
		}
	}
	if ip != "" && countryCode != "" {
		return ip + " " + strings.ToUpper(countryCode)
	}
	if ip != "" {
		return ip
	}
	if countryCode != "" {
		return strings.ToUpper(countryCode)
	}
	return "-"
}

func allCountries() []string {
	return []string{
		"AF - Afghanistan",
		"AX - Aland Islands",
		"AL - Albania",
		"DZ - Algeria",
		"AS - American Samoa",
		"AD - Andorra",
		"AO - Angola",
		"AI - Anguilla",
		"AQ - Antarctica",
		"AG - Antigua and Barbuda",
		"AR - Argentina",
		"AM - Armenia",
		"AW - Aruba",
		"AU - Australia",
		"AT - Austria",
		"AZ - Azerbaijan",
		"BS - Bahamas",
		"BH - Bahrain",
		"BD - Bangladesh",
		"BB - Barbados",
		"BY - Belarus",
		"BE - Belgium",
		"BZ - Belize",
		"BJ - Benin",
		"BM - Bermuda",
		"BT - Bhutan",
		"BO - Bolivia",
		"BQ - Bonaire, Sint Eustatius and Saba",
		"BA - Bosnia and Herzegovina",
		"BW - Botswana",
		"BV - Bouvet Island",
		"BR - Brazil",
		"IO - British Indian Ocean Territory",
		"BN - Brunei Darussalam",
		"BG - Bulgaria",
		"BF - Burkina Faso",
		"BI - Burundi",
		"KH - Cambodia",
		"CM - Cameroon",
		"CA - Canada",
		"CV - Cape Verde",
		"KY - Cayman Islands",
		"CF - Central African Republic",
		"TD - Chad",
		"CL - Chile",
		"CN - China",
		"CX - Christmas Island",
		"CC - Cocos Islands",
		"CO - Colombia",
		"KM - Comoros",
		"CG - Congo",
		"CD - Congo, Democratic Republic",
		"CK - Cook Islands",
		"CR - Costa Rica",
		"CI - Cote d'Ivoire",
		"HR - Croatia",
		"CU - Cuba",
		"CW - Curacao",
		"CY - Cyprus",
		"CZ - Czech Republic",
		"DK - Denmark",
		"DJ - Djibouti",
		"DM - Dominica",
		"DO - Dominican Republic",
		"EC - Ecuador",
		"EG - Egypt",
		"SV - El Salvador",
		"GQ - Equatorial Guinea",
		"ER - Eritrea",
		"EE - Estonia",
		"SZ - Eswatini",
		"ET - Ethiopia",
		"FK - Falkland Islands",
		"FO - Faroe Islands",
		"FJ - Fiji",
		"FI - Finland",
		"FR - France",
		"GF - French Guiana",
		"PF - French Polynesia",
		"TF - French Southern Territories",
		"GA - Gabon",
		"GM - Gambia",
		"GE - Georgia",
		"DE - Germany",
		"GH - Ghana",
		"GI - Gibraltar",
		"GR - Greece",
		"GL - Greenland",
		"GD - Grenada",
		"GP - Guadeloupe",
		"GU - Guam",
		"GT - Guatemala",
		"GG - Guernsey",
		"GN - Guinea",
		"GW - Guinea-Bissau",
		"GY - Guyana",
		"HT - Haiti",
		"HM - Heard Island and McDonald Islands",
		"VA - Holy See",
		"HN - Honduras",
		"HK - Hong Kong",
		"HU - Hungary",
		"IS - Iceland",
		"IN - India",
		"ID - Indonesia",
		"IR - Iran",
		"IQ - Iraq",
		"IE - Ireland",
		"IM - Isle of Man",
		"IL - Israel",
		"IT - Italy",
		"JM - Jamaica",
		"JP - Japan",
		"JE - Jersey",
		"JO - Jordan",
		"KZ - Kazakhstan",
		"KE - Kenya",
		"KI - Kiribati",
		"KP - Korea, Democratic People's Republic",
		"KR - Korea, Republic",
		"KW - Kuwait",
		"KG - Kyrgyzstan",
		"LA - Lao People's Democratic Republic",
		"LV - Latvia",
		"LB - Lebanon",
		"LS - Lesotho",
		"LR - Liberia",
		"LY - Libya",
		"LI - Liechtenstein",
		"LT - Lithuania",
		"LU - Luxembourg",
		"MO - Macao",
		"MG - Madagascar",
		"MW - Malawi",
		"MY - Malaysia",
		"MV - Maldives",
		"ML - Mali",
		"MT - Malta",
		"MH - Marshall Islands",
		"MQ - Martinique",
		"MR - Mauritania",
		"MU - Mauritius",
		"YT - Mayotte",
		"MX - Mexico",
		"FM - Micronesia",
		"MD - Moldova",
		"MC - Monaco",
		"MN - Mongolia",
		"ME - Montenegro",
		"MS - Montserrat",
		"MA - Morocco",
		"MZ - Mozambique",
		"MM - Myanmar",
		"NA - Namibia",
		"NR - Nauru",
		"NP - Nepal",
		"NL - Netherlands",
		"NC - New Caledonia",
		"NZ - New Zealand",
		"NI - Nicaragua",
		"NE - Niger",
		"NG - Nigeria",
		"NU - Niue",
		"NF - Norfolk Island",
		"MK - North Macedonia",
		"MP - Northern Mariana Islands",
		"NO - Norway",
		"OM - Oman",
		"PK - Pakistan",
		"PW - Palau",
		"PS - Palestine",
		"PA - Panama",
		"PG - Papua New Guinea",
		"PY - Paraguay",
		"PE - Peru",
		"PH - Philippines",
		"PN - Pitcairn",
		"PL - Poland",
		"PT - Portugal",
		"PR - Puerto Rico",
		"QA - Qatar",
		"RE - Reunion",
		"RO - Romania",
		"RU - Russian Federation",
		"RW - Rwanda",
		"BL - Saint Barthelemy",
		"SH - Saint Helena",
		"KN - Saint Kitts and Nevis",
		"LC - Saint Lucia",
		"MF - Saint Martin",
		"PM - Saint Pierre and Miquelon",
		"VC - Saint Vincent and the Grenadines",
		"WS - Samoa",
		"SM - San Marino",
		"ST - Sao Tome and Principe",
		"SA - Saudi Arabia",
		"SN - Senegal",
		"RS - Serbia",
		"SC - Seychelles",
		"SL - Sierra Leone",
		"SG - Singapore",
		"SX - Sint Maarten",
		"SK - Slovakia",
		"SI - Slovenia",
		"SB - Solomon Islands",
		"SO - Somalia",
		"ZA - South Africa",
		"GS - South Georgia and the South Sandwich Islands",
		"SS - South Sudan",
		"ES - Spain",
		"LK - Sri Lanka",
		"SD - Sudan",
		"SR - Suriname",
		"SJ - Svalbard and Jan Mayen",
		"SE - Sweden",
		"CH - Switzerland",
		"SY - Syrian Arab Republic",
		"TW - Taiwan",
		"TJ - Tajikistan",
		"TZ - Tanzania",
		"TH - Thailand",
		"TL - Timor-Leste",
		"TG - Togo",
		"TK - Tokelau",
		"TO - Tonga",
		"TT - Trinidad and Tobago",
		"TN - Tunisia",
		"TR - Turkiye",
		"TM - Turkmenistan",
		"TC - Turks and Caicos Islands",
		"TV - Tuvalu",
		"UG - Uganda",
		"UA - Ukraine",
		"AE - United Arab Emirates",
		"GB - United Kingdom",
		"US - United States",
		"UM - United States Minor Outlying Islands",
		"UY - Uruguay",
		"UZ - Uzbekistan",
		"VU - Vanuatu",
		"VE - Venezuela",
		"VN - Viet Nam",
		"VG - Virgin Islands, British",
		"VI - Virgin Islands, U.S.",
		"WF - Wallis and Futuna",
		"EH - Western Sahara",
		"YE - Yemen",
		"ZM - Zambia",
		"ZW - Zimbabwe",
	}
}

func detectLANIP() string {
	conn, err := net.DialTimeout("udp", "8.8.8.8:80", 2*time.Second)
	if err == nil {
		defer conn.Close()
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok && isUsableLANIP(addr.IP) {
			return addr.IP.String()
		}
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if isUsableLANIP(ip) {
				return ip.String()
			}
		}
	}
	return "127.0.0.1"
}

func isUsableLANIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	ip = ip.To4()
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
		return false
	}
	return ip.IsPrivate()
}

func localConnectHost(route core.PortRoute) string {
	switch route.LocalHost {
	case "", "0.0.0.0":
		return "127.0.0.1"
	default:
		return route.LocalHost
	}
}

func checkIP(route core.PortRoute) (publicIPInfo, error) {
	host := localConnectHost(route)
	localAddr := net.JoinHostPort(host, strconv.Itoa(route.LocalHTTPPort))
	transport := &http.Transport{}
	switch route.LocalProtocol {
	case core.ProtocolHTTP:
		localProxyURL := "http://" + localAddr
		parsedProxyURL, err := url.Parse(localProxyURL)
		if err != nil {
			return publicIPInfo{}, err
		}
		transport.Proxy = http.ProxyURL(parsedProxyURL)
	case core.ProtocolSOCKS5:
		dialer, err := proxy.SOCKS5("tcp", localAddr, nil, proxy.Direct)
		if err != nil {
			return publicIPInfo{}, err
		}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}
	default:
		return publicIPInfo{}, fmt.Errorf("unsupported local protocol %s", route.LocalProtocol)
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	return fetchPublicIPInfo(client)
}

func checkUpstream(upstream core.UpstreamProxy) (publicIPInfo, error) {
	transport := &http.Transport{}
	switch upstream.Protocol {
	case core.ProtocolHTTP:
		upstreamURL := &url.URL{Scheme: "http", Host: upstream.Address()}
		if upstream.Username != "" || upstream.Password != "" {
			upstreamURL.User = url.UserPassword(upstream.Username, upstream.Password)
		}
		transport.Proxy = http.ProxyURL(upstreamURL)
	case core.ProtocolSOCKS5:
		dialer, err := socks5Dialer(upstream)
		if err != nil {
			return publicIPInfo{}, err
		}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}
	default:
		return publicIPInfo{}, fmt.Errorf("unsupported upstream protocol %s", upstream.Protocol)
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	return fetchPublicIPInfo(client)
}

func socks5Dialer(upstream core.UpstreamProxy) (proxy.Dialer, error) {
	var auth *proxy.Auth
	if upstream.Username != "" || upstream.Password != "" {
		auth = &proxy.Auth{
			User:     upstream.Username,
			Password: upstream.Password,
		}
	}
	return proxy.SOCKS5("tcp", upstream.Address(), auth, proxy.Direct)
}

func fetchPublicIPInfo(client *http.Client) (publicIPInfo, error) {
	checkURLs := []string{
		"http://ip-api.com/json/?fields=status,message,query,country,regionName,city,countryCode",
		"http://ipinfo.io/json",
		"http://api.ipify.org?format=json",
		"http://ipinfo.io/ip",
		"http://icanhazip.com",
		"https://ip-api.com/json/?fields=status,message,query,country,regionName,city,countryCode",
		"https://ipinfo.io/json",
		"https://api.ipify.org?format=json",
		"https://ipinfo.io/ip",
		"https://icanhazip.com",
	}
	var errs []string
	for _, checkURL := range checkURLs {
		info, err := fetchPublicIPInfoFrom(client, checkURL)
		if err == nil && info.IP != "" {
			return info, nil
		}
		if err != nil {
			errs = append(errs, checkURL+": "+err.Error())
		}
	}
	return publicIPInfo{}, fmt.Errorf("all IP check endpoints failed: %s", strings.Join(errs, " | "))
}

func fetchPublicIPInfoFrom(client *http.Client, checkURL string) (publicIPInfo, error) {
	reqClient := *client
	reqClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	currentURL := checkURL
	var resp *http.Response
	var err error
	for range 5 {
		resp, err = reqClient.Get(currentURL)
		if err != nil {
			return publicIPInfo{}, err
		}
		if resp.StatusCode < 300 || resp.StatusCode >= 400 {
			break
		}
		location := resp.Header.Get("Location")
		_ = resp.Body.Close()
		if location == "" {
			return publicIPInfo{}, fmt.Errorf("redirect without Location: %s", resp.Status)
		}
		nextURL, err := url.Parse(location)
		if err != nil {
			return publicIPInfo{}, err
		}
		if !nextURL.IsAbs() {
			baseURL, err := url.Parse(currentURL)
			if err != nil {
				return publicIPInfo{}, err
			}
			nextURL = baseURL.ResolveReference(nextURL)
		}
		currentURL = nextURL.String()
	}
	if err != nil {
		return publicIPInfo{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return publicIPInfo{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return publicIPInfo{}, fmt.Errorf("status %s", resp.Status)
	}
	text := strings.TrimSpace(string(body))
	if strings.HasPrefix(text, "{") {
		var payload struct {
			IP          string `json:"ip"`
			Query       string `json:"query"`
			Country     string `json:"country"`
			CountryCode string `json:"countryCode"`
			Region      string `json:"region"`
			RegionName  string `json:"regionName"`
			City        string `json:"city"`
			Status      string `json:"status"`
			Message     string `json:"message"`
		}
		if err := json.Unmarshal(body, &payload); err == nil {
			if payload.Status == "fail" {
				return publicIPInfo{}, fmt.Errorf("%s", payload.Message)
			}
			info := publicIPInfo{
				IP:          firstNonEmpty(payload.IP, payload.Query),
				Country:     payload.Country,
				CountryCode: payload.CountryCode,
				Region:      firstNonEmpty(payload.RegionName, payload.Region),
				City:        payload.City,
			}
			if info.IP != "" {
				if net.ParseIP(info.IP) == nil {
					return publicIPInfo{}, fmt.Errorf("unexpected ip %q", trimForError(info.IP))
				}
				return info, nil
			}
		}
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return publicIPInfo{}, fmt.Errorf("empty response")
	}
	if net.ParseIP(fields[0]) == nil {
		return publicIPInfo{}, fmt.Errorf("unexpected response %q", trimForError(text))
	}
	return publicIPInfo{IP: fields[0]}, nil
}

func trimForError(text string) string {
	text = strings.TrimSpace(text)
	if len(text) > 120 {
		return text[:120] + "..."
	}
	return text
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
