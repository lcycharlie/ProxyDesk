//go:build !windows

package main

import "fmt"

func main() {
	fmt.Println("ProxyDesk desktop UI is currently built for Windows. Use GitHub Actions or scripts/build-windows.ps1 to create ProxyDesk.exe.")
}
