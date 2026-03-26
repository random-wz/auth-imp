#!/usr/bin/env bash
set -euo pipefail

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

# 默认参数
USERS=${1:-100}           # 用户数量
RATE=${2:-10}             # 每秒请求数
DURATION=${3:-30}         # 持续时间(秒)
API_URL="http://localhost:8080"
MONITOR_INTERVAL=1        # 监控采样间隔(秒)

# 临时文件
RESULTS_FILE="/tmp/bench_results_$$.txt"
MONITOR_FILE="/tmp/bench_monitor_$$.txt"
SERVER_PID_FILE="/tmp/bench_server_pid_$$.txt"

cleanup() {
  rm -f "$RESULTS_FILE" "$MONITOR_FILE" "$SERVER_PID_FILE"
  pkill -P $$ 2>/dev/null || true
}
trap cleanup EXIT

echo -e "${BOLD}╔══════════════════════════════════════════════════════════════════╗${RESET}"
echo -e "${BOLD}║            IDP Service 压力测试工具                             ║${RESET}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════════════╝${RESET}"
echo -e "  用户数量: ${CYAN}${USERS}${RESET}"
echo -e "  请求速率: ${CYAN}${RATE}/s${RESET}"
echo -e "  持续时间: ${CYAN}${DURATION}s${RESET}"
echo -e "  API 地址: ${CYAN}${API_URL}${RESET}\n"

# 查找服务器进程
if ! curl -sf "$API_URL/health" >/dev/null 2>&1; then
  echo -e "${RED}错误: 服务器未运行 ($API_URL)${RESET}"
  exit 1
fi

SERVER_PID=$(pgrep -f "idp-server" | head -1 || echo "")
if [ -z "$SERVER_PID" ]; then
  echo -e "${RED}错误: 无法获取服务器 PID${RESET}"
  exit 1
fi

echo "$SERVER_PID" > "$SERVER_PID_FILE"
echo -e "${GREEN}✓${RESET} 找到服务器进程 PID=${SERVER_PID}\n"

# 性能监控
monitor_resources() {
  local pid=$1
  echo "timestamp,cpu_percent,mem_mb,mem_percent" > "$MONITOR_FILE"

  while kill -0 "$pid" 2>/dev/null; do
    local stats
    stats=$(ps -p "$pid" -o %cpu=,%mem=,rss= 2>/dev/null || echo "0 0 0")
    local cpu mem_pct mem_kb
    read -r cpu mem_pct mem_kb <<< "$stats"
    local mem_mb=$((mem_kb / 1024))
    local ts
    ts=$(date +%s)
    echo "$ts,$cpu,$mem_mb,$mem_pct" >> "$MONITOR_FILE"
    sleep "$MONITOR_INTERVAL"
  done
}

# 启动监控
echo -e "${CYAN}${BOLD}▶ 启动资源监控${RESET}"
monitor_resources "$SERVER_PID" &
MONITOR_PID=$!

# 压测函数
run_benchmark() {
  local requests_per_user=$((RATE * DURATION / USERS))
  if [ "$requests_per_user" -lt 1 ]; then
    requests_per_user=1
  fi
  local delay=$(awk "BEGIN {print 1.0 / $RATE}")

  echo -e "${CYAN}${BOLD}▶ 开始压测${RESET}"
  echo -e "  每用户请求数: ${requests_per_user}"
  echo -e "  请求间隔: ${delay}s\n"

  > "$RESULTS_FILE"

  for i in $(seq 1 "$USERS"); do
    (
      local username="user$i"
      local password="Pass$i"
      local success=0
      local failed=0
      local total_time=0

      for j in $(seq 1 "$requests_per_user"); do
        local start
        start=$(date +%s%3N)

        local resp
        resp=$(curl -s -w "\n%{http_code}" -X POST "$API_URL/api/v1/auth/login" \
          -H "Content-Type: application/json" \
          -d "{\"username\":\"admin\",\"password\":\"Admin@123456\"}" 2>/dev/null || echo -e "\n000")

        local status
        status=$(echo "$resp" | tail -1)
        local end
        end=$(date +%s%3N)
        local elapsed=$((end - start))

        if [ "$status" = "200" ]; then
          success=$((success + 1))
        else
          failed=$((failed + 1))
        fi

        total_time=$((total_time + elapsed))
        sleep "$delay" 2>/dev/null || true
      done

      echo "$success,$failed,$total_time" >> "$RESULTS_FILE"
    ) &

    # 控制并发启动速率
    if [ $((i % 10)) -eq 0 ]; then
      sleep 0.1
    fi
  done

  # 等待所有请求完成
  wait
}

