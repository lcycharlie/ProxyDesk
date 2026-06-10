package provider

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"proxydesk/internal/app"
)

func TestFetchAddsCountryAndCityParams(t *testing.T) {
	client := Client{
		Config: app.APIConfig{
			Endpoint:     "http://example.test/gen?proto=http",
			Method:       http.MethodGet,
			CountryParam: "region",
			CityParam:    "st",
		},
	}

	endpoint, err := client.endpoint("US", "California", app.ProtocolSOCKS5)
	if err != nil {
		t.Fatalf("endpoint returned error: %v", err)
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}
	gotRegion := u.Query().Get("region")
	if gotRegion != "US" {
		t.Fatalf("region = %q, want US", gotRegion)
	}
	gotCity := u.Query().Get("st")
	if gotCity != "California" {
		t.Fatalf("st = %q, want California", gotCity)
	}
	gotProto := u.Query().Get("proto")
	if gotProto != "socks5" {
		t.Fatalf("proto = %q, want socks5", gotProto)
	}
}

func TestFetchOmitsEmptyCityParam(t *testing.T) {
	client := Client{
		Config: app.APIConfig{
			Endpoint:     "http://example.test/gen",
			Method:       http.MethodGet,
			CountryParam: "region",
			CityParam:    "st",
		},
	}

	endpoint, err := client.endpoint("US", "", app.ProtocolHTTP)
	if err != nil {
		t.Fatalf("endpoint returned error: %v", err)
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}
	gotCity := u.Query().Get("st")
	if gotCity != "" {
		t.Fatalf("st = %q, want empty", gotCity)
	}
}

func TestFetchParsesProviderResponse(t *testing.T) {
	client := Client{
		Config: app.APIConfig{
			Endpoint: "http://example.test/gen",
			Method:   http.MethodGet,
		},
		HTTP: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader("1.2.3.4:8080:user:pass")),
				Header:     make(http.Header),
			}, nil
		}).client(),
	}

	upstream, err := client.Fetch(context.Background(), "US", "", app.ProtocolHTTP)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if upstream.Address() != "1.2.3.4:8080" {
		t.Fatalf("upstream address = %q, want 1.2.3.4:8080", upstream.Address())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func (f roundTripFunc) client() *http.Client {
	return &http.Client{Transport: f}
}
