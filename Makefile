SHELL := /bin/sh

GO ?= go
INKSCAPE ?= inkscape
MAGICK ?= magick
PNPM ?= pnpm
WAILS ?= wails
BIN_DIR ?= bin
BINARY := $(BIN_DIR)/gdit
CLI_PACKAGE := ./cli/cmd/gdit
GO_PACKAGES := ./core/... ./cli/... ./gui/...
GUI_DIR := gui
GUI_FRONTEND_DIR := $(GUI_DIR)/frontend
GUI_BINARY := $(GUI_DIR)/build/bin/gdit-gui
WAILS_BUILD_TAGS ?= webkit2_41
WAILS_TAG_FLAGS := $(if $(strip $(WAILS_BUILD_TAGS)),-tags "$(WAILS_BUILD_TAGS)")
HERO_SVG := assets/readme/hero.svg
HERO_PNG := assets/readme/hero.png
HERO_PNG_WIDTH := 2400
APP_MASCOT_PNG := assets/mascot.png
GUI_APP_ICON := $(GUI_DIR)/build/appicon.png
GUI_WINDOWS_ICON := $(GUI_DIR)/build/windows/icon.ico

.DEFAULT_GOAL := build

.NOTPARALLEL: all build check

.PHONY: all build build-cli build-gui run run-cli frontend-check frontend-build test test-race fmt fmt-check vet check png appicon clean help

all: check build

build: build-cli build-gui

build-cli:
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -o $(BINARY) $(CLI_PACKAGE)

build-gui:
	cd $(GUI_DIR) && $(WAILS) build -clean $(WAILS_TAG_FLAGS)

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

check: fmt-check frontend-check test vet

png:
	$(INKSCAPE) $(HERO_SVG) --export-filename=$(HERO_PNG) --export-width=$(HERO_PNG_WIDTH)

appicon:
	@task_icon_dir="$$(mktemp -d)"; \
		trap 'rm -rf "$$task_icon_dir"' EXIT; \
		$(MAGICK) $(APP_MASCOT_PNG) -crop 460x460+115+15 +repage -trim -resize '820x820' -background none -gravity center -extent 1024x1024 +repage $(GUI_APP_ICON); \
		$(MAGICK) $(GUI_APP_ICON) -background none -define icon:auto-resize=256,128,64,48,32,16 $(GUI_WINDOWS_ICON)

clean:
	rm -f $(BINARY) $(GUI_BINARY)
	@rmdir $(BIN_DIR) 2>/dev/null || true

help:
	@printf '%s\n' \
		'build      构建 CLI 与 Wails GUI（默认目标）' \
		'build-cli  构建 CLI 到 bin/gdit' \
		'build-gui  构建 GUI 到 gui/build/bin/gdit-gui' \
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
		'appicon    从现有吉祥物头像生成 GUI PNG/ICO 图标' \
		'all        检查后构建 CLI 与 GUI' \
		'clean      删除构建产物'
