package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

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
	a := app.NewWithID("com.proxydesk.desktop")
	w := a.NewWindow("ProxyDesk")
	w.Resize(fyne.NewSize(980, 660))

	state := &runtimeState{}

	countries := []string{
		"US - United States",
		"JP - Japan",
		"GB - United Kingdom",
		"DE - Germany",
		"SG - Singapore",
		"BR - Brazil",
		"IN - India",
	}

	status := widget.NewLabel("未启动")
	status.TextStyle.Bold = true
	exitIP := widget.NewLabel("-")
	upstreamView := widget.NewLabel("-")
	localView := widget.NewLabel("127.0.0.1:7890")
	errorView := widget.NewLabel("-")
	errorView.Wrapping = fyne.TextWrapWord

	countrySelect := widget.NewSelect(countries, nil)
	countrySelect.SetSelected(countries[0])

	protocolSelect := widget.NewSelect([]string{string(core.ProtocolHTTP), string(core.ProtocolSOCKS5)}, nil)
	protocolSelect.SetSelected(string(core.ProtocolHTTP))

	portEntry := widget.NewEntry()
	portEntry.SetText("7890")
	portEntry.Validator = func(value string) error {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("端口需要在 1-65535 之间")
		}
		return nil
	}

	upstreamEntry := widget.NewMultiLineEntry()
	upstreamEntry.SetPlaceHolder("粘贴上游代理，格式 host:port:user:pass\n例如 global.rpip.lokiproxy.com:35001:USER096836-session-5MHDsJKATDS:48a951")
	upstreamEntry.SetMinRowsVisible(5)

	apiEndpoint := widget.NewEntry()
	apiEndpoint.SetPlaceHolder("API 地址，例如 https://example.com/getProxy")
	apiCountryParam := widget.NewEntry()
	apiCountryParam.SetPlaceHolder("国家参数名，例如 country")
	apiJSONKey := widget.NewEntry()
	apiJSONKey.SetPlaceHolder("JSON 字段名，纯文本返回可留空")

	logBox := widget.NewMultiLineEntry()
	logBox.SetMinRowsVisible(9)
	logBox.Disable()

	appendLog := func(format string, args ...any) {
		line := time.Now().Format("15:04:05") + "  " + fmt.Sprintf(format, args...)
		logBox.SetText(strings.TrimSpace(logBox.Text + "\n" + line))
	}

	buildRoute := func() (core.PortRoute, error) {
		if err := portEntry.Validate(); err != nil {
			return core.PortRoute{}, err
		}
		port, _ := strconv.Atoi(portEntry.Text)
		protocol := core.Protocol(protocolSelect.Selected)
		if protocol == core.ProtocolSOCKS5 {
			return core.PortRoute{}, fmt.Errorf("第一版界面已预留 SOCKS5，本地 SOCKS5 转发将在下一步接入；当前请先使用 HTTP")
		}

		line := strings.TrimSpace(upstreamEntry.Text)
		if strings.Contains(line, "\n") {
			line = strings.TrimSpace(strings.Split(line, "\n")[0])
		}
		upstream, err := proxyparse.ParseLine(line, protocol)
		if err != nil {
			return core.PortRoute{}, err
		}

		countryCode, countryName := splitCountry(countrySelect.Selected)
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

	startButton := widget.NewButton("启动转发", func() {
		route, err := buildRoute()
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		if state.server != nil {
			_ = state.server.Stop(context.Background())
		}
		server := localproxy.NewHTTPServer(route)
		if err := server.Start(); err != nil {
			errorView.SetText(err.Error())
			dialog.ShowError(err, w)
			return
		}
		state.server = server
		state.route = route
		status.SetText("运行中")
		localView.SetText("127.0.0.1:" + strconv.Itoa(route.LocalHTTPPort))
		upstreamView.SetText(proxyparse.Format(route.Upstream))
		errorView.SetText("-")
		appendLog("已启动 %s -> %s", localView.Text, route.Upstream.Address())
	})
	startButton.Importance = widget.HighImportance

	stopButton := widget.NewButton("停止", func() {
		if state.server == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := state.server.Stop(ctx); err != nil {
			dialog.ShowError(err, w)
			return
		}
		state.server = nil
		status.SetText("已停止")
		appendLog("已停止本地转发")
	})

	testButton := widget.NewButton("测试出口", func() {
		if state.server == nil {
			dialog.ShowInformation("提示", "请先启动本地转发", w)
			return
		}
		ip, err := checkIP(state.route.LocalHTTPPort)
		if err != nil {
			errorView.SetText(err.Error())
			appendLog("出口检测失败：%v", err)
			return
		}
		exitIP.SetText(ip)
		errorView.SetText("-")
		appendLog("出口检测成功：%s", ip)
	})

	enableSysProxy := widget.NewButton("开启系统代理", func() {
		port, err := strconv.Atoi(portEntry.Text)
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		if err := systemproxy.EnableHTTPProxy("127.0.0.1", port); err != nil {
			dialog.ShowError(err, w)
			return
		}
		appendLog("已开启 Windows 系统代理：127.0.0.1:%d", port)
	})

	disableSysProxy := widget.NewButton("关闭系统代理", func() {
		if err := systemproxy.DisableHTTPProxy(); err != nil {
			dialog.ShowError(err, w)
			return
		}
		appendLog("已关闭 Windows 系统代理")
	})

	fetchAPIButton := widget.NewButton("按国家获取 IP", func() {
		countryCode, _ := splitCountry(countrySelect.Selected)
		client := provider.Client{
			Config: core.APIConfig{
				Endpoint:        strings.TrimSpace(apiEndpoint.Text),
				Method:          http.MethodGet,
				CountryParam:    strings.TrimSpace(apiCountryParam.Text),
				ResponseJSONKey: strings.TrimSpace(apiJSONKey.Text),
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		upstream, err := client.Fetch(ctx, countryCode, core.Protocol(protocolSelect.Selected))
		if err != nil {
			errorView.SetText(err.Error())
			appendLog("API 获取失败：%v", err)
			dialog.ShowError(err, w)
			return
		}
		upstreamEntry.SetText(proxyparse.Format(upstream))
		errorView.SetText("-")
		appendLog("API 获取成功：%s %s", countryCode, upstream.Address())
	})

	statusGrid := container.NewGridWithColumns(2,
		widget.NewLabel("状态"), status,
		widget.NewLabel("出口 IP"), exitIP,
		widget.NewLabel("本地端口"), localView,
		widget.NewLabel("上游代理"), upstreamView,
		widget.NewLabel("错误"), errorView,
	)

	routeForm := widget.NewForm(
		widget.NewFormItem("国家", countrySelect),
		widget.NewFormItem("协议", protocolSelect),
		widget.NewFormItem("本地端口", portEntry),
		widget.NewFormItem("上游代理", upstreamEntry),
	)

	apiForm := widget.NewForm(
		widget.NewFormItem("API 地址", apiEndpoint),
		widget.NewFormItem("国家参数", apiCountryParam),
		widget.NewFormItem("JSON 字段", apiJSONKey),
	)

	left := container.NewBorder(nil, container.NewHBox(startButton, stopButton, testButton), nil, nil, routeForm)
	right := container.NewVBox(
		widget.NewCard("当前连接", "", statusGrid),
		widget.NewCard("供应商 API", "", container.NewBorder(nil, fetchAPIButton, nil, nil, apiForm)),
		widget.NewCard("系统代理", "", container.NewHBox(enableSysProxy, disableSysProxy)),
	)

	content := container.NewBorder(
		nil,
		widget.NewCard("日志", "", logBox),
		nil,
		nil,
		container.NewHSplit(left, right),
	)
	w.SetContent(content)
	w.SetCloseIntercept(func() {
		if state.server != nil {
			_ = state.server.Stop(context.Background())
		}
		w.Close()
	})
	w.ShowAndRun()
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
