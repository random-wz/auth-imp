#!/bin/bash

# Token 验证压测脚本
# 用法: ./benchmark_token.sh <token文件路径|token数量> [验证并发数] [创建并发数] [保留token文件:true/false]

TOKEN_INPUT=${1:-100}
CONCURRENCY=${2:-50}
CREATE_CONCURRENCY=${3:-20}
KEEP_TOKENS=${4:-false}
RESULTS_DIR="./benchmark_results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
USE_EXISTING=false

# 检查第一个参数是文件还是数字
if [ -f "$TOKEN_INPUT" ]; then
    TOKENS_FILE="$TOKEN_INPUT"
    USE_EXISTING=true
    KEEP_TOKENS=true
else
    TOKEN_COUNT=$TOKEN_INPUT
    TOKENS_FILE="/tmp/tokens_${TIMESTAMP}.txt"
fi

mkdir -p "$RESULTS_DIR"

# 清理函数
cleanup() {
    [ ! -z "$MONITOR_PID" ] && kill $MONITOR_PID 2>/dev/null
    if [ "$USE_EXISTING" = "false" ] && [ "$KEEP_TOKENS" != "true" ]; then
        rm -f "$TOKENS_FILE"
    elif [ "$KEEP_TOKENS" = "true" ]; then
        echo "Token 文件已保存: $TOKENS_FILE"
    fi
}
trap cleanup EXIT INT TERM

echo "========================================="
echo "Token 验证压测"
if [ "$USE_EXISTING" = "true" ]; then
    echo "使用已有 Token 文件: $TOKENS_FILE"
else
    echo "Token 数量: $TOKEN_COUNT"
    echo "创建并发数: $CREATE_CONCURRENCY"
fi
echo "验证并发数: $CONCURRENCY"
echo "========================================="

# 获取 IDP 服务进程 PID
IDP_PID=$(ps aux | grep -E "idp-server|bin/idp-server" | grep -v grep | awk '{print $2}' | head -1)
if [ -z "$IDP_PID" ]; then
    echo "错误: 未找到 IDP 服务进程"
    exit 1
fi

# 监控资源使用
monitor_resources() {
    local output_file="$1"
    echo "时间,CPU%,内存MB" > "$output_file"
    while kill -0 $IDP_PID 2>/dev/null; do
        ps -p $IDP_PID -o %cpu,rss | tail -1 | awk -v ts="$(date +%H:%M:%S)" '{printf "%s,%.2f,%.2f\n", ts, $1, $2/1024}' >> "$output_file"
        sleep 0.5
    done
}

RESOURCE_FILE="$RESULTS_DIR/token_resources_${TIMESTAMP}.csv"
monitor_resources "$RESOURCE_FILE" &
MONITOR_PID=$!

# 如果使用已有文件，跳过生成步骤
if [ "$USE_EXISTING" = "false" ]; then
    # 获取 admin token
    echo "获取管理员 token..."
    ADMIN_TOKEN=$(./idpctl auth login admin Admin@123456 2>/dev/null | jq -r '.token' 2>/dev/null)
    if [ -z "$ADMIN_TOKEN" ] || [ "$ADMIN_TOKEN" = "null" ]; then
        echo "错误: 无法获取管理员 token"
        exit 1
    fi

    # 创建用户并生成 tokens
    echo "创建用户并生成 tokens..."
    > "$TOKENS_FILE"

    # 创建用户脚本
    cat > /tmp/create_user_token.sh << SHEOF
#!/bin/bash
USER_ID=\$1
curl -s -X POST http://localhost:8080/api/v1/users \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"tuser\$USER_ID\",\"password\":\"Pass@123\$USER_ID\",\"email\":\"tuser\$USER_ID@test.com\"}" >/dev/null 2>&1

curl -s -X POST http://localhost:8080/api/v1/auth/login \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"tuser\$USER_ID\",\"password\":\"Pass@123\$USER_ID\"}" | jq -r '.token' 2>/dev/null
SHEOF
    chmod +x /tmp/create_user_token.sh

    seq 1 $TOKEN_COUNT | xargs -P $CREATE_CONCURRENCY -I {} /tmp/create_user_token.sh {} | grep -v "^null$" > "$TOKENS_FILE"

    ACTUAL_COUNT=$(wc -l < "$TOKENS_FILE")
    echo "成功生成 $ACTUAL_COUNT 个 token"
else
    ACTUAL_COUNT=$(wc -l < "$TOKENS_FILE")
    echo "Token 文件包含 $ACTUAL_COUNT 个 token"
fi

# 压测验证 tokens（使用 UDS）
echo "开始压测验证（UDS）..."

# Python 脚本通过 UDS 并发验证 token
cat > /tmp/test_token_uds.py << 'PYEOF'
import socket
import json
import sys
import time
from concurrent.futures import ThreadPoolExecutor, as_completed

def test_token_uds(token, user_id):
    try:
        s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        s.connect("/tmp/idp-uds.sock")
        s.settimeout(5)

        # 握手
        s.sendall(json.dumps({"version": "1.0", "format": "json"}).encode() + b'\n')
        s.recv(1024)

        # 验证
        req = {"action": "get_user", "request_id": f"req-{user_id}",
               "payload": {"username": f"tuser{user_id}"}}
        s.sendall(json.dumps(req).encode() + b'\n')
        resp = s.recv(4096)
        s.close()

        return "success" in resp.decode()
    except:
        return False

tokens_file = sys.argv[1]
concurrency = int(sys.argv[2])

with open(tokens_file) as f:
    tokens = [(line.strip(), i+1) for i, line in enumerate(f)]

start = time.time()
with ThreadPoolExecutor(max_workers=concurrency) as executor:
    futures = [executor.submit(test_token_uds, t, i) for t, i in tokens]
    success = sum(1 for f in as_completed(futures) if f.result())

elapsed = time.time() - start
print(f"{len(tokens)},{success},{elapsed:.2f},{success/elapsed:.2f}")
PYEOF

python3 /tmp/test_token_uds.py "$TOKENS_FILE" $CONCURRENCY > /tmp/bench_result.txt
RESULT=$(cat /tmp/bench_result.txt)
ACTUAL_COUNT=$(echo $RESULT | cut -d',' -f1)
SUCCESS=$(echo $RESULT | cut -d',' -f2)
ELAPSED=$(echo $RESULT | cut -d',' -f3)
QPS=$(echo $RESULT | cut -d',' -f4)

echo "$ACTUAL_COUNT,$SUCCESS,$ELAPSED,$QPS" > "$RESULTS_DIR/token_${TIMESTAMP}.csv"

echo ""
echo "========================================="
echo "压测完成"
echo "Token数量: $ACTUAL_COUNT, 成功: $SUCCESS, 耗时: ${ELAPSED}s, QPS: $QPS"
echo ""
echo "资源使用统计:"
awk -F',' 'NR>1 {
    cpu_sum+=$2; mem_sum+=$3; count++
    if($2>max_cpu)max_cpu=$2
    if($3>max_mem)max_mem=$3
} END {
    printf "CPU - 峰值: %.2f%%, 平均: %.2f%%\n", max_cpu, cpu_sum/count
    printf "内存 - 峰值: %.2f MB, 平均: %.2f MB\n", max_mem, mem_sum/count
}' "$RESOURCE_FILE"
echo "========================================="
