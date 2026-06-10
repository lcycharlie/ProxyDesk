# ProxyDesk UI Migration Plan

目标：在不影响当前 Walk 版本功能的前提下，逐步迁移到现代桌面控制台 UI。

## 原则

- 现有 `cmd/proxydesk` Walk 入口保持可编译、可打包、可回退。
- 代理转发、供应商 API、端口管理、出口检测、系统代理、托盘逻辑优先保留 Go 实现。
- 新 UI 以并行入口推进，稳定前不替换现有入口。
- 每一步都必须通过功能回归清单和构建验证。

## 推荐目标架构

```text
Go core packages
  internal/app
  internal/localproxy
  internal/provider
  internal/proxyparse
  internal/systemproxy

Desktop shell
  current: cmd/proxydesk       Walk native UI
  next:    cmd/proxydesk-wails Wails backend bindings

Frontend
  React + TypeScript + CSS/Tailwind
```

## 阶段

### Phase 0: 保护当前功能

- 建立功能不回退清单。
- 保持 GitHub Actions 继续产出当前 Windows exe 和安装包。
- 任何 UI 迁移前先确认 `go test ./...` 和 Windows build 通过。

### Phase 1: 抽离可复用服务层

- 将 Walk UI 中的运行状态、转发列表操作、API 提取、出口检测等逻辑逐步沉到可复用服务层。
- Walk UI 改成调用服务层，不直接承载业务流程。
- Wails 后续也调用同一套服务层。

### Phase 2: 并行创建现代 UI

- 新增 Wails 入口和前端项目。
- 第一版只做只读工作台：展示状态、线路列表、日志。
- 不影响 Walk 版本。

### Phase 3: 逐步接入可写操作

- 接入新增配置、启动、停止、删除、测试出口。
- 接入供应商 API 国家/城市提取。
- 接入系统代理开关。
- 每接入一组功能，就按回归清单测试。

### Phase 4: 替换默认构建产物

- Wails 版本通过完整回归后，再切换 GitHub Actions 默认产物。
- Walk 版本保留为 legacy fallback 一段时间。

## UI 目标

- 现代控制台布局：左侧导航、顶部状态、右侧内容区。
- 卡片化信息层级。
- 统一按钮、输入框、表格、徽标和弹窗。
- 状态清晰：运行中、未启动、检测中、失败、已启用系统代理。
- 国家/城市选择支持搜索、国旗图片、中文国家名。

