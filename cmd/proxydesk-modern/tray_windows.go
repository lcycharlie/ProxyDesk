//go:build windows

package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"proxydesk/internal/trayicon"
)

const (
	trayCallbackMessage = win.WM_USER + 76
	trayCommandShow     = 1001
	trayCommandExit     = 1002
)

var (
	trayOnce       sync.Once
	trayWindowProc uintptr
)

func setupModernTray(ctx context.Context) {
	trayOnce.Do(func() {
		go runModernTray(ctx)
	})
}

func runModernTray(ctx context.Context) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hInstance := win.GetModuleHandle(nil)
	className := syscall.StringToUTF16Ptr("ProxyDeskModernTrayWindow")
	trayWindowProc = syscall.NewCallback(func(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
		switch msg {
		case trayCallbackMessage:
			switch uint32(lParam) {
			case win.WM_LBUTTONUP, win.WM_LBUTTONDBLCLK:
				wailsruntime.Show(ctx)
				return 0
			case win.WM_RBUTTONUP:
				showTrayMenu(ctx, hwnd)
				return 0
			}
		case win.WM_COMMAND:
			switch uint32(wParam & 0xffff) {
			case trayCommandShow:
				wailsruntime.Show(ctx)
				return 0
			case trayCommandExit:
				removeTrayIcon(hwnd)
				wailsruntime.Quit(ctx)
				return 0
			}
		case win.WM_DESTROY:
			removeTrayIcon(hwnd)
			win.PostQuitMessage(0)
			return 0
		}
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	})

	wc := win.WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(win.WNDCLASSEX{})),
		LpfnWndProc:   trayWindowProc,
		HInstance:     hInstance,
		LpszClassName: className,
	}
	if win.RegisterClassEx(&wc) == 0 {
		return
	}

	hwnd := win.CreateWindowEx(0, className, syscall.StringToUTF16Ptr("ProxyDesk Tray"), 0, 0, 0, 0, 0, win.HWND_MESSAGE, 0, hInstance, nil)
	if hwnd == 0 {
		return
	}
	icon := loadTrayIcon()
	addTrayIcon(hwnd, icon)
	defer func() {
		removeTrayIcon(hwnd)
		if icon != 0 {
			win.DestroyIcon(icon)
		}
	}()

	var msg win.MSG
	for win.GetMessage(&msg, 0, 0, 0) > 0 {
		win.TranslateMessage(&msg)
		win.DispatchMessage(&msg)
	}
}

func loadTrayIcon() win.HICON {
	iconPath := filepath.Join(os.TempDir(), "ProxyDesk.ico")
	if err := os.WriteFile(iconPath, trayicon.Icon, 0600); err != nil {
		return 0
	}
	handle := win.LoadImage(0, syscall.StringToUTF16Ptr(iconPath), win.IMAGE_ICON, 16, 16, win.LR_LOADFROMFILE)
	return win.HICON(handle)
}

func addTrayIcon(hwnd win.HWND, icon win.HICON) {
	data := win.NOTIFYICONDATA{
		CbSize:           uint32(unsafe.Sizeof(win.NOTIFYICONDATA{})),
		HWnd:             hwnd,
		UID:              1,
		UFlags:           win.NIF_MESSAGE | win.NIF_ICON | win.NIF_TIP,
		UCallbackMessage: trayCallbackMessage,
		HIcon:            icon,
	}
	copy(data.SzTip[:], syscall.StringToUTF16("ProxyDesk 正在运行"))
	win.Shell_NotifyIcon(win.NIM_ADD, &data)
	data.UVersion = win.NOTIFYICON_VERSION
	win.Shell_NotifyIcon(win.NIM_SETVERSION, &data)
}

func removeTrayIcon(hwnd win.HWND) {
	data := win.NOTIFYICONDATA{
		CbSize: uint32(unsafe.Sizeof(win.NOTIFYICONDATA{})),
		HWnd:   hwnd,
		UID:    1,
	}
	win.Shell_NotifyIcon(win.NIM_DELETE, &data)
}

func showTrayMenu(ctx context.Context, hwnd win.HWND) {
	menu := win.CreatePopupMenu()
	if menu == 0 {
		return
	}
	defer win.DestroyMenu(menu)

	insertMenuText(menu, 0, trayCommandShow, "显示主窗口")
	insertMenuSeparator(menu, 1)
	insertMenuText(menu, 2, trayCommandExit, "退出 ProxyDesk")

	var point win.POINT
	win.GetCursorPos(&point)
	win.SetForegroundWindow(hwnd)
	command := win.TrackPopupMenu(menu, win.TPM_RETURNCMD|win.TPM_RIGHTBUTTON, point.X, point.Y, 0, hwnd, nil)
	switch command {
	case trayCommandShow:
		wailsruntime.Show(ctx)
	case trayCommandExit:
		removeTrayIcon(hwnd)
		wailsruntime.Quit(ctx)
	}
}

func insertMenuText(menu win.HMENU, position uint32, command uint32, text string) {
	label := syscall.StringToUTF16(text)
	item := win.MENUITEMINFO{
		CbSize:     uint32(unsafe.Sizeof(win.MENUITEMINFO{})),
		FMask:      win.MIIM_FTYPE | win.MIIM_ID | win.MIIM_STRING,
		FType:      win.MFT_STRING,
		WID:        command,
		DwTypeData: &label[0],
		Cch:        uint32(len(label) - 1),
	}
	win.InsertMenuItem(menu, position, true, &item)
}

func insertMenuSeparator(menu win.HMENU, position uint32) {
	item := win.MENUITEMINFO{
		CbSize: uint32(unsafe.Sizeof(win.MENUITEMINFO{})),
		FMask:  win.MIIM_FTYPE,
		FType:  win.MFT_SEPARATOR,
	}
	win.InsertMenuItem(menu, position, true, &item)
}
