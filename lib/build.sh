#!/usr/bin/env bash
set -e
mkdir -p build
go build -buildmode=c-archive -o ./build/dcproxy.a ./lib.go
go build -buildmode=c-shared -o ./build/dcproxy.so ./lib.go
cp dcproxy_types.h ./build/
echo "done"