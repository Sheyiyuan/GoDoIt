ifeq ($(OS),Windows_NT)
SHELL := bash
else
SHELL := /bin/sh
endif

GO ?= go
INKSCAPE ?= inkscape
MAGICK ?= magick
PNPM ?= pnpm
WAILS ?= wails
BIN_DIR ?= bin
BINARY := $(BIN_DIR)/gdit
CLI_PACKAGE := ./cli/cmd/gdit
GO_PACKAGES := ./core/... ./cli/... ./gui/...
BUILDINFO_PACKAGE := github.com/Sheyiyuan/GoDoIt/core/buildinfo
BUILD_VERSION ?= dev
BUILD_COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null)
BUILD_DATE ?=
BUILD_LDFLAGS := -X $(BUILDINFO_PACKAGE).version=$(BUILD_VERSION)
ifneq ($(strip $(BUILD_COMMIT)),)
BUILD_LDFLAGS += -X $(BUILDINFO_PACKAGE).commit=$(BUILD_COMMIT)
endif
ifneq ($(strip $(BUILD_DATE)),)
BUILD_LDFLAGS += -X $(BUILDINFO_PACKAGE).buildDate=$(BUILD_DATE)
endif
GUI_DIR := gui
GUI_FRONTEND_DIR := $(GUI_DIR)/frontend
# macOS 上 wails 将应用打包为 .app 包，可执行文件位于 Contents/MacOS 下；
# Linux/Windows 为裸二进制（Windows 追加 .exe 后缀）
UNAME_S := $(shell uname -s)
ifeq ($(OS),Windows_NT)
GUI_BINARY := $(GUI_DIR)/build/bin/gdit-gui.exe
WAILS_BUILD_TAGS ?=
else ifeq ($(UNAME_S),Darwin)
GUI_APP_BUNDLE := $(GUI_DIR)/build/bin/GoDoIt.app
GUI_BINARY := $(GUI_APP_BUNDLE)/Contents/MacOS/gdit-gui
WAILS_BUILD_TAGS ?=
else
GUI_BINARY := $(GUI_DIR)/build/bin/gdit-gui
WAILS_BUILD_TAGS ?= webkit2_41
endif
WAILS_TAG_FLAGS := $(if $(strip $(WAILS_BUILD_TAGS)),-tags "$(WAILS_BUILD_TAGS)")
HERO_SVG := assets/readme/hero.svg
HERO_PNG := assets/readme/hero.png
HERO_PNG_WIDTH := 2400
APP_ICON_PNG := assets/icon.png
GUI_APP_ICON := $(GUI_DIR)/build/appicon.png
GUI_WINDOWS_ICON := $(GUI_DIR)/build/windows/icon.ico
RELEASE_TOOL := $(GO) run ./core/cmd/gdit-release
RELEASE_VERSION ?= $(shell scripts/derive-version.sh version 2>/dev/null)
RELEASE_COMMIT ?= $(shell scripts/derive-version.sh commit 2>/dev/null)
RELEASE_BUILD_DATE ?= $(shell scripts/derive-version.sh build_date 2>/dev/null)
SOURCE_DATE_EPOCH ?= $(shell scripts/derive-version.sh source_date_epoch 2>/dev/null)
RELEASE_LDFLAGS := -X $(BUILDINFO_PACKAGE).version=$(RELEASE_VERSION) -X $(BUILDINFO_PACKAGE).commit=$(RELEASE_COMMIT) -X $(BUILDINFO_PACKAGE).buildDate=$(RELEASE_BUILD_DATE)
DIST_DIR ?= dist
LINUX_RELEASE_ROOT := $(abspath build/release/linux_amd64)
DARWIN_RELEASE_ROOT := $(abspath build/release/darwin_arm64)
WINDOWS_RELEASE_ROOT := $(abspath build/release/windows_amd64)
LINUX_PROJECT := $(LINUX_RELEASE_ROOT)/project
DARWIN_PROJECT := $(DARWIN_RELEASE_ROOT)/project
WINDOWS_PROJECT := $(WINDOWS_RELEASE_ROOT)/project

.DEFAULT_GOAL := build

.NOTPARALLEL: all build check package-linux package-macos package-windows

.PHONY: all build build-cli build-gui run run-cli frontend-check frontend-build test test-race fmt fmt-check vet check png appicon appicon-check legal legal-check release-validate package-linux package-macos package-windows package-installers package-linux-installers package-macos-dmg package-windows-installer release-checksums release-verify clean help

