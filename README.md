# ProxyDesk

ProxyDesk is a Windows-first desktop proxy forwarder.

The first version is designed around this workflow:

1. Select a country in the app.
2. Fetch or paste an upstream residential proxy.
3. Bind it to a local port such as `127.0.0.1:7890`.
4. Point browsers, fingerprint browsers, or tools to the local port.

## Supported proxy format

```text
host:port:username:password
```

Examples:

```text
global.rpip.lokiproxy.com:35001:USER096836-session-5MHDsJKATDS:48a951
107.150.104.202:2672:77ad76a2c1f1:dvy9mmpdknfcxxwcitwu
```

## Build

Local macOS development:

```bash
go mod tidy
go run ./cmd/proxydesk
```

Windows native build:

```powershell
.\scripts\build-windows.ps1
```

Windows EXE build is also configured in `.github/workflows/windows-build.yml`.
This is the preferred path when developing on macOS.

The desktop UI uses native Windows controls through `github.com/lxn/walk`, so it
does not require OpenGL on the target Windows computer.

## Current MVP

- Windows desktop UI with no login.
- Country and city/state selection for supplier API extraction.
- Upstream proxy input in `host:port:user:pass` format.
- Supplier API fetch by selected country and optional city/state.
- Local HTTP proxy forwarding, including HTTPS `CONNECT`.
- Local SOCKS5 proxy forwarding.
- Multiple simultaneous port routes.
- Optional Windows system proxy switch.
- Exit IP check through the local port.
- System tray minimize-on-close behavior.
- Windows installer build through GitHub Actions.

## UI migration

The current production UI is built with Walk native Windows controls.
Modern UI migration will be done in parallel so existing features remain
available while the new desktop console is built.

See:

- `docs/ui-migration-plan.md`
- `docs/feature-parity-checklist.md`
