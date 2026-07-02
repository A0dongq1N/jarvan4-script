# Makefile — jarvan4-script 开发工具集
# 用法: make <target>  (查看所有 target: make help)

SO_DIR ?= dist

##@ 编译 & 上传

.PHONY: build-so upload-so publish-so local-so

build-so: ## 编译所有正式脚本 .so（跳过下划线前缀目录）
	@mkdir -p $(SO_DIR)
	@for d in scripts/*/; do \
		[ -f "$$d/main.go" ] || continue; \
		name=$$(basename "$$d"); \
		[[ "$$name" == _* ]] && continue; \
		echo "==> building $$name"; \
		go build -tags plugin -buildmode=plugin -o "$(SO_DIR)/$$name.so" "./$$d"; \
	done
	@echo "✓ 编译完成: $(SO_DIR)/"

upload-so: ## 编译 + 上传到 COS + 通知 Master（需先设置 COS_SECRET_ID/COS_SECRET_KEY 环境变量）
	./scripts/upload-so.sh

publish-so: upload-so ## 同 upload-so（别名）

local-so: build-so ## 仅编译不上传（本地调试用，DB artifactUrl 设为本地路径）
	@echo ""
	@echo "本地 .so 路径: $(CURDIR)/$(SO_DIR)/"
	@echo "DB artifactUrl 示例:"
	@for f in $(SO_DIR)/*.so; do \
		[ -f "$$f" ] || continue; \
		name=$$(basename "$$f" .so); \
		echo "  $$name → $(CURDIR)/$(SO_DIR)/$$name.so"; \
	done

##@ 环境变量配置

.PHONY: env-check

env-check: ## 检查 COS 环境变量是否已设置
	@if [ -z "$$COS_SECRET_ID" ] || [ -z "$$COS_SECRET_KEY" ]; then \
		echo "✗ COS_SECRET_ID 或 COS_SECRET_KEY 未设置"; \
		echo ""; \
		echo "首次配置（写入 ~/.bashrc 永久生效）:"; \
		echo "  echo 'export COS_SECRET_ID=你的SecretId' >> ~/.bashrc"; \
		echo "  echo 'export COS_SECRET_KEY=你的SecretKey' >> ~/.bashrc"; \
		echo "  source ~/.bashrc"; \
	else \
		echo "✓ COS 环境变量已设置 (SECRET_ID=$${COS_SECRET_ID:0:8}...)"; \
	fi

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
	@echo "环境变量:"
	@echo "  COS_SECRET_ID   腾讯云 SecretId（首次设置，写入 ~/.bashrc）"
	@echo "  COS_SECRET_KEY  腾讯云 SecretKey（首次设置，写入 ~/.bashrc）"
	@echo "  COS_BUCKET      COS 桶名（默认 jarvan4-1257748620）"
	@echo "  COS_REGION      COS 地域（默认 ap-guangzhou）"
	@echo "  MASTER_URL      Master 地址（默认 http://localhost:8090）"
	@echo ""
	@echo "首次配置: make env-check 查看配置指引"