all: check build

build: build-cli build-gui

build-cli:
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -ldflags "$(BUILD_LDFLAGS)" -o $(BINARY) $(CLI_PACKAGE)

build-gui:
	cd $(GUI_DIR) && $(WAILS) build -clean -trimpath -ldflags "$(BUILD_LDFLAGS)" $(WAILS_TAG_FLAGS)

# 构建并运行 GUI；GUI 参数通过 ARGS 传入，如：make run ARGS="--devtools"
# GUI 构建失败不得回退为启动 CLI。
run: build-gui
	$(GUI_BINARY) $(ARGS)

# 构建后运行 CLI，后面的目标名作为参数透传，如：make run-cli list
# 以 - 开头的参数会被 make 当作自身选项，需用 make run-cli ARGS="--help" 传入；
# 与真实目标重名时（如 make run-cli help）该目标也会照常执行
RUN_CLI_ARGS := $(if $(filter run-cli,$(MAKECMDGOALS)),$(filter-out run-cli,$(MAKECMDGOALS)))
run-cli: build-cli
	$(BINARY) $(RUN_CLI_ARGS) $(ARGS)

# 兜底规则：run-cli 后面的参数被 make 视为目标，这里静默放行；
# 不在 run-cli 参数列表中的未知目标仍按惯例报错
%:
	@if [ -n "$(filter $@,$(RUN_CLI_ARGS))" ]; then \
		:; \
	else \
		echo "make: *** 没有规则可制作目标 '$@'。" >&2; \
		exit 2; \
	fi

frontend-check:
	cd $(GUI_FRONTEND_DIR) && $(PNPM) run check

frontend-build:
	cd $(GUI_FRONTEND_DIR) && $(PNPM) run build

test: frontend-build
	$(GO) test $(GO_PACKAGES)

test-race: frontend-build
	$(GO) test -race $(GO_PACKAGES)

fmt:
	$(GO) fmt $(GO_PACKAGES)

fmt-check:
	@test -z "$$(gofmt -l core cli gui)" || { \
		gofmt -l core cli gui; \
		echo 'Go 文件尚未格式化，请运行 make fmt' >&2; \
		exit 1; \
	}

vet: frontend-build
	$(GO) vet $(GO_PACKAGES)

check: fmt-check frontend-check test vet legal-check appicon-check

png:
	$(INKSCAPE) $(HERO_SVG) --export-filename=$(HERO_PNG) --export-width=$(HERO_PNG_WIDTH)

appicon:
	@mkdir -p $(dir $(GUI_APP_ICON)) $(dir $(GUI_WINDOWS_ICON))
	cp $(APP_ICON_PNG) $(GUI_APP_ICON)
	$(MAGICK) $(APP_ICON_PNG) -background none -define icon:auto-resize=256,128,64,48,32,16 $(GUI_WINDOWS_ICON)

appicon-check:
	$(RELEASE_TOOL) verify-icons --source $(APP_ICON_PNG) --wails $(GUI_APP_ICON) --windows $(GUI_WINDOWS_ICON)

legal:
	$(RELEASE_TOOL) notices --root . --metadata scripts/third_party_licenses.json --output THIRD_PARTY_NOTICES.txt

legal-check:
	$(RELEASE_TOOL) notices --root . --metadata scripts/third_party_licenses.json --output THIRD_PARTY_NOTICES.txt --check

release-validate:
	@test -n "$(RELEASE_VERSION)" || { echo 'RELEASE_VERSION 不能为空' >&2; exit 1; }
	@test -n "$(RELEASE_COMMIT)" || { echo 'RELEASE_COMMIT 不能为空' >&2; exit 1; }
	@test -n "$(RELEASE_BUILD_DATE)" || { echo 'RELEASE_BUILD_DATE 不能为空' >&2; exit 1; }
	@test -n "$(SOURCE_DATE_EPOCH)" || { echo 'SOURCE_DATE_EPOCH 不能为空' >&2; exit 1; }
	@test -s LICENSE && test -s THIRD_PARTY_NOTICES.txt

