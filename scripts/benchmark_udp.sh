#!/bin/bash

# UDP 认证压测脚本
# 用法: ./benchmark_udp.sh [用户数量] [并发数]

USER_COUNT=${1:-100}
CONCURRENCY=${2:-50}
RESULTS_DIR="./benchmark_results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p "$RESULTS_DIR"

echo "========================================="
echo "UDP 认证压测"
echo "用户数量: $USER_COUNT"
echo "并发数: $CONCURRENCY"
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

RESOURCE_FILE="$RESULTS_DIR/udp_resources_${TIMESTAMP}.csv"
monitor_resources "$RESOURCE_FILE" &
MONITOR_PID=$!

cleanup() {
    [ ! -z "$MONITOR_PID" ] && kill $MONITOR_PID 2>/dev/null
}
trap cleanup EXIT INT TERM

# 获取 admin token
echo "获取管理员 token..."
ADMIN_TOKEN=$(./idpctl auth login admin Admin@123456 2>/dev/null | jq -r '.token' 2>/dev/null)
if [ -z "$ADMIN_TOKEN" ] || [ "$ADMIN_TOKEN" = "null" ]; then
    echo "错误: 无法获取管理员 token"
    exit 1
fi

# 创建用户
echo "创建测试用户..."

cat > /tmp/create_user.sh << 'SHEOF'
#!/bin/bash
USER_ID=$1
ADMIN_TOKEN=$2
RESULT=$(curl -s -X POST http://localhost:8080/api/v1/users \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"udpuser$USER_ID\",\"password\":\"Pass@123$USER_ID\",\"email\":\"udpuser$USER_ID@test.com\"}")
if echo "$RESULT" | grep -q "id"; then
    echo "OK"
fi
SHEOF
chmod +x /tmp/create_user.sh

SUCCESS_COUNT=$(seq 1 $USER_COUNT | xargs -P 100 -I {} /tmp/create_user.sh {} "$ADMIN_TOKEN" | grep -c "OK")
echo "用户创建完成: $SUCCESS_COUNT/$USER_COUNT"
sleep 1

# Python 脚本并发测试 UDP 认证
cat > /tmp/test_udp.py << 'PYEOF'
import socket
import sys
import time
from threading import Semaphore, Thread

def test_udp_auth(user_id, sem):
    try:
        sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        sock.settimeout(3)
        msg = f'http://192.168.5.1/webAuth/index.htm -d username=udpuser{user_id}&pass_word=Pass@123{user_id}'
        sock.sendto(msg.encode(), ('localhost', 8888))
        sock.recvfrom(1024)
        sock.close()
    except:
        pass
    finally:
        sem.release()

user_count = int(sys.argv[1])
concurrency = int(sys.argv[2])

sem = Semaphore(concurrency)
threads = []

start = time.time()
for i in range(user_count):
    sem.acquire()
    t = Thread(target=test_udp_auth, args=(i + 1, sem))
    t.start()
    threads.append(t)

for t in threads:
    t.join()

elapsed = time.time() - start
print(f"{user_count},{elapsed:.2f}")
PYEOF

echo "开始压测 UDP 认证..."
LOG_START_LINE=$(wc -l < /tmp/server.log 2>/dev/null || echo 0)
python3 /tmp/test_udp.py $USER_COUNT $CONCURRENCY > /tmp/bench_result.txt
RESULT=$(cat /tmp/bench_result.txt)
TOTAL=$(echo $RESULT | cut -d',' -f1)
ELAPSED=$(echo $RESULT | cut -d',' -f2)

# 等待服务器日志中的认证成功数与请求数一致
echo "等待服务端处理完成..."
SERVER_SUCCESS=0
while [ $SERVER_SUCCESS -lt $USER_COUNT ]; do
    SERVER_SUCCESS=$(tail -n +$((LOG_START_LINE + 1)) /tmp/server.log 2>/dev/null | grep -c "Auth success" || echo 0)
    sleep 1
done

SERVER_SUCCESS=$(tail -n +$((LOG_START_LINE + 1)) /tmp/server.log 2>/dev/null | grep -c "Auth success" || echo 0)

echo "$TOTAL,$ELAPSED,$SERVER_SUCCESS" > "$RESULTS_DIR/udp_${TIMESTAMP}.csv"

echo ""
echo "========================================="
echo "压测完成"
echo "请求数: $TOTAL, 耗时: ${ELAPSED}s"
echo "服务端认证成功: $SERVER_SUCCESS"
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
