SHELL := /bin/sh

GO ?= go
INKSCAPE ?= inkscape
BIN_DIR ?= bin
BINARY := $(BIN_DIR)/gdit
CLI_PACKAGE := ./cli/cmd/gdit
GO_PACKAGES := ./core/... ./cli/...
HERO_SVG := assets/readme/hero.svg
HERO_PNG := assets/readme/hero.png
HERO_PNG_WIDTH := 2400

.DEFAULT_GOAL := build

.PHONY: all build run test test-race fmt fmt-check vet check png clean help

all: check build

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -o $(BINARY) $(CLI_PACKAGE)

# 构建后运行 CLI，后面的目标名作为参数透传，如：make run list
# 以 - 开头的参数会被 make 当作自身选项，需用 make run ARGS="--help" 传入；
# 与真实目标重名时（如 make run help）该目标也会照常执行
RUN_ARGS := $(if $(filter run,$(MAKECMDGOALS)),$(filter-out run,$(MAKECMDGOALS)))
run: build
	$(BINARY) $(RUN_ARGS) $(ARGS)

# 兜底规则：run 后面的参数被 make 视为目标，这里静默放行；
# 不在 run 参数列表中的未知目标仍按惯例报错
%:
	@if [ -n "$(filter $@,$(RUN_ARGS))" ]; then \
		:; \
	else \
		echo "make: *** 没有规则可制作目标 '$@'。" >&2; \
		exit 2; \
	fi

test:
	$(GO) test $(GO_PACKAGES)

test-race:
	$(GO) test -race $(GO_PACKAGES)

fmt:
	$(GO) fmt $(GO_PACKAGES)

fmt-check:
	@test -z "$$(gofmt -l core cli)" || { \
		gofmt -l core cli; \
		echo 'Go 文件尚未格式化，请运行 make fmt' >&2; \
		exit 1; \
	}

vet:
	$(GO) vet $(GO_PACKAGES)

check: fmt-check test vet

png:
	$(INKSCAPE) $(HERO_SVG) --export-filename=$(HERO_PNG) --export-width=$(HERO_PNG_WIDTH)

clean:
	rm -f $(BINARY)
	@rmdir $(BIN_DIR) 2>/dev/null || true

help:
	@printf '%s\n' \
		'build      构建 CLI 到 bin/gdit（默认目标）' \
		'run        构建并运行 CLI，参数直接跟在后面，如 make run list' \
		'test       运行 core 和 CLI 测试' \
		'test-race  使用竞争检测器运行测试' \
		'fmt        格式化所有 Go 包' \
		'fmt-check  检查 Go 文件格式但不修改文件' \
		'vet        静态检查所有 Go 包' \
		'check      运行格式检查、测试和静态检查' \
		'png        使用 Inkscape 将 README hero 渲染为 2400px 宽 PNG' \
		'all        检查后构建 CLI' \
		'clean      删除构建产物'
