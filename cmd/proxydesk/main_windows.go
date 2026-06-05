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
	IP      string
	Country string
	Region  string
	City    string
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
	countries := []string{"US - United States", "JP - Japan", "GB - United Kingdom", "DE - Germany", "SG - Singapore", "BR - Brazil", "IN - India"}
	detectedLANIP := detectLANIP()

	var mw *walk.MainWindow
	var countryCB, localProtocolCB, protocolCB *walk.ComboBox
	var listenHostEdit, portEdit, apiEndpoint, apiCountryParam, apiJSONKey *walk.LineEdit
	var upstreamEdit, logBox *walk.TextEdit
	var routeList *walk.ListBox
	var contentTitle *walk.Label
	var dashboardPage, configPage, routePage, apiPage, logPage *walk.Composite
	var statusLabel, exitIPLabel, upstreamLabel, errorLabel, localProtocolLabel, upstreamProtocolLabel *walk.Label
	var envExitLabel, localIPLabel *walk.Label
	var configCountryLabel, actualExitLabel *walk.Label
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
		idx := countryCB.CurrentIndex()
		if idx < 0 || idx >= len(countries) {
			return countries[0]
		}
		return countries[idx]
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
		configCountry := strings.TrimSpace(rt.route.CountryCode)
		if configCountry == "" {
			configCountry = "-"
		}
		return fmt.Sprintf("[%s] %s:%d  本地:%s  上游:%s %s  配置:%s  实际出口:%s",
			status,
			rt.route.LocalHost,
			rt.route.LocalHTTPPort,
			rt.route.LocalProtocol,
			rt.route.Upstream.Protocol,
			rt.route.Upstream.Address(),
			configCountry,
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
		if configCountryLabel != nil {
			configCountry := strings.TrimSpace(route.CountryCode + " " + route.CountryName)
			if configCountry == "" {
				configCountry = "-"
			}
			_ = configCountryLabel.SetText(configCountry)
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
		_ = portEdit.SetText(strconv.Itoa(route.LocalHTTPPort))
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
		port, err := strconv.Atoi(strings.TrimSpace(portEdit.Text()))
		if err != nil || port < 1 || port > 65535 {
			return core.PortRoute{}, fmt.Errorf("端口需要在 1-65535 之间")
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

		countryCode, countryName := splitCountry(selectedCountry())
		return core.PortRoute{
			ID:            countryCode + "-" + strconv.Itoa(port),
			Name:          countryName + " " + strconv.Itoa(port),
			CountryCode:   countryCode,
			CountryName:   countryName,
			LocalHost:     listenHost,
			LocalHTTPPort: port,
			LocalProtocol: localProtocol,
			Protocol:      upstreamProtocol,
			Upstream:      upstream,
			Enabled:       true,
			UpdatedAt:     time.Now(),
		}, nil
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
		state.routes = append(state.routes, routeRuntime{route: route})
		state.selected = len(state.routes) - 1
		appendLog("已新增转发配置：%s:%d", route.LocalHost, route.LocalHTTPPort)
		refreshRouteList()
		showRoute(route, false)
	}
	updateRoute := func() {
		idx := state.selected
		if routeList != nil && routeList.CurrentIndex() >= 0 {
			idx = routeList.CurrentIndex()
		}
		if idx < 0 || idx >= len(state.routes) {
			walk.MsgBox(mw, "提示", "请先在转发列表中选择一条配置", walk.MsgBoxIconInformation)
			return
		}
		route, err := buildRoute()
		if err != nil {
			walk.MsgBox(mw, "配置无效", err.Error(), walk.MsgBoxIconError)
			return
		}
		if state.routes[idx].running() {
			_ = state.routes[idx].server.Stop(context.Background())
		}
		state.routes[idx] = routeRuntime{route: route}
		state.selected = idx
		appendLog("已更新转发配置：%s:%d", route.LocalHost, route.LocalHTTPPort)
		refreshRouteList()
		showRoute(route, false)
	}
	startRoute := func() {
		idx := state.selected
		if routeList != nil && routeList.CurrentIndex() >= 0 {
			idx = routeList.CurrentIndex()
		}
		if idx < 0 || idx >= len(state.routes) {
			route, err := buildRoute()
			if err != nil {
				walk.MsgBox(mw, "启动失败", err.Error(), walk.MsgBoxIconError)
				return
			}
			state.routes = append(state.routes, routeRuntime{route: route})
			idx = len(state.routes) - 1
			state.selected = idx
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

	testUpstream := func() {
		route, err := buildRoute()
		if err != nil {
			walk.MsgBox(mw, "上游代理无效", err.Error(), walk.MsgBoxIconError)
			return
		}
		info, err := checkUpstream(route.Upstream)
		if err != nil {
			_ = errorLabel.SetText(err.Error())
			appendLog("上游检测失败：%v", err)
			walk.MsgBox(mw, "上游检测失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		_ = exitIPLabel.SetText(info.Display())
		_ = upstreamLabel.SetText(proxyparse.Format(route.Upstream))
		_ = errorLabel.SetText("-")
		appendLog("上游检测成功：%s", info.Display())
	}

	fetchAPI := func() {
		countryCode, _ := splitCountry(selectedCountry())
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
		upstream, err := client.Fetch(ctx, countryCode, selectedUpstreamProtocol())
		if err != nil {
			_ = errorLabel.SetText(err.Error())
			appendLog("API 获取失败：%v", err)
			walk.MsgBox(mw, "API 获取失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		_ = upstreamEdit.SetText(proxyparse.Format(upstream))
		_ = errorLabel.SetText("-")
		appendLog("API 获取成功：%s %s", countryCode, upstream.Address())
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
	pageNames := []string{"工作台", "线路配置", "转发列表", "供应商 API", "运行日志"}
	openPage := func(index int) func() {
		return func() {
			pages := []*walk.Composite{dashboardPage, configPage, routePage, apiPage, logPage}
			if index < 0 || index >= len(pages) {
				return
			}
			for i, page := range pages {
				if page != nil {
					page.SetVisible(i == index)
				}
			}
			if contentTitle != nil {
				_ = contentTitle.SetText(pageNames[index])
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
			_ = envExitLabel.SetText(info.Display())
		})
	}
	time.AfterFunc(300*time.Millisecond, updateEnvironmentExit)

	exitCode, err := MainWindow{
		AssignTo:   &mw,
		Title:      "ProxyDesk",
		MinSize:    Size{Width: 1040, Height: 700},
		Size:       Size{Width: 1180, Height: 760},
		Font:       Font{Family: "Microsoft YaHei UI", PointSize: 9},
		Background: SolidColorBrush{Color: walk.RGB(237, 252, 247)},
		Layout:     VBox{Margins: Margins{Left: 18, Top: 16, Right: 18, Bottom: 16}, Spacing: 12},
		Children: []Widget{
			Composite{
				Background: SolidColorBrush{Color: walk.RGB(223, 249, 241)},
				Layout:     VBox{Margins: Margins{Left: 22, Top: 16, Right: 22, Bottom: 16}, Spacing: 8},
				Children: []Widget{
					Composite{
						Layout: HBox{MarginsZero: true, Spacing: 12},
						Children: []Widget{
							Composite{
								Layout: VBox{MarginsZero: true, Spacing: 2},
								Children: []Widget{
									Label{
										Text:      "ProxyDesk",
										Font:      Font{Family: "Microsoft YaHei UI", PointSize: 18, Bold: true},
										TextColor: walk.RGB(11, 47, 71),
									},
									Label{
										Text:      "国家住宅 IP 本地端口转发器",
										TextColor: walk.RGB(37, 99, 105),
									},
								},
							},
							HSpacer{},
							Composite{
								MinSize:    Size{Width: 260, Height: 56},
								Layout:     VBox{Margins: Margins{Left: 14, Top: 8, Right: 14, Bottom: 8}, Spacing: 2},
								Background: SolidColorBrush{Color: walk.RGB(202, 245, 233)},
								Children: []Widget{
									Label{Text: "当前环境出口", TextColor: walk.RGB(15, 94, 91)},
									Label{
										AssignTo:  &envExitLabel,
										Text:      "检测中...",
										Font:      Font{Family: "Consolas", PointSize: 12, Bold: true},
										TextColor: walk.RGB(14, 116, 101),
									},
								},
							},
							Composite{
								MinSize:    Size{Width: 190, Height: 56},
								Layout:     VBox{Margins: Margins{Left: 14, Top: 8, Right: 14, Bottom: 8}, Spacing: 2},
								Background: SolidColorBrush{Color: walk.RGB(202, 245, 233)},
								Children: []Widget{
									Label{Text: "本地 IP", TextColor: walk.RGB(15, 94, 91)},
									Label{
										AssignTo:  &localIPLabel,
										Text:      detectedLANIP,
										Font:      Font{Family: "Consolas", PointSize: 12, Bold: true},
										TextColor: walk.RGB(14, 116, 101),
									},
								},
							},
						},
					},
					HSeparator{},
					Composite{
						Layout: HBox{MarginsZero: true, Spacing: 14},
						Children: []Widget{
							Label{Text: "状态"},
							Label{
								AssignTo:  &statusLabel,
								Text:      "未启动",
								Font:      Font{Family: "Microsoft YaHei UI", PointSize: 10, Bold: true},
								TextColor: walk.RGB(123, 94, 0),
							},
							Label{Text: "出口 IP"},
							Label{AssignTo: &exitIPLabel, Text: "-", TextColor: walk.RGB(14, 116, 101)},
							Label{Text: "运行本地协议"},
							Label{AssignTo: &localProtocolLabel, Text: "HTTP/HTTPS", TextColor: walk.RGB(14, 116, 101)},
							Label{Text: "运行上游协议"},
							Label{AssignTo: &upstreamProtocolLabel, Text: "HTTP", TextColor: walk.RGB(14, 116, 101)},
						},
					},
				},
			},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 0},
				Children: []Widget{
					Composite{
						MinSize:    Size{Width: 154, Height: 520},
						MaxSize:    Size{Width: 154},
						Background: SolidColorBrush{Color: walk.RGB(18, 27, 43)},
						Layout:     VBox{Margins: Margins{Left: 12, Top: 18, Right: 12, Bottom: 18}, Spacing: 10},
						Children: []Widget{
							Label{Text: "ProxyDesk", Font: Font{Family: "Microsoft YaHei UI", PointSize: 12, Bold: true}, TextColor: walk.RGB(236, 253, 245)},
							Label{Text: "端口转发", TextColor: walk.RGB(148, 163, 184)},
							VSpacer{Size: 8},
							PushButton{Text: "概览", MinSize: Size{Height: 34}, Background: SolidColorBrush{Color: walk.RGB(35, 180, 150)}, OnClicked: openPage(0)},
							PushButton{Text: "线路配置", MinSize: Size{Height: 34}, OnClicked: openPage(1)},
							PushButton{Text: "转发列表", MinSize: Size{Height: 34}, OnClicked: openPage(2)},
							PushButton{Text: "供应商 API", MinSize: Size{Height: 34}, OnClicked: openPage(3)},
							PushButton{Text: "运行日志", MinSize: Size{Height: 34}, OnClicked: openPage(4)},
							VSpacer{},
							Label{Text: "实际国家看出口检测", TextColor: walk.RGB(148, 163, 184)},
						},
					},
					Composite{
						MinSize:       Size{Width: 760, Height: 520},
						StretchFactor: 1,
						Background:    SolidColorBrush{Color: walk.RGB(247, 255, 252)},
						Layout:        VBox{Margins: Margins{Left: 12, Top: 10, Right: 12, Bottom: 10}, Spacing: 8},
						Children: []Widget{
							Label{AssignTo: &contentTitle, Text: "工作台", Font: Font{Family: "Microsoft YaHei UI", PointSize: 13, Bold: true}, TextColor: walk.RGB(11, 47, 71)},
							Composite{
								AssignTo: &dashboardPage,
								Layout:   VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}, Spacing: 12},
								Children: []Widget{
									GroupBox{
										Title:      "当前连接",
										Layout:     VBox{Margins: Margins{Left: 16, Top: 12, Right: 16, Bottom: 12}, Spacing: 10},
										Background: SolidColorBrush{Color: walk.RGB(250, 255, 253)},
										Children: []Widget{
											Composite{
												Layout: Grid{Columns: 2, MarginsZero: true, Spacing: 8},
												Children: []Widget{
													Label{Text: "配置国家/地区", TextColor: walk.RGB(71, 85, 105)},
													Label{AssignTo: &configCountryLabel, Text: "-", TextColor: walk.RGB(15, 118, 110), EllipsisMode: EllipsisEnd},
													Label{Text: "实际出口", TextColor: walk.RGB(71, 85, 105)},
													Label{AssignTo: &actualExitLabel, Text: "-", TextColor: walk.RGB(15, 118, 110), EllipsisMode: EllipsisEnd},
													Label{Text: "上游代理", TextColor: walk.RGB(71, 85, 105)},
													Label{AssignTo: &upstreamLabel, Text: "-", TextColor: walk.RGB(15, 23, 42), EllipsisMode: EllipsisEnd},
													Label{Text: "最近错误", TextColor: walk.RGB(71, 85, 105)},
													Label{AssignTo: &errorLabel, Text: "-", TextColor: walk.RGB(185, 28, 28), EllipsisMode: EllipsisEnd},
												},
											},
											HSeparator{},
											Label{Text: "系统代理", Font: Font{Family: "Microsoft YaHei UI", PointSize: 10, Bold: true}, TextColor: walk.RGB(23, 37, 84)},
											Label{Text: "需要让浏览器或多数桌面软件直接走代理时，可开启 Windows 系统代理。", TextColor: walk.RGB(100, 116, 139)},
											Composite{
												Layout: HBox{MarginsZero: true, Spacing: 8},
												Children: []Widget{
													PushButton{Text: "开启系统代理", MinSize: Size{Width: 130, Height: 32}, Background: SolidColorBrush{Color: walk.RGB(35, 180, 150)}, OnClicked: enableSystemProxy},
													PushButton{Text: "关闭系统代理", MinSize: Size{Width: 130, Height: 32}, OnClicked: disableSystemProxy},
													PushButton{Text: "测试选中出口", MinSize: Size{Width: 130, Height: 32}, OnClicked: testExitIP},
													HSpacer{},
												},
											},
										},
									},
									GroupBox{
										Title:      "使用提示",
										Layout:     VBox{Margins: Margins{Left: 16, Top: 12, Right: 16, Bottom: 12}, Spacing: 8},
										Background: SolidColorBrush{Color: walk.RGB(250, 255, 253)},
										Children: []Widget{
											Label{Text: "其他设备使用“本地 IP”加对应端口；工具需要 SOCKS5 时，本地协议请选择 SOCKS5。", TextColor: walk.RGB(37, 99, 105)},
											Label{Text: "多条运行中的端口可以同时给不同浏览器、指纹浏览器或桌面工具使用。", TextColor: walk.RGB(37, 99, 105)},
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
										Background: SolidColorBrush{Color: walk.RGB(250, 255, 253)},
										Children: []Widget{
											Composite{
												Layout: Grid{Columns: 2, MarginsZero: true, Spacing: 8},
												Children: []Widget{
													Label{Text: "配置国家/地区", TextColor: walk.RGB(71, 85, 105)},
													ComboBox{AssignTo: &countryCB, Model: countries, CurrentIndex: 0, MinSize: Size{Height: 26}},
													Label{Text: "本地协议", TextColor: walk.RGB(71, 85, 105)},
													ComboBox{AssignTo: &localProtocolCB, Model: []string{"HTTP/HTTPS", "SOCKS5"}, CurrentIndex: 0, MinSize: Size{Height: 26}, OnCurrentIndexChanged: markConfigChanged},
													Label{Text: "上游协议", TextColor: walk.RGB(71, 85, 105)},
													ComboBox{AssignTo: &protocolCB, Model: []string{"HTTP", "SOCKS5"}, CurrentIndex: 0, MinSize: Size{Height: 26}, OnCurrentIndexChanged: markConfigChanged},
													Label{Text: "监听地址", TextColor: walk.RGB(71, 85, 105)},
													LineEdit{AssignTo: &listenHostEdit, Text: detectedLANIP, MinSize: Size{Height: 26}, OnTextChanged: markConfigChanged},
													Label{Text: "本地端口", TextColor: walk.RGB(71, 85, 105)},
													LineEdit{AssignTo: &portEdit, Text: "7890", MinSize: Size{Height: 26}, OnTextChanged: markConfigChanged},
												},
											},
											Label{Text: "上游代理", TextColor: walk.RGB(71, 85, 105)},
											TextEdit{AssignTo: &upstreamEdit, MinSize: Size{Height: 170}, OnTextChanged: markConfigChanged},
											Label{Text: "配置国家用于供应商 API 或线路标记；实际国家/地区以“测试出口”检测结果为准。", TextColor: walk.RGB(100, 116, 139)},
											Composite{
												Layout: HBox{MarginsZero: true, Spacing: 8},
												Children: []Widget{
													PushButton{Text: "新增配置", MinSize: Size{Width: 90, Height: 32}, Background: SolidColorBrush{Color: walk.RGB(35, 180, 150)}, OnClicked: addRoute},
													PushButton{Text: "更新选中", MinSize: Size{Width: 90, Height: 32}, OnClicked: updateRoute},
													PushButton{Text: "启动选中", MinSize: Size{Width: 120, Height: 32}, Background: SolidColorBrush{Color: walk.RGB(35, 180, 150)}, Font: Font{Family: "Microsoft YaHei UI", PointSize: 9, Bold: true}, OnClicked: startRoute},
													PushButton{Text: "停止选中", MinSize: Size{Width: 90, Height: 32}, OnClicked: stopRoute},
													PushButton{Text: "测试当前上游", MinSize: Size{Width: 110, Height: 32}, OnClicked: testUpstream},
													PushButton{Text: "测试选中出口", MinSize: Size{Width: 110, Height: 32}, OnClicked: testExitIP},
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
										Background: SolidColorBrush{Color: walk.RGB(250, 255, 253)},
										Children: []Widget{
											Label{Text: "选中哪一条，测试选中出口、开启系统代理就针对哪一条。列表中的“实际出口”会显示检测到的国家/地区。", TextColor: walk.RGB(37, 99, 105)},
											ListBox{
												AssignTo:              &routeList,
												Model:                 []string{},
												MinSize:               Size{Height: 280},
												OnCurrentIndexChanged: loadSelectedRoute,
											},
											Composite{
												Layout: HBox{MarginsZero: true, Spacing: 8},
												Children: []Widget{
													PushButton{Text: "启动选中", MinSize: Size{Width: 110, Height: 30}, Background: SolidColorBrush{Color: walk.RGB(35, 180, 150)}, OnClicked: startRoute},
													PushButton{Text: "停止选中", MinSize: Size{Width: 110, Height: 30}, OnClicked: stopRoute},
													PushButton{Text: "测试选中出口", MinSize: Size{Width: 120, Height: 30}, OnClicked: testExitIP},
													PushButton{Text: "删除选中", MinSize: Size{Width: 110, Height: 30}, OnClicked: deleteRoute},
													PushButton{Text: "停止全部", MinSize: Size{Width: 110, Height: 30}, OnClicked: stopAllRoutes},
													HSpacer{},
												},
											},
										},
									},
								},
							},
							Composite{
								AssignTo: &apiPage,
								Visible:  false,
								Layout:   VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}, Spacing: 10},
								Children: []Widget{
									GroupBox{
										Title:      "供应商 API",
										Layout:     VBox{Margins: Margins{Left: 14, Top: 12, Right: 14, Bottom: 12}, Spacing: 10},
										Background: SolidColorBrush{Color: walk.RGB(250, 255, 253)},
										Children: []Widget{
											Composite{
												Layout: Grid{Columns: 2, MarginsZero: true, Spacing: 8},
												Children: []Widget{
													Label{Text: "API 地址"},
													LineEdit{AssignTo: &apiEndpoint, MinSize: Size{Height: 26}},
													Label{Text: "国家参数"},
													LineEdit{AssignTo: &apiCountryParam, MinSize: Size{Height: 26}},
													Label{Text: "JSON 字段"},
													LineEdit{AssignTo: &apiJSONKey, MinSize: Size{Height: 26}},
												},
											},
											Label{Text: "如果供应商支持国家参数，这里会按上方“配置国家/地区”请求；实际出口仍建议测试确认。", TextColor: walk.RGB(100, 116, 139)},
											Composite{
												Layout: HBox{MarginsZero: true},
												Children: []Widget{
													HSpacer{},
													PushButton{Text: "按国家获取 IP", MinSize: Size{Width: 150, Height: 32}, Background: SolidColorBrush{Color: walk.RGB(35, 180, 150)}, OnClicked: fetchAPI},
												},
											},
										},
									},
								},
							},
							Composite{
								AssignTo: &logPage,
								Visible:  false,
								Layout:   VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}, Spacing: 8},
								Children: []Widget{
									Composite{
										Layout: HBox{MarginsZero: true},
										Children: []Widget{
											Label{Text: "运行日志会自动滚动到底部，可手动滑动查看历史。", TextColor: walk.RGB(37, 99, 105)},
											HSpacer{},
											PushButton{Text: "清理日志", MinSize: Size{Width: 100, Height: 28}, OnClicked: clearLogs},
										},
									},
									TextEdit{
										AssignTo: &logBox,
										ReadOnly: true,
										MinSize:  Size{Height: 430},
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
	}.Run()
	if err != nil {
		writeStartupError(err)
		walk.MsgBox(nil, "ProxyDesk 启动失败", err.Error(), walk.MsgBoxIconError)
		os.Exit(1)
	}
	os.Exit(exitCode)
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
				IP:      firstNonEmpty(payload.IP, payload.Query),
				Country: firstNonEmpty(payload.Country, payload.CountryCode),
				Region:  firstNonEmpty(payload.RegionName, payload.Region),
				City:    payload.City,
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
