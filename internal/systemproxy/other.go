//go:build !windows

package systemproxy

import "fmt"

func EnableHTTPProxy(host string, port int) error {
	return fmt.Errorf("system proxy control is only implemented on Windows")
}

func EnableProxy(host string, port int, protocol string) error {
	return fmt.Errorf("system proxy control is only implemented on Windows")
}

func DisableHTTPProxy() error {
	return fmt.Errorf("system proxy control is only implemented on Windows")
}