package-linux: release-validate frontend-build
	@test "$$(uname -s)" = Linux && test "$$(uname -m)" = x86_64
	$(RELEASE_TOOL) stage-gui --root . --output $(LINUX_PROJECT) --version $(RELEASE_VERSION)
	@mkdir -p $(LINUX_RELEASE_ROOT)/bin $(DIST_DIR)
	cd $(LINUX_PROJECT) && GOWORK=$(LINUX_PROJECT)/go.work $(GO) build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o $(LINUX_RELEASE_ROOT)/bin/gdit ./cli/cmd/gdit
	cd $(LINUX_PROJECT)/gui && GOWORK=$(LINUX_PROJECT)/go.work $(WAILS) build -clean -s -skipbindings -m -trimpath -tags "webkit2_41" -ldflags "$(RELEASE_LDFLAGS)"
	$(RELEASE_TOOL) verify-binaries --cli $(LINUX_RELEASE_ROOT)/bin/gdit --gui $(LINUX_PROJECT)/gui/build/bin/gdit-gui --version $(RELEASE_VERSION) --commit $(RELEASE_COMMIT) --build-date $(RELEASE_BUILD_DATE)
	SOURCE_DATE_EPOCH=$(SOURCE_DATE_EPOCH) $(RELEASE_TOOL) package --root . --platform linux_amd64 --version $(RELEASE_VERSION) --cli $(LINUX_RELEASE_ROOT)/bin/gdit --gui $(LINUX_PROJECT)/gui/build/bin/gdit-gui --output $(DIST_DIR)/GoDoIt_$(RELEASE_VERSION)_linux_amd64.tar.gz

package-macos: release-validate frontend-build
	@test "$$(uname -s)" = Darwin && test "$$(uname -m)" = arm64
	$(RELEASE_TOOL) stage-gui --root . --output $(DARWIN_PROJECT) --version $(RELEASE_VERSION)
	@mkdir -p $(DARWIN_RELEASE_ROOT)/bin $(DIST_DIR)
	cd $(DARWIN_PROJECT) && GOWORK=$(DARWIN_PROJECT)/go.work $(GO) build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o $(DARWIN_RELEASE_ROOT)/bin/gdit ./cli/cmd/gdit
	cd $(DARWIN_PROJECT)/gui && GOWORK=$(DARWIN_PROJECT)/go.work $(WAILS) build -clean -s -skipbindings -m -trimpath -ldflags "$(RELEASE_LDFLAGS)"
	$(RELEASE_TOOL) install-macos-legal --app $(DARWIN_PROJECT)/gui/build/bin/GoDoIt.app --license LICENSE --notices THIRD_PARTY_NOTICES.txt
	$(RELEASE_TOOL) verify-binaries --cli $(DARWIN_RELEASE_ROOT)/bin/gdit --gui $(DARWIN_PROJECT)/gui/build/bin/GoDoIt.app/Contents/MacOS/gdit-gui --version $(RELEASE_VERSION) --commit $(RELEASE_COMMIT) --build-date $(RELEASE_BUILD_DATE)
	@test "$$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' $(DARWIN_PROJECT)/gui/build/bin/GoDoIt.app/Contents/Info.plist)" = "$(RELEASE_VERSION)"
	@test "$$(/usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' $(DARWIN_PROJECT)/gui/build/bin/GoDoIt.app/Contents/Info.plist)" = "$(RELEASE_VERSION)"
	@test "$$(lipo -archs $(DARWIN_PROJECT)/gui/build/bin/GoDoIt.app/Contents/MacOS/gdit-gui)" = arm64
	codesign --force --deep --sign - $(DARWIN_PROJECT)/gui/build/bin/GoDoIt.app
	codesign --verify --strict --verbose=2 $(DARWIN_PROJECT)/gui/build/bin/GoDoIt.app
	SOURCE_DATE_EPOCH=$(SOURCE_DATE_EPOCH) $(RELEASE_TOOL) package --root . --platform darwin_arm64 --version $(RELEASE_VERSION) --cli $(DARWIN_RELEASE_ROOT)/bin/gdit --gui $(DARWIN_PROJECT)/gui/build/bin/GoDoIt.app --output $(DIST_DIR)/GoDoIt_$(RELEASE_VERSION)_darwin_arm64.zip

