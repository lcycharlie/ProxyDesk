//go:build windows

package main

import (
	"context"
	"sync"

	"fyne.io/systray"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"proxydesk/internal/trayicon"
)

var trayOnce sync.Once

func setupModernTray(ctx context.Context) {
	trayOnce.Do(func() {
		go systray.Run(func() {
			systray.SetIcon(trayicon.Icon)
			systray.SetTooltip("ProxyDesk 正在运行")

			showItem := systray.AddMenuItem("显示主窗口", "显示 ProxyDesk 主窗口")
			systray.AddSeparator()
			quitItem := systray.AddMenuItem("退出 ProxyDesk", "完全退出 ProxyDesk")

			go func() {
				for range showItem.ClickedCh {
					wailsruntime.Show(ctx)
				}
			}()
			go func() {
				<-quitItem.ClickedCh
				systray.Quit()
				wailsruntime.Quit(ctx)
			}()
		}, func() {})
	})
}
