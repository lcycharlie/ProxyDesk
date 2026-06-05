//go:build windows

package systemproxy

import (
	"fmt"
	"os/exec"
	"strconv"
)

func EnableHTTPProxy(host string, port int) error {
	server := host + ":" + strconv.Itoa(port)
	cmd := exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyServer", "/t", "REG_SZ", "/d", server, "/f")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("set ProxyServer: %w", err)
	}
	cmd = exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "1", "/f")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("enable ProxyEnable: %w", err)
	}
	return nil
}

func DisableHTTPProxy() error {
	cmd := exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "0", "/f")
	return cmd.Run()
}
