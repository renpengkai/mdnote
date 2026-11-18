#!/bin/sh
# 在 macOS 上构建 MDNote。
#
# 为什么只能在 Mac 上编译：Viva 的 macOS 后端通过 cgo 调用 Cocoa / Metal
# （Objective-C），无法交叉编译。需要 Xcode 命令行工具：
#     xcode-select --install
#
# 运行：
#     sh build-macos.sh
# 产物在 build/ 目录：build/mdnote（裸可执行文件）与 build/MDNote.app（应用包）。
set -eu
cd "$(dirname "$0")"

echo "== go test =="
go test ./... -count=1

echo
echo "== 构建裸可执行文件 build/mdnote =="
mkdir -p build
go build -trimpath -ldflags="-s -w -buildid=" -o build/mdnote .

echo
echo "== 打包 MDNote.app =="
go build -o build/viva github.com/vivaui/viva/cmd/viva
./build/viva package --target darwin --format zip

echo
echo "== 产物 =="
ls -la build