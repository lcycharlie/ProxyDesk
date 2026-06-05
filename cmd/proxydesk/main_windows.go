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
	server interface {
		Stop(context.Context) error
	}
	route core.PortRoute
}

func main() {
	state := &runtimeState{}
	countries := []string{"US - United States", "JP - Japan", "GB - United Kingdom", "DE - Germany", "SG - Singapore", "BR - Brazil", "IN - India"}
	detectedLANIP := detectLANIP()

	var mw *walk.MainWindow
	var countryCB, localProtocolCB, protocolCB *walk.ComboBox
	var listenHostEdit, portEdit, apiEndpoint, apiCountryParam, apiJSONKey *walk.LineEdit
	var upstreamEdit, logBox *walk.TextEdit
	var statusLabel, exitIPLabel, upstreamLabel, localLabel, errorLabel, localProtocolLabel, upstreamProtocolLabel *walk.Label

	appendLogDirect := func(format string, args ...any) {
		if logBox == nil {
			return
		}
		line := time.Now().Format("15:04:05") + "  " + fmt.Sprintf(format, args...)
		current := strings.TrimSpace(logBox.Text())
		if current != "" {
			current += "\r\n"
		}
		_ = logBox.SetText(current + line)
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
		if state.server == nil || statusLabel == nil {
			return
		}
		_ = statusLabel.SetText("配置已变更，需重启")
		statusLabel.SetTextColor(walk.RGB(185, 100, 0))
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

	startRoute := func() {
		route, err := buildRoute()
		if err != nil {
			walk.MsgBox(mw, "启动失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		if state.server != nil {
			_ = state.server.Stop(context.Background())
		}
		var server interface {
			Start() error
			Stop(context.Context) error
		}
		switch route.LocalProtocol {
		case core.ProtocolHTTP:
			httpServer := localproxy.NewHTTPServer(route)
			httpServer.OnLog = appendLog
			server = httpServer
		case core.ProtocolSOCKS5:
			socksServer := localproxy.NewSOCKS5Server(route)
			socksServer.OnLog = appendLog
			server = socksServer
		default:
			walk.MsgBox(mw, "启动失败", "不支持的本地协议："+string(route.LocalProtocol), walk.MsgBoxIconError)
			return
		}
		if err := server.Start(); err != nil {
			_ = errorLabel.SetText(err.Error())
			walk.MsgBox(mw, "启动失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		state.server = server
		state.route = route
		_ = statusLabel.SetText("运行中")
		statusLabel.SetTextColor(walk.RGB(22, 120, 75))
		updateRunningProtocolLabels(route)
		_ = localLabel.SetText(route.LocalHost + ":" + strconv.Itoa(route.LocalHTTPPort))
		_ = upstreamLabel.SetText(proxyparse.Format(route.Upstream))
		_ = errorLabel.SetText("-")
		appendLog("已启动本地 %s 代理 %s -> %s 上游 %s", route.LocalProtocol, localLabel.Text(), route.Upstream.Protocol, route.Upstream.Address())
		if route.LocalHost == "0.0.0.0" {
			appendLog("局域网设备请使用这台 Windows 电脑的内网 IP:%d 作为 %s 代理", route.LocalHTTPPort, route.LocalProtocol)
		}
	}

	stopRoute := func() {
		if state.server == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := state.server.Stop(ctx); err != nil {
			walk.MsgBox(mw, "停止失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		state.server = nil
		_ = statusLabel.SetText("已停止")
		statusLabel.SetTextColor(walk.RGB(123, 94, 0))
		appendLog("已停止本地转发")
	}

	testExitIP := func() {
		if state.server == nil {
			walk.MsgBox(mw, "提示", "请先启动本地转发", walk.MsgBoxIconInformation)
			return
		}
		ip, err := checkIP(state.route)
		if err != nil {
			_ = errorLabel.SetText(err.Error())
			appendLog("出口检测失败：%v", err)
			return
		}
		_ = exitIPLabel.SetText(ip)
		_ = errorLabel.SetText("-")
		appendLog("出口检测成功：%s", ip)
	}

	testUpstream := func() {
		route, err := buildRoute()
		if err != nil {
			walk.MsgBox(mw, "上游代理无效", err.Error(), walk.MsgBoxIconError)
			return
		}
		ip, err := checkUpstream(route.Upstream)
		if err != nil {
			_ = errorLabel.SetText(err.Error())
			appendLog("上游检测失败：%v", err)
			walk.MsgBox(mw, "上游检测失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		_ = exitIPLabel.SetText(ip)
		_ = upstreamLabel.SetText(proxyparse.Format(route.Upstream))
		_ = errorLabel.SetText("-")
		appendLog("上游检测成功：%s", ip)
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
		route, err := buildRoute()
		if err != nil {
			walk.MsgBox(mw, "系统代理失败", err.Error(), walk.MsgBoxIconError)
			return
		}
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

	exitCode, err := MainWindow{
		AssignTo: &mw,
		Title:    "ProxyDesk",
		MinSize:  Size{Width: 1040, Height: 700},
		Size:     Size{Width: 1180, Height: 760},
		Font:     Font{Family: "Microsoft YaHei UI", PointSize: 9},
		Background: SolidColorBrush{
			Color: walk.RGB(244, 247, 251),
		},
		Layout: VBox{Margins: Margins{Left: 18, Top: 16, Right: 18, Bottom: 16}, Spacing: 12},
		Children: []Widget{
			Composite{
				Background: SolidColorBrush{Color: walk.RGB(255, 255, 255)},
				Layout:     VBox{Margins: Margins{Left: 18, Top: 14, Right: 18, Bottom: 14}, Spacing: 8},
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
										TextColor: walk.RGB(23, 37, 84),
									},
									Label{
										Text:      "国家住宅 IP 本地端口转发器",
										TextColor: walk.RGB(82, 95, 127),
									},
								},
							},
							HSpacer{},
							Composite{
								MinSize: Size{Width: 220, Height: 56},
								Layout:  VBox{Margins: Margins{Left: 14, Top: 8, Right: 14, Bottom: 8}, Spacing: 2},
								Background: SolidColorBrush{
									Color: walk.RGB(239, 246, 255),
								},
								Children: []Widget{
									Label{Text: "当前本地代理", TextColor: walk.RGB(65, 85, 125)},
									Label{
										AssignTo:  &localLabel,
										Text:      detectedLANIP + ":7890",
										Font:      Font{Family: "Consolas", PointSize: 12, Bold: true},
										TextColor: walk.RGB(29, 78, 216),
									},
								},
							},
						},
					},
					HSeparator{},
					Composite{
						Layout: HBox{MarginsZero: true, Spacing: 8},
						Children: []Widget{
							Label{Text: "状态"},
							Label{
								AssignTo:  &statusLabel,
								Text:      "未启动",
								Font:      Font{Family: "Microsoft YaHei UI", PointSize: 10, Bold: true},
								TextColor: walk.RGB(123, 94, 0),
							},
							VSeparator{},
							Label{Text: "出口 IP"},
							Label{AssignTo: &exitIPLabel, Text: "-", TextColor: walk.RGB(30, 64, 175)},
							VSeparator{},
							Label{Text: "运行本地协议"},
							Label{AssignTo: &localProtocolLabel, Text: "HTTP/HTTPS", TextColor: walk.RGB(30, 64, 175)},
							VSeparator{},
							Label{Text: "运行上游协议"},
							Label{AssignTo: &upstreamProtocolLabel, Text: "HTTP", TextColor: walk.RGB(30, 64, 175)},
						},
					},
				},
			},
			HSplitter{
				HandleWidth: 6,
				Children: []Widget{
					GroupBox{
						Title:   "线路配置",
						MinSize: Size{Width: 520, Height: 290},
						Layout:  VBox{Margins: Margins{Left: 14, Top: 12, Right: 14, Bottom: 12}, Spacing: 10},
						Children: []Widget{
							Composite{
								Layout: Grid{Columns: 2, MarginsZero: true, Spacing: 8},
								Children: []Widget{
									Label{Text: "国家/地区", TextColor: walk.RGB(71, 85, 105)},
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
							TextEdit{AssignTo: &upstreamEdit, MinSize: Size{Width: 460, Height: 120}, OnTextChanged: markConfigChanged},
							Label{Text: "其他设备按本地协议连接内网 IP:端口；要给工具用 SOCKS5，请把本地协议选 SOCKS5。", TextColor: walk.RGB(100, 116, 139)},
							Composite{
								Layout: HBox{MarginsZero: true, Spacing: 8},
								Children: []Widget{
									PushButton{
										Text:      "启动转发",
										MinSize:   Size{Width: 120, Height: 32},
										Font:      Font{Family: "Microsoft YaHei UI", PointSize: 9, Bold: true},
										OnClicked: startRoute,
									},
									PushButton{Text: "停止", MinSize: Size{Width: 90, Height: 32}, OnClicked: stopRoute},
									PushButton{Text: "测试上游", MinSize: Size{Width: 96, Height: 32}, OnClicked: testUpstream},
									PushButton{Text: "测试出口", MinSize: Size{Width: 96, Height: 32}, OnClicked: testExitIP},
									HSpacer{},
								},
							},
						},
					},
					GroupBox{
						Title:   "连接详情",
						MinSize: Size{Width: 430, Height: 290},
						Layout:  VBox{Margins: Margins{Left: 14, Top: 12, Right: 14, Bottom: 12}, Spacing: 10},
						Children: []Widget{
							Composite{
								Layout: Grid{Columns: 2, MarginsZero: true, Spacing: 8},
								Children: []Widget{
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
									PushButton{Text: "开启系统代理", MinSize: Size{Width: 130, Height: 32}, OnClicked: enableSystemProxy},
									PushButton{Text: "关闭系统代理", MinSize: Size{Width: 130, Height: 32}, OnClicked: disableSystemProxy},
									HSpacer{},
								},
							},
						},
					},
				},
			},
			TabWidget{
				MinSize: Size{Height: 230},
				Pages: []TabPage{
					{
						Title:  "供应商 API",
						Layout: VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}, Spacing: 10},
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
							Composite{
								Layout: HBox{MarginsZero: true},
								Children: []Widget{
									HSpacer{},
									PushButton{Text: "按国家获取 IP", MinSize: Size{Width: 150, Height: 32}, OnClicked: fetchAPI},
								},
							},
						},
					},
					{
						Title:  "运行日志",
						Layout: VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}},
						Children: []Widget{
							TextEdit{
								AssignTo: &logBox,
								ReadOnly: true,
								MinSize:  Size{Height: 170},
								Font:     Font{Family: "Consolas", PointSize: 9},
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

func checkIP(route core.PortRoute) (string, error) {
	host := localConnectHost(route)
	localAddr := net.JoinHostPort(host, strconv.Itoa(route.LocalHTTPPort))
	transport := &http.Transport{}
	switch route.LocalProtocol {
	case core.ProtocolHTTP:
		localProxyURL := "http://" + localAddr
		parsedProxyURL, err := url.Parse(localProxyURL)
		if err != nil {
			return "", err
		}
		transport.Proxy = http.ProxyURL(parsedProxyURL)
	case core.ProtocolSOCKS5:
		dialer, err := proxy.SOCKS5("tcp", localAddr, nil, proxy.Direct)
		if err != nil {
			return "", err
		}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}
	default:
		return "", fmt.Errorf("unsupported local protocol %s", route.LocalProtocol)
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	return fetchPublicIP(client)
}

func checkUpstream(upstream core.UpstreamProxy) (string, error) {
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
			return "", err
		}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}
	default:
		return "", fmt.Errorf("unsupported upstream protocol %s", upstream.Protocol)
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	return fetchPublicIP(client)
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

func fetchPublicIP(client *http.Client) (string, error) {
	checkURLs := []string{
		"http://api.ipify.org?format=json",
		"http://ipinfo.io/ip",
		"http://icanhazip.com",
		"https://api.ipify.org?format=json",
		"https://ipinfo.io/ip",
		"https://icanhazip.com",
	}
	var errs []string
	for _, checkURL := range checkURLs {
		ip, err := fetchPublicIPFrom(client, checkURL)
		if err == nil && ip != "" {
			return ip, nil
		}
		if err != nil {
			errs = append(errs, checkURL+": "+err.Error())
		}
	}
	return "", fmt.Errorf("all IP check endpoints failed: %s", strings.Join(errs, " | "))
}

func fetchPublicIPFrom(client *http.Client, checkURL string) (string, error) {
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
			return "", err
		}
		if resp.StatusCode < 300 || resp.StatusCode >= 400 {
			break
		}
		location := resp.Header.Get("Location")
		_ = resp.Body.Close()
		if location == "" {
			return "", fmt.Errorf("redirect without Location: %s", resp.Status)
		}
		nextURL, err := url.Parse(location)
		if err != nil {
			return "", err
		}
		if !nextURL.IsAbs() {
			baseURL, err := url.Parse(currentURL)
			if err != nil {
				return "", err
			}
			nextURL = baseURL.ResolveReference(nextURL)
		}
		currentURL = nextURL.String()
	}
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("status %s", resp.Status)
	}
	text := strings.TrimSpace(string(body))
	if strings.HasPrefix(text, "{") {
		var payload struct {
			IP string `json:"ip"`
		}
		if err := json.Unmarshal(body, &payload); err == nil && payload.IP != "" {
			return payload.IP, nil
		}
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", fmt.Errorf("empty response")
	}
	if net.ParseIP(fields[0]) == nil {
		return "", fmt.Errorf("unexpected response %q", trimForError(text))
	}
	return fields[0], nil
}

func trimForError(text string) string {
	text = strings.TrimSpace(text)
	if len(text) > 120 {
		return text[:120] + "..."
	}
	return text
}
