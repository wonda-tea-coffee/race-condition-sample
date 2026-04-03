#!/bin/bash
set -uo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
CONCURRENT=20
PRODUCT_ID=1
STOCK=5

# 色付き出力
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

run_test() {
  local strategy=$1
  local label=$2

  echo ""
  echo "==========================================="
  echo -e " ${YELLOW}${label}${NC}"
  echo "==========================================="

  # リセット
  curl -s -X POST "${BASE_URL}/reset" -H "Content-Type: application/json" -d "{\"stock\": ${STOCK}}" > /dev/null

  # 現在の在庫を確認
  initial_stock=$(curl -s "${BASE_URL}/products/${PRODUCT_ID}" | jq '.stock')
  echo "初期在庫: ${initial_stock}"

  # 並行リクエスト送信
  echo "${CONCURRENT} 件の並行購入リクエストを送信中..."
  pids=()
  results_dir=$(mktemp -d)

  for i in $(seq 1 $CONCURRENT); do
    curl -s -o "${results_dir}/${i}.json" -w "%{http_code} %{time_total}" \
      -X POST "${BASE_URL}/purchase/${strategy}" \
      -H "Content-Type: application/json" \
      -d "{\"product_id\": ${PRODUCT_ID}, \"quantity\": 1}" \
      > "${results_dir}/${i}.result" &
    pids+=($!)
  done

  # 全リクエスト完了を待つ
  for pid in "${pids[@]}"; do
    wait "$pid" 2>/dev/null || true
  done

  # 結果集計
  success=0
  conflict=0
  error=0
  times=()
  for i in $(seq 1 $CONCURRENT); do
    result=$(cat "${results_dir}/${i}.result" 2>/dev/null || echo "000 0")
    status=$(echo "$result" | awk '{print $1}')
    time_sec=$(echo "$result" | awk '{print $2}')
    times+=("$time_sec")
    case "$status" in
      200) success=$((success + 1)) ;;
      409) conflict=$((conflict + 1)) ;;
      *)   error=$((error + 1)) ;;
    esac
  done

  # レスポンスタイム集計
  time_stats=$(printf '%s\n' "${times[@]}" | awk '
    BEGIN { min=999; max=0; sum=0; n=0 }
    { n++; sum+=$1; if($1<min) min=$1; if($1>max) max=$1 }
    END { printf "%.3f %.3f %.3f", min, max, sum/n }
  ')
  time_min=$(echo "$time_stats" | awk '{print $1}')
  time_max=$(echo "$time_stats" | awk '{print $2}')
  time_avg=$(echo "$time_stats" | awk '{print $3}')

  # 最終状態を確認
  final_stock=$(curl -s "${BASE_URL}/products/${PRODUCT_ID}" | jq '.stock')
  order_count=$(curl -s "${BASE_URL}/orders" | jq 'length')
  payment_count=$(curl -s "${BASE_URL}/mock/payments" | jq '.count')

  echo ""
  echo "--- 結果 ---"
  echo "成功レスポンス (200): ${success}"
  echo "在庫切れ (409):       ${conflict}"
  echo "エラー:               ${error}"
  echo "最終在庫:             ${final_stock}"
  echo "注文数:               ${order_count}"
  echo "決済成功数:           ${payment_count}"
  echo ""
  echo "--- レスポンスタイム ---"
  echo "最小: ${time_min}s  最大: ${time_max}s  平均: ${time_avg}s"
  echo ""

  # 整合性チェック
  local ok=true

  # 在庫の整合性: 在庫 = 初期在庫 - 注文数
  expected_stock=$((initial_stock - order_count))
  if [ "$final_stock" -ne "$expected_stock" ]; then
    echo -e "${RED}NG: 在庫の不整合${NC} (在庫=${final_stock}, 期待=${expected_stock})"
    ok=false
  fi

  # 在庫がマイナス
  if [ "$final_stock" -lt 0 ]; then
    echo -e "${RED}NG: 在庫がマイナス${NC} (${final_stock})"
    ok=false
  fi

  # 決済数と注文数の整合性
  if [ "$payment_count" -ne "$order_count" ]; then
    echo -e "${RED}NG: 決済と注文の不整合${NC} (決済=${payment_count}, 注文=${order_count})"
    ok=false
  fi

  if [ "$ok" = true ]; then
    echo -e "判定: ${GREEN}OK${NC} — 在庫・注文・決済が全て整合"
  else
    echo -e "判定: ${RED}NG${NC} — 上記の不整合あり"
  fi

  rm -rf "$results_dir"
}

# 戦略名→ラベルのマッピング
declare -A LABELS=(
  [none]="ロックなし (レースコンディション発生)"
  [pessimistic]="悲観的ロック (SELECT FOR UPDATE)"
  [optimistic]="楽観的ロック (version)"
  [lock]="ネームドロック (GET_LOCK)"
)
ALL_STRATEGIES=(none pessimistic optimistic lock)

echo "========================================="
echo " 排他制御 検証スクリプト"
echo "========================================="

if [ $# -gt 0 ]; then
  for s in "$@"; do
    if [ -z "${LABELS[$s]+x}" ]; then
      echo "不明な戦略: $s (選択肢: ${ALL_STRATEGIES[*]})"
      exit 1
    fi
    run_test "$s" "${LABELS[$s]}"
  done
else
  for s in "${ALL_STRATEGIES[@]}"; do
    run_test "$s" "${LABELS[$s]}"
  done
fi
