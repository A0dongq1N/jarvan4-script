# Makefile — jarvan4-script 开发工具集
# 用法: make <target>  (查看所有 target: make help)

BIN_DIR ?= dist

##@ 编译

.PHONY: build-bin

build-bin: ## 编译所有正式脚本二进制（跳过下划线前缀目录）
	@mkdir -p $(BIN_DIR)
	@for d in scripts/*/; do \
		[ -f "$$d/main.go" ] || continue; \
		name=$$(basename "$$d"); \
		[[ "$$name" == _* ]] && continue; \
		echo "==> building $$name"; \
		go build -o "$(BIN_DIR)/$$name" "./$$d"; \
	done
	@echo "✓ 编译完成: $(BIN_DIR)/"

##@ 帮助

.PHONY: help

help: ## 显示帮助信息
	@echo ""
	@echo "jarvan4-script Makefile"
	@echo "用法: make <target>"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"; printf "Targets:\n"} \
	/^[a-zA-Z_-]+:.*?##/ { printf "  %-16s %s\n", $$1, $$2 } \
	/^##@/ { printf "\n%s\n", substr($$0, 5) }' $(MAKEFILE_LIST)
	@echo ""
