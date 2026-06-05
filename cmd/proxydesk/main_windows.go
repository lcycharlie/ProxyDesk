//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	core "proxydesk/internal/app"
	"proxydesk/internal/localproxy"
	"proxydesk/internal/provider"
	"proxydesk/internal/proxyparse"
	"proxydesk/internal/systemproxy"
)

type runtimeState struct {
	server *localproxy.HTTPServer
	route  core.PortRoute
}

func main() {
	state := &runtimeState{}
	countries := []string{"US - United States", "JP - Japan", "GB - United Kingdom", "DE - Germany", "SG - Singapore", "BR - Brazil", "IN - India"}

	var mw *walk.MainWindow
	var countryCB, protocolCB *walk.ComboBox
	var portEdit, apiEndpoint, apiCountryParam, apiJSONKey *walk.LineEdit
	var upstreamEdit, logBox *walk.TextEdit
	var statusLabel, exitIPLabel, upstreamLabel, localLabel, errorLabel *walk.Label

	appendLog := func(format string, args ...any) {
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

	selectedCountry := func() string {
		idx := countryCB.CurrentIndex()
		if idx < 0 || idx >= len(countries) {
			return countries[0]
		}
		return countries[idx]
	}

	selectedProtocol := func() core.Protocol {
		if protocolCB.CurrentIndex() == 1 {
			return core.ProtocolSOCKS5
		}
		return core.ProtocolHTTP
	}

	buildRoute := func() (core.PortRoute, error) {
		port, err := strconv.Atoi(strings.TrimSpace(portEdit.Text()))
		if err != nil || port < 1 || port > 65535 {
			return core.PortRoute{}, fmt.Errorf("端口需要在 1-65535 之间")
		}
		protocol := selectedProtocol()
		if protocol == core.ProtocolSOCKS5 {
			return core.PortRoute{}, fmt.Errorf("当前版本先支持本地 HTTP 转发，SOCKS5 本地监听下一版接入")
		}

		line := strings.TrimSpace(upstreamEdit.Text())
		if strings.Contains(line, "\n") {
			line = strings.TrimSpace(strings.Split(line, "\n")[0])
		}
		upstream, err := proxyparse.ParseLine(line, protocol)
		if err != nil {
			return core.PortRoute{}, err
		}

		countryCode, countryName := splitCountry(selectedCountry())
		return core.PortRoute{
			ID:            countryCode + "-" + strconv.Itoa(port),
			Name:          countryName + " " + strconv.Itoa(port),
			CountryCode:   countryCode,
			CountryName:   countryName,
			LocalHTTPPort: port,
			Protocol:      protocol,
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
		server := localproxy.NewHTTPServer(route)
		if err := server.Start(); err != nil {
			_ = errorLabel.SetText(err.Error())
			walk.MsgBox(mw, "启动失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		state.server = server
		state.route = route
		_ = statusLabel.SetText("运行中")
		statusLabel.SetTextColor(walk.RGB(22, 120, 75))
		_ = localLabel.SetText("127.0.0.1:" + strconv.Itoa(route.LocalHTTPPort))
		_ = upstreamLabel.SetText(proxyparse.Format(route.Upstream))
		_ = errorLabel.SetText("-")
		appendLog("已启动 %s -> %s", localLabel.Text(), route.Upstream.Address())
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
		ip, err := checkIP(state.route.LocalHTTPPort)
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
		upstream, err := client.Fetch(ctx, countryCode, selectedProtocol())
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
		port, err := strconv.Atoi(strings.TrimSpace(portEdit.Text()))
		if err != nil {
			walk.MsgBox(mw, "端口错误", err.Error(), walk.MsgBoxIconError)
			return
		}
		if err := systemproxy.EnableHTTPProxy("127.0.0.1", port); err != nil {
			walk.MsgBox(mw, "系统代理失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		appendLog("已开启 Windows 系统代理：127.0.0.1:%d", port)
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
										Text:      "127.0.0.1:7890",
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
							Label{Text: "协议"},
							Label{Text: "HTTP", TextColor: walk.RGB(30, 64, 175)},
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
									ComboBox{AssignTo: &protocolCB, Model: []string{"HTTP", "SOCKS5"}, CurrentIndex: 0, MinSize: Size{Height: 26}},
									Label{Text: "本地端口", TextColor: walk.RGB(71, 85, 105)},
									LineEdit{AssignTo: &portEdit, Text: "7890", MinSize: Size{Height: 26}},
								},
							},
							Label{Text: "上游代理", TextColor: walk.RGB(71, 85, 105)},
							TextEdit{AssignTo: &upstreamEdit, MinSize: Size{Width: 460, Height: 120}},
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

func checkIP(localPort int) (string, error) {
	localProxyURL := "http://127.0.0.1:" + strconv.Itoa(localPort)
	parsedProxyURL, err := url.Parse(localProxyURL)
	if err != nil {
		return "", err
	}

	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(parsedProxyURL)},
		Timeout:   30 * time.Second,
	}
	resp, err := client.Get("https://ipinfo.io/json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ipinfo status: %s", resp.Status)
	}
	var payload struct {
		IP      string `json:"ip"`
		Country string `json:"country"`
		City    string `json:"city"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return strings.TrimSpace(string(body)), nil
	}
	return strings.TrimSpace(payload.IP + " " + payload.Country + " " + payload.City), nil
}

func checkUpstream(upstream core.UpstreamProxy) (string, error) {
	upstreamURL := &url.URL{Scheme: "http", Host: upstream.Address()}
	if upstream.Username != "" || upstream.Password != "" {
		upstreamURL.User = url.UserPassword(upstream.Username, upstream.Password)
	}

	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(upstreamURL)},
		Timeout:   30 * time.Second,
	}
	resp, err := client.Get("https://ipinfo.io/json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ipinfo status: %s", resp.Status)
	}
	var payload struct {
		IP      string `json:"ip"`
		Country string `json:"country"`
		City    string `json:"city"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return strings.TrimSpace(string(body)), nil
	}
	return strings.TrimSpace(payload.IP + " " + payload.Country + " " + payload.City), nil
}
