#!/usr/bin/env sh

set -e

mkdir -p build

~/go/bin/go-winres simply

GOOS="linux" \
    GOARCH="amd64" \
    go build -trimpath -ldflags="-s -w" -tags "noaudio" \
    -o "./build/tagit-launcher"

GOOS="windows" \
    GOARCH="amd64" \
    CGO_ENABLED="1" \
    CC="x86_64-w64-mingw32-gcc" \
    CXX="x86_64-w64-mingw32-g++" \
    go build -trimpath -ldflags="-s -w -H=windowsgui" -tags "noaudio" \
    -o "./build/tagit-launcher.exe"
