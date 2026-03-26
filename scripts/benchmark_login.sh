#!/bin/bash

# UDS 登录压测脚本
# 用法: ./benchmark_login.sh <用户数> [并发数] [创建用户并发数]

USER_COUNT=${1:-1000}
CONCURRENCY=${2:-100}
CREATE_CONCURRENCY=${3:-50}
UDS_PATH="/tmp/idp-uds.sock"
RESULTS_DIR="./benchmark_results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p "$RESULTS_DIR"

# 清理函数
cleanup() {
    [ ! -z "$MONITOR_PID" ] && kill $MONITOR_PID 2>/dev/null
    exit
}
trap cleanup EXIT INT TERM

echo "========================================="
echo "UDS 登录压测"
echo "用户数: $USER_COUNT"
echo "登录并发数: $CONCURRENCY"
echo "创建用户并发数: $CREATE_CONCURRENCY"
echo "========================================="
echo "环境信息:"
echo "CPU: $(grep -c ^processor /proc/cpuinfo) 核心"
echo "内存: $(free -h | awk '/^Mem:/ {print $2}')"
echo "系统: $(uname -s) $(uname -r)"
echo "========================================="

# 获取 IDP 服务进程 PID
IDP_PID=$(ps aux | grep -E "idp-server|bin/idp-server" | grep -v grep | awk '{print $2}' | head -1)
if [ -z "$IDP_PID" ]; then
    echo "错误: 未找到 IDP 服务进程"
    exit 1
fi
echo "IDP 服务 PID: $IDP_PID"

# 获取 admin token
echo "获取管理员 token..."
TOKEN=$(./idpctl auth login admin Admin@123456 2>/dev/null | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
if [ -z "$TOKEN" ]; then
    echo "错误: 无法获取 token"
    exit 1
fi

# 监控资源使用
monitor_resources() {
    local output_file="$1"
    echo "时间,CPU%,内存MB,虚拟内存MB" > "$output_file"
    while kill -0 $IDP_PID 2>/dev/null; do
        ps -p $IDP_PID -o %cpu,rss,vsz | tail -1 | awk -v ts="$(date +%H:%M:%S)" '{printf "%s,%.2f,%.2f,%.2f\n", ts, $1, $2/1024, $3/1024}' >> "$output_file"
        sleep 0.5
    done
}

# 启动资源监控
RESOURCE_FILE="$RESULTS_DIR/resources_${TIMESTAMP}.csv"
monitor_resources "$RESOURCE_FILE" &
MONITOR_PID=$!

# 创建测试用户（如果不存在）
echo "准备测试用户..."
cat > /tmp/create_users.sh << SHEOF
#!/bin/bash
USER_ID=\$1
curl -s -X POST http://localhost:8080/api/v1/users \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"user\$USER_ID\",\"password\":\"Pass@\$USER_ID\",\"email\":\"user\$USER_ID@test.com\"}" \
    >/dev/null 2>&1
SHEOF
chmod +x /tmp/create_users.sh

seq 1 $USER_COUNT | xargs -P $CREATE_CONCURRENCY -I {} /tmp/create_users.sh {}
echo "用户创建完成"

# Python 压测脚本
cat > /tmp/uds_login_bench.py << 'PYEOF'
import socket
import json
import time
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed

def uds_login(user_id):
    try:
        s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        s.connect("/tmp/idp-uds.sock")
        s.settimeout(10)
        
        # 握手
        s.sendall(json.dumps({"version": "1.0", "format": "json"}).encode() + b'\n')
        s.recv(1024)
        
        # 登录
        username = f"user{user_id}"
        password = f"Pass@{user_id}"
        req = {"action": "auth", "request_id": f"req-{user_id}", 
               "payload": {"username": username, "password": password}}
        s.sendall(json.dumps(req).encode() + b'\n')
        s.recv(1024)
        s.close()
        return True
    except Exception as e:
        return False

user_count = int(sys.argv[1])
concurrency = int(sys.argv[2])
start = time.time()

with ThreadPoolExecutor(max_workers=concurrency) as executor:
    futures = [executor.submit(uds_login, i) for i in range(1, user_count + 1)]
    success = sum(1 for f in as_completed(futures) if f.result())

elapsed = time.time() - start
print(f"{user_count},{success},{elapsed:.2f},{success/elapsed:.2f}")
PYEOF

echo "开始压测..."
echo "用户数,成功数,耗时(秒),QPS" > "$RESULTS_DIR/login_${TIMESTAMP}.csv"
python3 /tmp/uds_login_bench.py $USER_COUNT $CONCURRENCY >> "$RESULTS_DIR/login_${TIMESTAMP}.csv"

# 停止监控
kill $MONITOR_PID 2>/dev/null

# 输出结果
echo ""
echo "========================================="
echo "压测完成"
cat "$RESULTS_DIR/login_${TIMESTAMP}.csv"
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