package-windows: release-validate frontend-build
	$(RELEASE_TOOL) stage-gui --root . --output $(WINDOWS_PROJECT) --version $(RELEASE_VERSION)
	@mkdir -p $(WINDOWS_RELEASE_ROOT)/bin $(DIST_DIR)
	cd $(WINDOWS_PROJECT) && GOWORK=$(WINDOWS_PROJECT)/go.work $(GO) build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o $(WINDOWS_RELEASE_ROOT)/bin/gdit.exe ./cli/cmd/gdit
	cd $(WINDOWS_PROJECT)/gui && GOWORK=$(WINDOWS_PROJECT)/go.work $(WAILS) build -clean -s -skipbindings -m -trimpath -webview2 browser -ldflags "$(RELEASE_LDFLAGS)"
	$(RELEASE_TOOL) verify-binaries --cli $(WINDOWS_RELEASE_ROOT)/bin/gdit.exe --gui $(WINDOWS_PROJECT)/gui/build/bin/gdit-gui.exe --version $(RELEASE_VERSION) --commit $(RELEASE_COMMIT) --build-date $(RELEASE_BUILD_DATE)
	SOURCE_DATE_EPOCH=$(SOURCE_DATE_EPOCH) $(RELEASE_TOOL) package --root . --platform windows_amd64 --version $(RELEASE_VERSION) --cli $(WINDOWS_RELEASE_ROOT)/bin/gdit.exe --gui $(WINDOWS_PROJECT)/gui/build/bin/gdit-gui.exe --output $(DIST_DIR)/GoDoIt_$(RELEASE_VERSION)_windows_amd64.zip

package-installers:
	@test -x scripts/package-linux-installers.sh && test -x scripts/package-macos-dmg.sh
	@test -f dist/GoDoIt_$(RELEASE_VERSION)_linux_amd64.tar.gz
	@test -f dist/GoDoIt_$(RELEASE_VERSION)_darwin_arm64.zip
	@test -f dist/GoDoIt_$(RELEASE_VERSION)_windows_amd64.zip

package-linux-installers:
	scripts/package-linux-installers.sh $(RELEASE_VERSION) $(LINUX_RELEASE_ROOT) $(DIST_DIR)

package-macos-dmg:
	scripts/package-macos-dmg.sh $(RELEASE_VERSION) $(DARWIN_PROJECT)/gui/build/bin/GoDoIt.app $(DIST_DIR)

package-windows-installer:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/package-windows-installer.ps1 -Version $(RELEASE_VERSION) -Root $(WINDOWS_RELEASE_ROOT) -Output $(DIST_DIR)

release-checksums:
	$(RELEASE_TOOL) checksums --dir $(DIST_DIR) --version $(RELEASE_VERSION)

release-verify:
	$(RELEASE_TOOL) verify-final --root . --dir $(DIST_DIR) --version $(RELEASE_VERSION)

clean:
	rm -f $(BINARY)
	rm -rf $(GUI_APP_BUNDLE) $(GUI_BINARY)
	rm -rf build/release
	@rmdir $(BIN_DIR) 2>/dev/null || true

help:
	@printf '%s\n' \
		'build      构建 CLI 与 Wails GUI（默认目标）' \
		'build-cli  构建 CLI 到 bin/gdit' \
		'build-gui  构建 GUI 到 gui/build/bin（macOS 为 GoDoIt.app）' \
		'run        构建并运行 GUI，可用 ARGS 传入参数' \
		'run-cli    构建并运行 CLI，参数直接跟在后面，如 make run-cli list' \
		'frontend-check  运行前端类型检查与组件测试' \
		'frontend-build  构建 React 前端' \
		'test       构建前端并运行 core、CLI 和 GUI 测试' \
		'test-race  使用竞争检测器运行测试' \
		'fmt        格式化所有 Go 包' \
		'fmt-check  检查 Go 文件格式但不修改文件' \
		'vet        静态检查所有 Go 包' \
		'check      运行前端检查、格式检查、测试和静态检查' \
		'png        使用 Inkscape 将 README hero 渲染为 2400px 宽 PNG' \
		'appicon    从统一品牌图生成 GUI PNG/ICO 图标' \
		'appicon-check  校验提交的 GUI 图标来自统一品牌图' \
		'legal      从锁文件与固定元数据生成第三方声明' \
		'legal-check  校验第三方声明与当前运行时依赖一致' \
		'package-linux   原生构建 Linux amd64 发布归档' \
		'package-macos   原生构建 macOS arm64 发布归档并 ad-hoc 签名' \
		'package-windows 原生构建 Windows amd64 发布归档' \
		'package-linux-installers  生成 Linux deb/rpm 安装包' \
		'package-macos-dmg  生成 macOS dmg 磁盘映像' \
		'package-windows-installer  生成可选安装目录并创建快捷方式的 Windows 安装程序' \
		'release-checksums  为平台归档和安装包生成 SHA256SUMS' \
		'release-verify     校验最终发布目录白名单、摘要和包内容' \
		'all        检查后构建 CLI 与 GUI' \
		'clean      删除构建产物'
