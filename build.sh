#!/usr/bin/env sh

set -e

mkdir -p build

~/go/bin/go-winres simply

GOOS="linux" \
    GOARCH="amd64" \
    CC="zig cc -target x86_64-linux-gnu \
        -isystem /usr/include \
        -L/usr/lib" \
    CXX="zig c++ -target x86_64-linux-gnu \
        -isystem /usr/include \
        -L/usr/lib" \
    go build -trimpath -ldflags="-s -w" -tags "noaudio" \
    -o "./build/tagit-launcher-linux"

GOOS="windows" \
    GOARCH="amd64" \
    CC="zig cc -target x86_64-windows-gnu" \
    CXX="zig c++ -target x86_64-windows-gnu" \
    go build -trimpath -ldflags="-s -w -H=windowsgui" -tags "noaudio" \
    -o "./build/tagit-launcher.exe"
