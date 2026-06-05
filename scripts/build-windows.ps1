$ErrorActionPreference = "Stop"

go mod tidy
New-Item -ItemType Directory -Force -Path dist | Out-Null
go build -ldflags="-H windowsgui -s -w" -o dist/ProxyDesk.exe ./cmd/proxydesk

Write-Host "Built dist/ProxyDesk.exe"

