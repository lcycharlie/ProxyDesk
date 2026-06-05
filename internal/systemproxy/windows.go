//go:build windows

package systemproxy

import (
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
)

func EnableHTTPProxy(host string, port int) error {
	return EnableProxy(host, port, "HTTP")
}

func EnableProxy(host string, port int, protocol string) error {
	addr := host + ":" + strconv.Itoa(port)
	server := "http=" + addr + ";https=" + addr
	if protocol == "SOCKS5" {
		server = "socks=" + addr
	}
	cmd := exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyServer", "/t", "REG_SZ", "/d", server, "/f")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("set ProxyServer: %w", err)
	}
	cmd = exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyOverride", "/t", "REG_SZ", "/d", "localhost;127.0.0.1;<local>", "/f")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("set ProxyOverride: %w", err)
	}
	cmd = exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "1", "/f")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("enable ProxyEnable: %w", err)
	}
	return refresh()
}

func DisableHTTPProxy() error {
	cmd := exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "0", "/f")
	if err := cmd.Run(); err != nil {
		return err
	}
	return refresh()
}

func refresh() error {
	wininet := syscall.NewLazyDLL("wininet.dll")
	internetSetOption := wininet.NewProc("InternetSetOptionW")
	const (
		internetOptionSettingsChanged = 39
		internetOptionRefresh         = 37
	)
	if r, _, err := internetSetOption.Call(0, internetOptionSettingsChanged, 0, 0); r == 0 {
		return fmt.Errorf("notify proxy settings changed: %w", err)
	}
	if r, _, err := internetSetOption.Call(0, internetOptionRefresh, 0, 0); r == 0 {
		return fmt.Errorf("refresh proxy settings: %w", err)
	}
	return nil
}
