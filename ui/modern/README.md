# ProxyDesk Modern UI Prototype

这是 ProxyDesk 的现代 UI 原型，当前已经通过 `cmd/proxydesk-modern` 嵌入到并行 Windows 桌面应用中，不替换现有 Walk 版 Windows 应用。

## 目标

- 确认最终视觉方向：左侧导航、顶部状态、卡片、表格、圆角按钮、状态徽标。
- 提前稳定页面结构，后续接入 Go 后端时不重新设计交互。
- 不影响当前 `cmd/proxydesk` 的构建、打包和功能。

## 当前页面

- 概览：当前连接、系统代理、使用提示。
- 线路配置：本地协议、上游协议、监听地址、端口、上游代理。
- 转发列表：多线路状态表格。
- 设置：端口范围、供应商 API、运行日志。

## 打开方式

直接用浏览器预览：

```text
ui/modern/index.html
```

构建现代桌面应用：

```powershell
go build -tags desktop,production -ldflags="-H windowsgui -s -w" -o dist/ProxyDeskModern.exe ./cmd/proxydesk-modern
```

`scripts/build-windows.ps1` 和 GitHub Actions 会同时产出：

- `dist/ProxyDesk.exe`：当前功能完整的 Walk 版。
- `dist/ProxyDeskModern.exe`：已嵌入现代 UI 的 Wails/WebView2 版。

## 下一步接入顺序

1. 保留 Walk 版作为 legacy fallback。
2. 先接只读状态：当前环境出口、本地 IP、转发列表、日志。
3. 再接可写操作：新增配置、启动、停止、删除、供应商 API、系统代理。
4. 通过 `docs/feature-parity-checklist.md` 后再替换默认打包产物。
