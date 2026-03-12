#!/usr/bin/env bash
set -euo pipefail

APP="esurfing"
OUT="bin"
LDFLAGS="-s -w"

TARGETS=(
  "linux/amd64"
  "linux/arm64"
  "windows/amd64"
  "darwin/amd64"
  "darwin/arm64"
)

rm -rf "$OUT"
mkdir -p "$OUT"

for target in "${TARGETS[@]}"; do
  GOOS="${target%/*}"
  GOARCH="${target#*/}"
  output="${OUT}/${APP}-${GOOS}-${GOARCH}"
  if [ "$GOOS" = "windows" ]; then
    output="${output}.exe"
  fi
  echo "Building ${GOOS}/${GOARCH} -> ${output}"
  GOOS="$GOOS" GOARCH="$GOARCH" go build -trimpath -ldflags="$LDFLAGS" -o "$output" .
done

echo "Done. Binaries in ./${OUT}/"
