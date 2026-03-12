$ErrorActionPreference = "Stop"

$App = "esurfing"
$Out = "bin"
$LDFlags = "-s -w"

$Targets = @(
    @{ GOOS = "linux";   GOARCH = "amd64" },
    @{ GOOS = "linux";   GOARCH = "arm64" },
    @{ GOOS = "windows"; GOARCH = "amd64" },
    @{ GOOS = "darwin";  GOARCH = "amd64" },
    @{ GOOS = "darwin";  GOARCH = "arm64" }
)

if (Test-Path $Out) { Remove-Item -Recurse -Force $Out }
New-Item -ItemType Directory -Path $Out | Out-Null

foreach ($t in $Targets) {
    $ext = if ($t.GOOS -eq "windows") { ".exe" } else { "" }
    $output = "$Out/$App-$($t.GOOS)-$($t.GOARCH)$ext"
    Write-Host "Building $($t.GOOS)/$($t.GOARCH) -> $output"

    $env:GOOS = $t.GOOS
    $env:GOARCH = $t.GOARCH
    go build -trimpath -ldflags="$LDFlags" -o $output .
    if ($LASTEXITCODE -ne 0) { throw "Build failed for $($t.GOOS)/$($t.GOARCH)" }
}

# Restore env
Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue

Write-Host "Done. Binaries in ./$Out/"
