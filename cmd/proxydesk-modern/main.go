package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"golang.org/x/net/proxy"

	core "proxydesk/internal/app"
	"proxydesk/internal/catalog"
	"proxydesk/internal/modernui"
	"proxydesk/internal/provider"
	"proxydesk/internal/routeproxy"
	"proxydesk/internal/routing"
	"proxydesk/internal/systemproxy"
	"proxydesk/internal/uistate"
)

type App struct {
	ctx     context.Context
	localIP string
	routes  *routeproxy.Manager
	logs    []string
}

func NewApp() *App {
	app := &App{
		localIP: detectLANIP(),
	}
	app.routes = routeproxy.NewManager(app.appendLog)
	return app
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) AppName() string {
	return "ProxyDesk"
}

type InitialState struct {
	AppName           string     `json:"appName"`
	LocalIP           string     `json:"localIP"`
	EnvironmentExit   string     `json:"environmentExit"`
	PortStart         string     `json:"portStart"`
	PortEnd           string     `json:"portEnd"`
	PortOptions       []string   `json:"portOptions"`
	Countries         []string   `json:"countries"`
	Cities            []string   `json:"cities"`
	LocalProtocols    []string   `json:"localProtocols"`
	UpstreamProtocols []string   `json:"upstreamProtocols"`
	Routes            []RouteRow `json:"routes"`
}

type ManualRouteRequest struct {
	LocalProtocol    string `json:"localProtocol"`
	UpstreamProtocol string `json:"upstreamProtocol"`
	LocalPort        string `json:"localPort"`
	ProxyLine        string `json:"proxyLine"`
	PortStart        string `json:"portStart"`
	PortEnd          string `json:"portEnd"`
}

type ProviderAPIRequest struct {
	CountryLabel     string `json:"countryLabel"`
	City             string `json:"city"`
	LocalProtocol    string `json:"localProtocol"`
	UpstreamProtocol string `json:"upstreamProtocol"`
	LocalPort        string `json:"localPort"`
	Endpoint         string `json:"endpoint"`
	CountryParam     string `json:"countryParam"`
	CityParam        string `json:"cityParam"`
	JSONKey          string `json:"jsonKey"`
	PortStart        string `json:"portStart"`
	PortEnd          string `json:"portEnd"`
}

type RouteRow struct {
	Index            int    `json:"index"`
	Status           string `json:"status"`
	Running          bool   `json:"running"`
	LocalAddress     string `json:"localAddress"`
	LocalProtocol    string `json:"localProtocol"`
	UpstreamProtocol string `json:"upstreamProtocol"`
	UpstreamAddress  string `json:"upstreamAddress"`
	ExitDisplay      string `json:"exitDisplay"`
}

func (a *App) GetInitialState() InitialState {
	portInput := uistate.PortRangeText{
		Start: fmt.Sprintf("%d", routing.DefaultPortStart),
		End:   fmt.Sprintf("%d", routing.DefaultPortEnd),
	}
	return InitialState{
		AppName:           "ProxyDesk",
		LocalIP:           a.localIP,
		EnvironmentExit:   "点击刷新检测",
		PortStart:         portInput.Start,
		PortEnd:           portInput.End,
		PortOptions:       uistate.AvailablePortOptions(portInput, nil),
		Countries:         catalog.Countries(),
		Cities:            catalog.CityOptions("US"),
		LocalProtocols:    []string{"HTTP/HTTPS", "SOCKS5"},
		UpstreamProtocols: []string{"HTTP", "SOCKS5"},
		Routes:            a.GetRoutes(),
	}
}

func (a *App) CitiesForCountry(countryLabel string) []string {
	code, _ := catalog.SplitCountry(countryLabel)
	return catalog.CityOptions(code)
}

func (a *App) FilterCountries(query string) []string {
	return catalog.FilterCountries(catalog.Countries(), query)
}

func (a *App) RefreshEnvironmentExit() string {
	client := &http.Client{Timeout: 12 * time.Second}
	info, err := fetchPublicIPInfo(client)
	if err != nil {
		return "检测失败"
	}
	return environmentCountryDisplay(info)
}

