.PHONY: all test vet build build-darwin-arm64 build-darwin-amd64 build-windows-amd64 build-windows-arm64 windows-syso clean

all: test build

# 版本号：默认取 git tag（v1.2.3 → 1.2.3，含未打 tag 时的提交哈希），
# 也可命令行覆盖：make build VERSION=1.2.3
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
VERSION := $(patsubst v%,%,$(VERSION))
LDFLAGS = -s -w -X change-timezone/internal/config.Version=$(VERSION)

vet:
	go vet ./...

test: vet
	go test ./...

# Windows UAC manifest 资源（构建期工具，非运行时依赖；网络不可用时会跳过并给出警告）
windows-syso: rsrc_windows_amd64.syso rsrc_windows_arm64.syso

rsrc_windows_amd64.syso: manifest.xml
	-go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -manifest manifest.xml -o $@ || echo "警告: 无法生成 amd64 manifest 资源，将构建无 UAC 声明的版本"

rsrc_windows_arm64.syso: manifest.xml
	-go run github.com/akavel/rsrc@v0.10.2 -arch arm64 -manifest manifest.xml -o $@ || echo "警告: 无法生成 arm64 manifest 资源，将构建无 UAC 声明的版本"

build: test build-darwin-arm64 build-darwin-amd64 build-windows-amd64 build-windows-arm64

build-darwin-arm64:
	mkdir -p dist
	GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags='$(LDFLAGS)' -o dist/change-timezone_darwin_arm64 .

build-darwin-amd64:
	mkdir -p dist
	GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags='$(LDFLAGS)' -o dist/change-timezone_darwin_amd64 .

build-windows-amd64: rsrc_windows_amd64.syso
	mkdir -p dist
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='$(LDFLAGS)' -o dist/change-timezone_windows_amd64.exe .

build-windows-arm64: rsrc_windows_arm64.syso
	mkdir -p dist
	GOOS=windows GOARCH=arm64 go build -trimpath -ldflags='$(LDFLAGS)' -o dist/change-timezone_windows_arm64.exe .

clean:
	rm -rf dist *.syso
