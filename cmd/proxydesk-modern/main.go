package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	core "proxydesk/internal/app"
	"proxydesk/internal/catalog"
	"proxydesk/internal/modernui"
	"proxydesk/internal/routeproxy"
	"proxydesk/internal/routing"
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
		PortRange:        defaultPortRange(),
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
	input := uistate.PortRangeText{
		Start: fmt.Sprintf("%d", routing.DefaultPortStart),
		End:   fmt.Sprintf("%d", routing.DefaultPortEnd),
	}
	return uistate.AvailablePortOptions(input, a.routes.UsedPorts(0))
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

func defaultPortRange() routing.PortRange {
	return routing.PortRange{Start: routing.DefaultPortStart, End: routing.DefaultPortEnd}
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
	resp, err := client.Get(checkURL)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
