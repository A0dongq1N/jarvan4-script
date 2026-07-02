#!/bin/bash
# upload-so.sh — 编译 .so 并上传到 COS，通知 Master
#
# 用法:
#   本地: COS_SECRET_ID=xxx COS_SECRET_KEY=xxx ./scripts/upload-so.sh
#   CI:   环境变量已配置，直接 ./scripts/upload-so.sh
#
# 环境变量:
#   COS_SECRET_ID   — 腾讯云 SecretId（必填）
#   COS_SECRET_KEY  — 腾讯云 SecretKey（必填）
#   COS_BUCKET      — COS 桶名（可选，默认 jarvan4-1257748620）
#   COS_REGION      — COS 地域（可选，默认 ap-guangzhou）
#   MASTER_URL      — Master 地址（可选，默认 http://localhost:8090）
#   COMMIT_HASH     — 版本号（可选，默认 git SHA）
#   SKIP_UPLOAD     — 设为 1 只编译不上传（本地调试用）

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

# 默认值
COS_BUCKET="${COS_BUCKET:-jarvan4-1257748620}"
COS_REGION="${COS_REGION:-ap-guangzhou}"
MASTER_URL="${MASTER_URL:-http://localhost:8090}"
COMMIT_HASH="${COMMIT_HASH:-$(git rev-parse HEAD 2>/dev/null || echo local)}"
DIST_DIR="dist"

echo "=== 1. 编译 .so ==="
mkdir -p "$DIST_DIR"
for d in scripts/*/; do
    [ -f "$d/main.go" ] || continue
    name=$(basename "$d")
    # 跳过下划线前缀目录
    [[ "$name" == _* ]] && continue
    echo "  ==> building $name"
    go build -tags plugin -buildmode=plugin -o "$DIST_DIR/${name}.so" "./$d"
done
echo "✓ 编译完成: $DIST_DIR/"

# 只编译不上传（本地调试用）
if [ "$SKIP_UPLOAD" = "1" ]; then
    echo ""
    echo "=== 跳过上传（SKIP_UPLOAD=1）==="
    echo "本地 .so 路径: $PROJECT_ROOT/$DIST_DIR/"
    echo "DB artifactUrl 可设为: $PROJECT_ROOT/$DIST_DIR/http_login.so"
    exit 0
fi

# 检查 COS 密钥
if [ -z "$COS_SECRET_ID" ] || [ -z "$COS_SECRET_KEY" ]; then
    echo "Error: COS_SECRET_ID 和 COS_SECRET_KEY 环境变量未设置"
    echo ""
    echo "获取方式: Polaris 控制台 → Development → jarvan4 → master.yaml → cos 段"
    echo "或者从腾讯云控制台获取 API 密钥"
    exit 1
fi

echo ""
echo "=== 2. 上传到 COS ==="
pip install coscmd -q 2>/dev/null
coscmd config -a "$COS_SECRET_ID" -s "$COS_SECRET_KEY" -b "$COS_BUCKET" -r "$COS_REGION"

COMMIT_MSG=$(git log -1 --pretty=format:'%s' 2>/dev/null || echo "local build")
AUTHOR=$(git log -1 --pretty=format:'%an' 2>/dev/null || echo "local")

for f in "$DIST_DIR"/*.so; do
    [ -f "$f" ] || continue
    name=$(basename "$f" .so)

    # 上传版本化文件名 + 固定 latest 文件名
    cos_key_versioned="scripts/${name}/${COMMIT_HASH}.so"
    cos_key_latest="scripts/${name}/${name}.so"

    echo "  ⬆️  $f → cos://${COS_BUCKET}/${cos_key_versioned}"
    coscmd upload "$f" "$cos_key_versioned"
    echo "  ⬆️  $f → cos://${COS_BUCKET}/${cos_key_latest} (latest)"
    coscmd upload "$f" "$cos_key_latest"

    # 通知 Master
    echo "  📡 通知 Master: name=$name commitHash=${COMMIT_HASH:0:12}"
    http_code=$(curl -s -o /tmp/publish_resp.json -w "%{http_code}" \
        -X POST "${MASTER_URL}/api/internal/scripts/publish" \
        -H "Content-Type: application/json" \
        -d "{
            \"name\": \"${name}\",
            \"commitHash\": \"${COMMIT_HASH}\",
            \"artifactUrl\": \"${cos_key_latest}\",
            \"commitMsg\": \"${COMMIT_MSG}\",
            \"author\": \"${AUTHOR}\",
            \"sourceRepo\": \"https://github.com/A0dongq1N/jarvan4-script\",
            \"sourcePath\": \"scripts/${name}/main.go\"
        }")

    if [ "$http_code" = "200" ]; then
        resp_code=$(python3 -c "import json; print(json.load(open('/tmp/publish_resp.json'))['code'])" 2>/dev/null || echo "?")
        if [ "$resp_code" = "0" ]; then
            echo "  ✓ Master 已更新脚本 $name"
        else
            echo "  ✗ Master 返回业务错误: $(cat /tmp/publish_resp.json)"
        fi
    else
        echo "  ✗ Master 通知失败，HTTP $http_code"
    fi
done

echo ""
echo "✓ 全部完成"