func (a *App) AddManualRoute(input ManualRouteRequest) ([]RouteRow, error) {
	route, err := routing.BuildManualRoute(routing.ManualRouteInput{
		ListenHost:       a.localIP,
		PortText:         input.LocalPort,
		PortRange:        portRangeFromText(input.PortStart, input.PortEnd),
		UsedPorts:        a.routes.UsedPorts(0),
		LocalProtocol:    parseLocalProtocol(input.LocalProtocol),
		UpstreamProtocol: parseUpstreamProtocol(input.UpstreamProtocol),
		ProxyLine:        input.ProxyLine,
		Now:              time.Now(),
	})
	if err != nil {
		return nil, err
	}
	a.routes.Add(route)
	a.appendLog("已新增转发配置：%s:%d -> %s", route.LocalHost, route.LocalHTTPPort, route.Upstream.Address())
	return a.GetRoutes(), nil
}

func (a *App) StartRoute(index int) ([]RouteRow, error) {
	route, err := a.routes.Start(index)
	if err != nil {
		a.appendLog("启动失败：%v", err)
		return nil, err
	}
	a.appendLog("已启动本地 %s 代理 %s:%d -> %s 上游 %s", route.LocalProtocol, route.LocalHost, route.LocalHTTPPort, route.Upstream.Protocol, route.Upstream.Address())
	return a.GetRoutes(), nil
}

func (a *App) StopRoute(index int) ([]RouteRow, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	route, err := a.routes.Stop(index, ctx)
	if err != nil {
		a.appendLog("停止失败：%v", err)
		return nil, err
	}
	a.appendLog("已停止本地转发：%s:%d", route.LocalHost, route.LocalHTTPPort)
	return a.GetRoutes(), nil
}

func (a *App) DeleteRoute(index int) ([]RouteRow, error) {
	route, ok := a.routes.Delete(index, context.Background())
	if !ok {
		return nil, fmt.Errorf("route index out of range")
	}
	a.appendLog("已删除转发配置：%s:%d", route.LocalHost, route.LocalHTTPPort)
	return a.GetRoutes(), nil
}

func (a *App) StopAllRoutes() []RouteRow {
	a.routes.StopAll(context.Background())
	a.appendLog("已停止全部转发")
	return a.GetRoutes()
}

func (a *App) TestRouteExit(index int) ([]RouteRow, error) {
	route, running, ok := a.routes.Route(index)
	if !ok {
		return nil, fmt.Errorf("请先选择一条转发配置")
	}
	if !running {
		return nil, fmt.Errorf("请先启动本地转发")
	}
	info, err := checkIP(route)
	if err != nil {
		a.appendLog("选中转发出口检测失败：%v", err)
		return nil, err
	}
	route.LastExitIP = info.IP
	route.LastExitCountry = info.Country
	route.LastExitRegion = info.Region
	route.LastExitCity = info.City
	a.routes.SetRoute(index, route)
	a.appendLog("选中转发出口检测成功：%s", publicIPDisplay(info))
	return a.GetRoutes(), nil
}

