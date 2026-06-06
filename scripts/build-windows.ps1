$ErrorActionPreference = "Stop"

go mod tidy
New-Item -ItemType Directory -Force -Path dist | Out-Null
go run github.com/akavel/rsrc@v0.10.2 -manifest build/windows/ProxyDesk.exe.manifest -ico build/windows/ProxyDesk.ico -o cmd/proxydesk/rsrc.syso
go run ./build/tools/fix_icon_group.go cmd/proxydesk/rsrc.syso
go build -ldflags="-H windowsgui -s -w" -o dist/ProxyDesk.exe ./cmd/proxydesk

if (Get-Command iscc -ErrorAction SilentlyContinue) {
    iscc build/windows/ProxyDesk.iss
    Write-Host "Built dist/ProxyDeskSetup.exe"
} else {
    Write-Host "Inno Setup is not installed; skipped installer build."
}

Write-Host "Built dist/ProxyDesk.exe"
