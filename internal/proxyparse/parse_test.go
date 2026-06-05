package proxyparse

import (
	"testing"

	"proxydesk/internal/app"
)

func TestParseLineFormats(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		host     string
		port     string
		username string
		password string
	}{
		{
			name:     "colon auth",
			line:     "107.150.104.202:2672:77ad76a2c1f1:dvy9mmpdknfcxxwcitwu",
			host:     "107.150.104.202",
			port:     "2672",
			username: "77ad76a2c1f1",
			password: "dvy9mmpdknfcxxwcitwu",
		},
		{
			name:     "scheme colon auth",
			line:     "http://107.150.104.202:2672:77ad76a2c1f1:dvy9mmpdknfcxxwcitwu",
			host:     "107.150.104.202",
			port:     "2672",
			username: "77ad76a2c1f1",
			password: "dvy9mmpdknfcxxwcitwu",
		},
		{
			name:     "userinfo auth",
			line:     "http://77ad76a2c1f1:dvy9mmpdknfcxxwcitwu@107.150.104.202:2672",
			host:     "107.150.104.202",
			port:     "2672",
			username: "77ad76a2c1f1",
			password: "dvy9mmpdknfcxxwcitwu",
		},
		{
			name: "host port",
			line: "107.150.104.202:2672",
			host: "107.150.104.202",
			port: "2672",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLine(tt.line, app.ProtocolHTTP)
			if err != nil {
				t.Fatalf("ParseLine() error = %v", err)
			}
			if got.Host != tt.host || got.Port != tt.port || got.Username != tt.username || got.Password != tt.password {
				t.Fatalf("ParseLine() = %#v", got)
			}
		})
	}
}
