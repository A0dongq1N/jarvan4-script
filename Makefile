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

upload-so: ## 编译 + 上传到 COS + 通知 Master（需要 COS_SECRET_ID/COS_SECRET_KEY）
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
	@echo "  COS_SECRET_ID   腾讯云 SecretId（upload-so 必填）"
	@echo "  COS_SECRET_KEY  腾讯云 SecretKey（upload-so 必填）"
	@echo "  COS_BUCKET      COS 桶名（默认 jarvan4-1257748620）"
	@echo "  COS_REGION      COS 地域（默认 ap-guangzhou）"
	@echo "  MASTER_URL      Master 地址（默认 http://localhost:8090）"
	@echo "  SKIP_UPLOAD=1   只编译不上传（local-so 等效）"
