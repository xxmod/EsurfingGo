$ErrorActionPreference = "Stop"

$App = "esurfing"
$Out = "bin"
$LDFlags = "-s -w"

$Targets = @(
    @{ GOOS = "linux";   GOARCH = "amd64";   Label = "linux-amd64" },
    @{ GOOS = "linux";   GOARCH = "arm64";   Label = "linux-arm64" },
    @{ GOOS = "linux";   GOARCH = "arm"; GOARM = "5"; Label = "linux-armv5" },
    @{ GOOS = "linux";   GOARCH = "arm"; GOARM = "6"; Label = "linux-armv6" },
    @{ GOOS = "linux";   GOARCH = "arm"; GOARM = "7"; Label = "linux-armv7" },
    @{ GOOS = "linux";   GOARCH = "mips";    GOMIPS = "softfloat"; Label = "linux-mips" },
    @{ GOOS = "linux";   GOARCH = "mipsle";  GOMIPS = "softfloat"; Label = "linux-mipsle" },
    @{ GOOS = "windows"; GOARCH = "amd64";   Label = "windows-amd64" },
    @{ GOOS = "darwin";  GOARCH = "amd64";   Label = "darwin-amd64" },
    @{ GOOS = "darwin";  GOARCH = "arm64";   Label = "darwin-arm64" }
)

if (Test-Path $Out) { Remove-Item -Recurse -Force $Out }
New-Item -ItemType Directory -Path $Out | Out-Null

foreach ($t in $Targets) {
    $ext = if ($t.GOOS -eq "windows") { ".exe" } else { "" }
    $label = $t.Label
    $output = "$Out/$App-$label$ext"
    Write-Host "Building $label -> $output"

    $env:CGO_ENABLED = "0"
    $env:GOOS = $t.GOOS
    $env:GOARCH = $t.GOARCH
    if ($t.ContainsKey("GOARM"))  { $env:GOARM = $t.GOARM }   else { Remove-Item Env:\GOARM -ErrorAction SilentlyContinue }
    if ($t.ContainsKey("GOMIPS")) { $env:GOMIPS = $t.GOMIPS }  else { Remove-Item Env:\GOMIPS -ErrorAction SilentlyContinue }
    go build -trimpath -ldflags="$LDFlags" -o $output .
    if ($LASTEXITCODE -ne 0) { throw "Build failed for $label" }
}

# Restore env
Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue

Write-Host "Done. Binaries in ./$Out/"
