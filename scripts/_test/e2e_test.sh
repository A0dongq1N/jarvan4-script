#!/bin/bash
# 本地 e2e:启动 _target + 编译脚本 + 跑 cmd/runner + 校验指标
# 用法:bash scripts/_test/e2e_test.sh [script_name]
#   script_name:scripts/*/ 下的目录名,默认 http_demo
#
# 前置:
#   - Go 1.25+
#   - cd 到 jarvan4-script/ 目录
#   - jarvan4-platform 已在 ../jarvan4-platform/

set -e

SCRIPT_NAME="${1:-http_demo}"
TARGET_ADDR="${TARGET_ADDR:-:8888}"
BASE_URL="${BASE_URL:-http://localhost:8888}"
VU_COUNT=10
DURATION=5s
# e2e_test.sh 路径: jarvan4-script/scripts/_test/e2e_test.sh
# SCRIPT_DIR 应为 jarvan4-script/ 根目录
SCRIPT_DIR="$(cd "$(dirname "$(readlink -f "$0")")/../.." && pwd)"
PLATFORM_DIR="$(cd "$SCRIPT_DIR/../jarvan4-platform" && pwd)"
SO_FILE="/tmp/e2e_${SCRIPT_NAME}.so"
TARGET_PID=""

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

cleanup() {
    if [ -n "$TARGET_PID" ] && kill -0 "$TARGET_PID" 2>/dev/null; then
        echo -e "${YELLOW}>> 停止 _target (pid=$TARGET_PID)${NC}"
        kill "$TARGET_PID" 2>/dev/null || true
        wait "$TARGET_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT

echo -e "${YELLOW}============================================${NC}"
echo -e "${YELLOW}> e2e test: $SCRIPT_NAME vs _target${NC}"
echo -e "${YELLOW}============================================${NC}"
echo

# 1. 编译脚本
echo -e "${YELLOW}>>> [1/5] 编译脚本 $SCRIPT_NAME${NC}"
if [ ! -d "$SCRIPT_DIR/scripts/$SCRIPT_NAME" ]; then
    echo -e "${RED}✗ 脚本目录不存在: $SCRIPT_DIR/scripts/$SCRIPT_NAME${NC}"
    exit 1
fi
cd "$SCRIPT_DIR"
go build -tags plugin -buildmode=plugin -o "$SO_FILE" "./scripts/$SCRIPT_NAME"
echo -e "${GREEN}✓ 编译成功: $SO_FILE${NC}"
echo

# 2. 启 _target
echo -e "${YELLOW}>>> [2/5] 启动 _target (端口 $TARGET_ADDR)${NC}"
cd "$SCRIPT_DIR/scripts/_target"
TARGET_ADDR="$TARGET_ADDR" go run ./main.go &
TARGET_PID=$!
echo "  pid=$TARGET_PID"
# 等服务就绪
for i in $(seq 1 20); do
    if curl -s -m 1 "http://localhost${TARGET_ADDR}/__stats" > /dev/null 2>&1; then
        echo -e "${GREEN}✓ _target 已就绪${NC}"
        break
    fi
    if [ $i -eq 20 ]; then
        echo -e "${RED}✗ _target 启动超时${NC}"
        exit 1
    fi
    sleep 0.5
done
echo

# 3. 跑 cmd/runner
echo -e "${YELLOW}>>> [3/5] 跑压测: VU=$VU_COUNT, duration=$DURATION${NC}"
cd "$PLATFORM_DIR"
go run ./cmd/runner \
    -so "$SO_FILE" \
    -vu $VU_COUNT \
    -duration "$DURATION" \
    -env "BASE_URL=$BASE_URL" 2>&1 | tee /tmp/e2e_runner.log
echo

# 4. 校验
echo -e "${YELLOW}>>> [4/5] 校验指标${NC}"
RUNNER_LOG=/tmp/e2e_runner.log
# cmd/runner 输出中文:"总请求数: 65030";如果改了输出格式这里要同步
if ! grep -qE "总请求数[:：]\s*[1-9]|TotalReqs[:=]\s*[1-9]" "$RUNNER_LOG"; then
    echo -e "${RED}✗ 压测未产生任何请求${NC}"
    tail -20 "$RUNNER_LOG"
    exit 1
fi
TOTAL=$(grep -oE "总请求数[:：]\s*[0-9]+|TotalReqs[:=]\s*[0-9]+" "$RUNNER_LOG" | head -1 | grep -oE "[0-9]+")
FAILS=$(grep -oE "失败数[:：]\s*[0-9]+|TotalFails[:=]\s*[0-9]+" "$RUNNER_LOG" | head -1 | grep -oE "[0-9]+")
QPS=$(grep -oE "平均 QPS[:：]\s*[0-9.]+|QPS[:=]\s*[0-9.]+" "$RUNNER_LOG" | head -1 | grep -oE "[0-9.]+")

# 校验 _target 收到了请求
STATS=$(curl -s "http://localhost${TARGET_ADDR}/__stats")
SUCCESS=$(echo "$STATS" | python3 -c "import json,sys; print(json.load(sys.stdin).get('success', 0))" 2>/dev/null || echo 0)
echo "  压测统计: TotalReqs=$TOTAL TotalFails=$FAILS QPS=$QPS"
echo "  _target 统计: $STATS"

if [ "$SUCCESS" -eq 0 ]; then
    echo -e "${RED}✗ _target 未收到任何请求${NC}"
    exit 1
fi

# 允许少量 fail(http_demo 是简单 GET,不应该 fail)
if [ "${FAILS:-0}" -gt 0 ] && [ "$SCRIPT_NAME" = "http_demo" ]; then
    echo -e "${YELLOW}⚠ http_demo 出现 $FAILS 个 fail,请检查${NC}"
fi

echo -e "${GREEN}✓ 压测通过: _target 收到 $SUCCESS 个成功请求${NC}"
echo

# 5. 总结
echo -e "${YELLOW}============================================${NC}"
echo -e "${GREEN}✓ e2e PASSED: $SCRIPT_NAME${NC}"
echo -e "${YELLOW}============================================${NC}"
echo "  压测请求:   $TOTAL"
echo "  失败数:     $FAILS"
echo "  QPS:        $QPS"
echo "  Target 收到: $SUCCESS"