# 执行压测
START_TIME=$(date +%s)
run_benchmark
END_TIME=$(date +%s)
ACTUAL_DURATION=$((END_TIME - START_TIME))

# 停止监控
kill "$MONITOR_PID" 2>/dev/null || true
wait "$MONITOR_PID" 2>/dev/null || true

# 统计结果
echo -e "\n${CYAN}${BOLD}▶ 统计结果${RESET}"

total_success=0
total_failed=0
total_time=0
count=0

while IFS=, read -r success failed time; do
  total_success=$((total_success + success))
  total_failed=$((total_failed + failed))
  total_time=$((total_time + time))
  count=$((count + 1))
done < "$RESULTS_FILE"

total_requests=$((total_success + total_failed))
success_rate=0
if [ "$total_requests" -gt 0 ]; then
  success_rate=$(awk "BEGIN {printf \"%.2f\", ($total_success * 100.0) / $total_requests}")
fi

avg_latency=0
if [ "$total_success" -gt 0 ]; then
  avg_latency=$(awk "BEGIN {printf \"%.2f\", $total_time / $total_success}")
fi

qps=0
if [ "$ACTUAL_DURATION" -gt 0 ]; then
  qps=$(awk "BEGIN {printf \"%.2f\", $total_requests / $ACTUAL_DURATION}")
fi

echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
echo -e "${BOLD}                        请求统计${RESET}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
echo -e "  总请求数:   ${BOLD}${total_requests}${RESET}"
echo -e "  成功:       ${GREEN}${BOLD}${total_success}${RESET}"
echo -e "  失败:       ${RED}${BOLD}${total_failed}${RESET}"
echo -e "  成功率:     ${BOLD}${success_rate}%${RESET}"
echo -e "  实际耗时:   ${BOLD}${ACTUAL_DURATION}s${RESET}"
echo -e "  实际 QPS:   ${BOLD}${qps}${RESET}"
echo -e "  平均延迟:   ${BOLD}${avg_latency}ms${RESET}"

# 资源统计
if [ -f "$MONITOR_FILE" ]; then
  echo -e "\n${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
  echo -e "${BOLD}                        资源消耗${RESET}"
  echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"

  local cpu_avg cpu_max mem_avg mem_max
  cpu_avg=$(awk -F, 'NR>1 {sum+=$2; count++} END {if(count>0) printf "%.2f", sum/count; else print "0"}' "$MONITOR_FILE")
  cpu_max=$(awk -F, 'NR>1 {if($2>max) max=$2} END {printf "%.2f", max+0}' "$MONITOR_FILE")
  mem_avg=$(awk -F, 'NR>1 {sum+=$3; count++} END {if(count>0) printf "%.0f", sum/count; else print "0"}' "$MONITOR_FILE")
  mem_max=$(awk -F, 'NR>1 {if($3>max) max=$3} END {printf "%.0f", max+0}' "$MONITOR_FILE")

  echo -e "  CPU 平均:   ${BOLD}${cpu_avg}%${RESET}"
  echo -e "  CPU 峰值:   ${BOLD}${cpu_max}%${RESET}"
  echo -e "  内存平均:   ${BOLD}${mem_avg} MB${RESET}"
  echo -e "  内存峰值:   ${BOLD}${mem_max} MB${RESET}"
fi

echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"

if [ "$total_failed" -eq 0 ]; then
  echo -e "\n${GREEN}${BOLD}✓ 压测完成，所有请求成功${RESET}"
else
  echo -e "\n${YELLOW}${BOLD}⚠ 压测完成，部分请求失败${RESET}"
fi
