#!/usr/bin/env bash
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

TOTAL_USERS=${1:-100}
CONCURRENCY=${2:-10}
DURATION=${3:-10}
UDS_SOCK="/tmp/idp-uds.sock"

echo -e "${BOLD}╔══════════════════════════════════════════════════════════════════╗${RESET}"
echo -e "${BOLD}║            IDP Service UDS 并发压力测试                          ║${RESET}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════════════╝${RESET}"
echo -e "  用户数量: ${CYAN}${TOTAL_USERS}${RESET}"
echo -e "  并发数:   ${CYAN}${CONCURRENCY}${RESET}"
echo -e "  持续时间: ${CYAN}${DURATION}s${RESET}\n"

if [ ! -S "$UDS_SOCK" ]; then
  echo -e "${RED}错误: UDS Socket 不存在 ($UDS_SOCK)${RESET}"; exit 1
fi

SERVER_PID=$(pgrep -f "idp-server" | head -1 || echo "")
if [ -z "$SERVER_PID" ]; then
  echo -e "${RED}错误: 无法获取服务器 PID${RESET}"; exit 1
fi
echo -e "${GREEN}✓${RESET} 服务器 PID=${SERVER_PID}\n"

MONITOR_FILE="/tmp/bench_uds_monitor_$$.txt"
RESULT_FILE="/tmp/bench_uds_result_$$.txt"
echo "timestamp,cpu_percent,mem_mb" > "$MONITOR_FILE"
> "$RESULT_FILE"

monitor_resources() {
  while kill -0 "$SERVER_PID" 2>/dev/null && [ -f "$MONITOR_FILE" ]; do
    stats=$(ps -p "$SERVER_PID" -o %cpu=,rss= 2>/dev/null || echo "0 0")
    read -r cpu mem_kb <<< "$stats"
    mem_mb=$((mem_kb / 1024))
    echo "$(date +%s),$cpu,$mem_mb" >> "$MONITOR_FILE"
    sleep 1
  done
}

cleanup() {
  kill $MONITOR_PID 2>/dev/null || true
  wait $MONITOR_PID 2>/dev/null || true
  rm -f "$RESULT_FILE"
}
trap cleanup EXIT

monitor_resources &
MONITOR_PID=$!

echo -e "${CYAN}${BOLD}▶ 准备测试用户${RESET}"

uds_req() {
  local action=$1
  local payload=$2
  echo "{\"action\":\"$action\",\"payload\":$payload}" | nc -U "$UDS_SOCK" 2>/dev/null | head -1
}

# 使用 admin 创建测试用户（通过 REST API）
TOKEN=$(curl -s -X POST "http://localhost:8080/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"Admin@123456"}' | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

for i in $(seq 1 "$TOTAL_USERS"); do
  curl -s -X POST "http://localhost:8080/api/v1/users" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"udsuser$i\",\"password\":\"Pass@123\",\"email\":\"udsuser$i@test.com\"}" >/dev/null 2>&1 || true
done
echo -e "  创建完成\n"

echo -e "${CYAN}${BOLD}▶ 开始 UDS 并发压测${RESET}"
START=$(date +%s)
END_TIME=$((START + DURATION))

auth_user() {
  local i=$1
  local start=$(date +%s%3N)

  # 握手 + 认证，取第二行（认证响应）
  local resp=$(
    (
      echo '{"version":"1.0","format":"json"}'
      sleep 0.05
      echo "{\"action\":\"auth\",\"payload\":{\"username\":\"udsuser$i\",\"password\":\"Pass@123\"}}"
    ) | timeout 2 nc -U "$UDS_SOCK" 2>/dev/null | sed -n '2p'
  )

  local status=$(echo "$resp" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
  local end=$(date +%s%3N)
  local elapsed=$((end - start))
  echo "$status,$elapsed"
}
export -f auth_user
export UDS_SOCK

counter=0
while [ "$(date +%s)" -lt "$END_TIME" ]; do
  for i in $(seq 1 "$CONCURRENCY"); do
    if [ "$(date +%s)" -ge "$END_TIME" ]; then
      break
    fi
    user_id=$((counter % TOTAL_USERS + 1))
    echo "$user_id"
    counter=$((counter + 1))
  done
done | xargs -P "$CONCURRENCY" -I {} bash -c 'auth_user {}' >> "$RESULT_FILE"

END=$(date +%s)
ACTUAL_DURATION=$((END - START))
TOTAL_REQUESTS=$(wc -l < "$RESULT_FILE")

echo -e "\n${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
echo -e "${BOLD}                        请求统计${RESET}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"

success=$(grep -c "^success," "$RESULT_FILE" 2>/dev/null | tr -d '\n' || echo "0")
total_lines=$(wc -l < "$RESULT_FILE" 2>/dev/null | tr -d ' \n' || echo "0")
failed=$((total_lines - success))

if [ "$total_lines" -eq 0 ]; then
  echo -e "  ${RED}错误: 无有效请求结果${RESET}"
  exit 1
fi

success_rate=$(awk "BEGIN {printf \"%.2f\", ($success * 100.0) / $total_lines}")
qps=$(awk "BEGIN {printf \"%.2f\", $total_lines / $ACTUAL_DURATION}")
avg_latency=$(awk -F, 'NF==2 && $2~/^[0-9]+$/ {sum+=$2; count++} END {if(count>0) printf "%.2f", sum/count; else print "0"}' "$RESULT_FILE")

echo -e "  总请求数:   ${BOLD}${total_lines}${RESET}"
echo -e "  成功:       ${GREEN}${BOLD}${success}${RESET}"
echo -e "  失败:       ${RED}${BOLD}${failed}${RESET}"
echo -e "  成功率:     ${BOLD}${success_rate}%${RESET}"
echo -e "  耗时:       ${BOLD}${ACTUAL_DURATION}s${RESET}"
echo -e "  实际 QPS:   ${BOLD}${qps}${RESET}"
echo -e "  平均延迟:   ${BOLD}${avg_latency}ms${RESET}"

echo -e "\n${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
echo -e "${BOLD}                        资源消耗${RESET}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"

cpu_avg=$(awk -F, 'NR>1 {sum+=$2; count++} END {printf "%.2f", sum/count}' "$MONITOR_FILE")
cpu_max=$(awk -F, 'NR>1 {if($2>max) max=$2} END {printf "%.2f", max}' "$MONITOR_FILE")
mem_avg=$(awk -F, 'NR>1 {sum+=$3; count++} END {printf "%.0f", sum/count}' "$MONITOR_FILE")
mem_max=$(awk -F, 'NR>1 {if($3>max) max=$3} END {printf "%.0f", max}' "$MONITOR_FILE")

echo -e "  CPU 平均:   ${BOLD}${cpu_avg}%${RESET}"
echo -e "  CPU 峰值:   ${BOLD}${cpu_max}%${RESET}"
echo -e "  内存平均:   ${BOLD}${mem_avg} MB${RESET}"
echo -e "  内存峰值:   ${BOLD}${mem_max} MB${RESET}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"

[ "$failed" -eq 0 ] && echo -e "\n${GREEN}${BOLD}✓ 压测完成${RESET}" || echo -e "\n${CYAN}压测完成${RESET}"
