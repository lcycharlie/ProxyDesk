package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"proxydesk/internal/app"
	"proxydesk/internal/proxyparse"
)

type Client struct {
	Config app.APIConfig
	HTTP   *http.Client
}

func (c Client) Fetch(ctx context.Context, countryCode string, protocol app.Protocol) (app.UpstreamProxy, error) {
	if c.Config.Endpoint == "" {
		return app.UpstreamProxy{}, fmt.Errorf("API endpoint is empty")
	}

	endpoint := c.Config.Endpoint
	if c.Config.CountryParam != "" || strings.Contains(endpoint, "proto=") {
		u, err := url.Parse(endpoint)
		if err != nil {
			return app.UpstreamProxy{}, err
		}
		q := u.Query()
		if c.Config.CountryParam != "" {
			q.Set(c.Config.CountryParam, countryCode)
		}
		if _, ok := q["proto"]; ok {
			q.Set("proto", strings.ToLower(string(protocol)))
		}
		u.RawQuery = q.Encode()
		endpoint = u.String()
	}

	method := c.Config.Method
	if method == "" {
		method = http.MethodGet
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return app.UpstreamProxy{}, err
	}
	for k, v := range c.Config.Headers {
		req.Header.Set(k, v)
	}

	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return app.UpstreamProxy{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return app.UpstreamProxy{}, fmt.Errorf("API status: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return app.UpstreamProxy{}, err
	}

	text := strings.TrimSpace(string(body))
	if c.Config.ResponseJSONKey != "" {
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			return app.UpstreamProxy{}, err
		}
		value, ok := payload[c.Config.ResponseJSONKey]
		if !ok {
			return app.UpstreamProxy{}, fmt.Errorf("JSON key %q not found", c.Config.ResponseJSONKey)
		}
		text = strings.TrimSpace(fmt.Sprint(value))
	}

	text = strings.NewReplacer(`\r\n`, "\n", `\n`, "\n", "\r\n", "\n", "\r", "\n").Replace(text)
	firstLine := ""
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) != "" {
			firstLine = strings.TrimSpace(line)
			break
		}
	}
	return proxyparse.ParseLine(firstLine, protocol)
}
