package routeproxy

import (
	"context"
	"fmt"

	"proxydesk/internal/app"
	"proxydesk/internal/localproxy"
)

type Server interface {
	Start() error
	Stop(context.Context) error
}

type Snapshot struct {
	Route   app.PortRoute
	Running bool
}

type Manager struct {
	routes   []routeRuntime
	selected int
	onLog    func(format string, args ...any)
}

type routeRuntime struct {
	route  app.PortRoute
	server Server
}

func NewManager(onLog func(format string, args ...any)) *Manager {
	return &Manager{selected: -1, onLog: onLog}
}

func (m *Manager) Len() int {
	return len(m.routes)
}

func (m *Manager) Selected() int {
	return m.selected
}

func (m *Manager) SetSelected(index int) {
	if index < 0 || index >= len(m.routes) {
		m.selected = -1
		return
	}
	m.selected = index
}

func (m *Manager) Add(route app.PortRoute) int {
	m.routes = append(m.routes, routeRuntime{route: route})
	m.selected = len(m.routes) - 1
	return m.selected
}

func (m *Manager) Route(index int) (app.PortRoute, bool, bool) {
	if index < 0 || index >= len(m.routes) {
		return app.PortRoute{}, false, false
	}
	rt := m.routes[index]
	return rt.route, rt.server != nil, true
}

func (m *Manager) SetRoute(index int, route app.PortRoute) bool {
	if index < 0 || index >= len(m.routes) {
		return false
	}
	m.routes[index].route = route
	return true
}

func (m *Manager) IsRunning(index int) bool {
	if index < 0 || index >= len(m.routes) {
		return false
	}
	return m.routes[index].server != nil
}

func (m *Manager) Snapshots() []Snapshot {
	snapshots := make([]Snapshot, len(m.routes))
	for i, rt := range m.routes {
		snapshots[i] = Snapshot{Route: rt.route, Running: rt.server != nil}
	}
	return snapshots
}

func (m *Manager) UsedPorts(keepPort int) map[int]bool {
	used := map[int]bool{}
	for _, rt := range m.routes {
		if rt.route.LocalHTTPPort != keepPort {
			used[rt.route.LocalHTTPPort] = true
		}
	}
	return used
}

func (m *Manager) Start(index int) (app.PortRoute, error) {
	if index < 0 || index >= len(m.routes) {
		return app.PortRoute{}, fmt.Errorf("route index out of range")
	}
	if m.routes[index].server != nil {
		_ = m.routes[index].server.Stop(context.Background())
		m.routes[index].server = nil
	}
	route := m.routes[index].route
	server, err := m.newServer(route)
	if err != nil {
		return app.PortRoute{}, err
	}
	if err := server.Start(); err != nil {
		return app.PortRoute{}, err
	}
	m.routes[index].server = server
	m.selected = index
	return route, nil
}

func (m *Manager) Stop(index int, ctx context.Context) (app.PortRoute, error) {
	if index < 0 || index >= len(m.routes) {
		return app.PortRoute{}, fmt.Errorf("route index out of range")
	}
	route := m.routes[index].route
	if m.routes[index].server == nil {
		return route, nil
	}
	if err := m.routes[index].server.Stop(ctx); err != nil {
		return route, err
	}
	m.routes[index].server = nil
	return route, nil
}

func (m *Manager) Delete(index int, ctx context.Context) (app.PortRoute, bool) {
	if index < 0 || index >= len(m.routes) {
		return app.PortRoute{}, false
	}
	route := m.routes[index].route
	if m.routes[index].server != nil {
		_ = m.routes[index].server.Stop(ctx)
	}
	m.routes = append(m.routes[:index], m.routes[index+1:]...)
	if index >= len(m.routes) {
		index = len(m.routes) - 1
	}
	m.selected = index
	return route, true
}

func (m *Manager) StopAll(ctx context.Context) {
	for i := range m.routes {
		if m.routes[i].server != nil {
			_ = m.routes[i].server.Stop(ctx)
			m.routes[i].server = nil
		}
	}
}

func (m *Manager) newServer(route app.PortRoute) (Server, error) {
	switch route.LocalProtocol {
	case app.ProtocolHTTP:
		httpServer := localproxy.NewHTTPServer(route)
		httpServer.OnLog = m.onLog
		return httpServer, nil
	case app.ProtocolSOCKS5:
		socksServer := localproxy.NewSOCKS5Server(route)
		socksServer.OnLog = m.onLog
		return socksServer, nil
	default:
		return nil, fmt.Errorf("不支持的本地协议：%s", route.LocalProtocol)
	}
}