func (a *App) FetchProviderIP(input ProviderAPIRequest) ([]RouteRow, error) {
	countryCode, _ := catalog.SplitCountry(input.CountryLabel)
	city := strings.TrimSpace(input.City)
	if city == catalog.CityAllOption {
		city = ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	client := provider.Client{
		Config: core.APIConfig{
			Endpoint:        strings.TrimSpace(input.Endpoint),
			Method:          http.MethodGet,
			CountryParam:    strings.TrimSpace(input.CountryParam),
			CityParam:       strings.TrimSpace(input.CityParam),
			ResponseJSONKey: strings.TrimSpace(input.JSONKey),
		},
	}
	upstream, err := client.Fetch(ctx, countryCode, city, parseUpstreamProtocol(input.UpstreamProtocol))
	if err != nil {
		a.appendLog("API 获取失败：%v", err)
		return nil, err
	}
	route, err := routing.BuildRouteFromUpstream(routing.UpstreamRouteInput{
		ListenHost:    a.localIP,
		PortText:      input.LocalPort,
		PortRange:     portRangeFromText(input.PortStart, input.PortEnd),
		UsedPorts:     a.routes.UsedPorts(0),
		LocalProtocol: parseLocalProtocol(input.LocalProtocol),
		Upstream:      upstream,
		Now:           time.Now(),
	})
	if err != nil {
		a.appendLog("API 新增转发失败：%v", err)
		return nil, err
	}
	a.routes.Add(route)
	location := countryCode
	if city != "" {
		location += " " + city
	}
	a.appendLog("API 获取成功：%s %s，已加入转发列表端口 %d", location, upstream.Address(), route.LocalHTTPPort)
	return a.GetRoutes(), nil
}

func (a *App) EnableSystemProxy(index int) error {
	route, _, ok := a.routes.Route(index)
	if !ok {
		return fmt.Errorf("请先在转发列表中选择一条配置")
	}
	host := localConnectHost(route)
	if err := systemproxy.EnableProxy(host, route.LocalHTTPPort, string(route.LocalProtocol)); err != nil {
		a.appendLog("系统代理开启失败：%v", err)
		return err
	}
	a.appendLog("已开启 Windows %s 系统代理：%s:%d", route.LocalProtocol, host, route.LocalHTTPPort)
	return nil
}

func (a *App) DisableSystemProxy() error {
	if err := systemproxy.DisableHTTPProxy(); err != nil {
		a.appendLog("系统代理关闭失败：%v", err)
		return err
	}
	a.appendLog("已关闭 Windows 系统代理")
	return nil
}

func (a *App) GetRoutes() []RouteRow {
	snapshots := a.routes.Snapshots()
	rows := make([]RouteRow, 0, len(snapshots))
	for index, snapshot := range snapshots {
		route := snapshot.Route
		rows = append(rows, RouteRow{
			Index:            index,
			Status:           statusText(snapshot.Running),
			Running:          snapshot.Running,
			LocalAddress:     net.JoinHostPort(route.LocalHost, fmt.Sprintf("%d", route.LocalHTTPPort)),
			LocalProtocol:    uistate.LocalProtocolDisplay(route.LocalProtocol),
			UpstreamProtocol: uistate.UpstreamProtocolDisplay(route.Protocol),
			UpstreamAddress:  route.Upstream.Address(),
			ExitDisplay:      uistate.ExitDisplay(route),
		})
	}
	return rows
}

func (a *App) GetPortOptions() []string {
	return a.GetPortOptionsForRange(fmt.Sprintf("%d", routing.DefaultPortStart), fmt.Sprintf("%d", routing.DefaultPortEnd))
}

func (a *App) GetPortOptionsForRange(start string, end string) []string {
	input := uistate.PortRangeText{
		Start: start,
		End:   end,
	}
	return uistate.AvailablePortOptions(input, a.routes.UsedPorts(0))
}

func (a *App) GetLogs() string {
	return strings.Join(a.logs, "\n")
}

func (a *App) ClearLogs() string {
	a.logs = nil
	return ""
}

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "ProxyDesk",
		Width:  1180,
		Height: 760,
		AssetServer: &assetserver.Options{
			Assets: modernui.FS(),
		},
		BackgroundColour: &options.RGBA{R: 244, G: 248, B: 247, A: 255},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		println("ProxyDesk modern UI startup failed:", err.Error())
	}
}

func (a *App) appendLog(format string, args ...any) {
	a.logs = append(a.logs, time.Now().Format("15:04:05")+"  "+fmt.Sprintf(format, args...))
}

func portRangeFromText(start string, end string) routing.PortRange {
	portRange := uistate.PortRangeFromText(uistate.PortRangeText{Start: start, End: end})
	return routing.PortRange{Start: portRange.Start, End: portRange.End}
}

func parseLocalProtocol(value string) core.Protocol {
	if strings.EqualFold(strings.TrimSpace(value), string(core.ProtocolSOCKS5)) {
		return core.ProtocolSOCKS5
	}
	return core.ProtocolHTTP
}

func parseUpstreamProtocol(value string) core.Protocol {
	if strings.EqualFold(strings.TrimSpace(value), string(core.ProtocolSOCKS5)) {
		return core.ProtocolSOCKS5
	}
	return core.ProtocolHTTP
}

func statusText(running bool) string {
	if running {
		return "运行中"
	}
	return "未启动"
}

func localConnectHost(route core.PortRoute) string {
	switch route.LocalHost {
	case "", "0.0.0.0":
		return "127.0.0.1"
	default:
		return route.LocalHost
	}
}

type publicIPInfo struct {
	IP          string
	Country     string
	CountryCode string
	Region      string
	City        string
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

func checkIP(route core.PortRoute) (publicIPInfo, error) {
	host := localConnectHost(route)
	localAddr := net.JoinHostPort(host, fmt.Sprintf("%d", route.LocalHTTPPort))
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
	if resp == nil {
		return publicIPInfo{}, fmt.Errorf("empty response")
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
				return info, nil
			}
		}
	}
	fields := strings.Fields(text)
	if len(fields) == 0 || net.ParseIP(fields[0]) == nil {
		return publicIPInfo{}, fmt.Errorf("unexpected response")
	}
	return publicIPInfo{IP: fields[0]}, nil
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

func publicIPDisplay(info publicIPInfo) string {
	parts := []string{}
	if info.IP != "" {
		parts = append(parts, info.IP)
	}
	for _, part := range []string{info.Country, info.Region, info.City} {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
