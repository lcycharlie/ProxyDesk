# ProxyDesk Modern UI Prototype

这是 ProxyDesk 的现代 UI 原型，当前作为并行前端资源存在，不替换现有 Walk 版 Windows 应用。

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

直接用浏览器打开：

```text
ui/modern/index.html
```

## 下一步接入顺序

1. 新增现代 UI 桌面壳，优先考虑 WebView2/Wails 方案。
2. 保留 Walk 版作为 legacy fallback。
3. 先接只读状态：当前环境出口、本地 IP、转发列表、日志。
4. 再接可写操作：新增配置、启动、停止、删除、供应商 API、系统代理。
5. 通过 `docs/feature-parity-checklist.md` 后再替换默认打包产物。
