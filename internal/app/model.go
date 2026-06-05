package app

import "time"

type Protocol string

const (
	ProtocolHTTP   Protocol = "HTTP"
	ProtocolSOCKS5 Protocol = "SOCKS5"
)

type UpstreamProxy struct {
	Host     string   `json:"host"`
	Port     string   `json:"port"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	Protocol Protocol `json:"protocol"`
}

func (p UpstreamProxy) Address() string {
	return p.Host + ":" + p.Port
}

type PortRoute struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	CountryCode    string        `json:"country_code"`
	CountryName    string        `json:"country_name"`
	LocalHost      string        `json:"local_host"`
	LocalHTTPPort  int           `json:"local_http_port"`
	LocalSocksPort int           `json:"local_socks_port"`
	LocalProtocol  Protocol      `json:"local_protocol"`
	Protocol       Protocol      `json:"protocol"`
	Upstream       UpstreamProxy `json:"upstream"`
	Enabled        bool          `json:"enabled"`
	LastExitIP     string        `json:"last_exit_ip"`
	LastLatencyMS  int64         `json:"last_latency_ms"`
	LastError      string        `json:"last_error"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type APIConfig struct {
	Endpoint        string            `json:"endpoint"`
	Method          string            `json:"method"`
	Headers         map[string]string `json:"headers"`
	CountryParam    string            `json:"country_param"`
	ResponseJSONKey string            `json:"response_json_key"`
}
