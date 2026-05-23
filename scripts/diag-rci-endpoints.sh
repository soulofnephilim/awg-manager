#!/bin/sh
# diag-rci-endpoints.sh — сравнить ответы NDMS RCI для всех endpoint'ов,
# которые читает awg-manager, двумя путями:
#   1) Direct GET  http://localhost:79/rci<path>
#   2) Batch POST  http://localhost:79/rci/  body: [{"show":{...:{}}}]
#
# Запуск на роутере: sh /opt/tmp/diag-rci-endpoints.sh [out_file]
# По умолчанию out_file=/opt/tmp/rci-diag.<pid>.log
# Всё в одном файле: таблица + полные GET/POST/diff каждого endpoint'а.

set -u

# /rci/ с trailing slash обязателен для POST — без него NDMS отвечает 405.
RCI_BASE=http://localhost:79/rci
RCI_POST=http://localhost:79/rci/
OUT="${1:-/opt/tmp/rci-diag.$$.log}"
: >"$OUT" || { echo "не могу писать в $OUT"; exit 1; }

# Список endpoint'ов: "GET_path|json_payload_for_POST"
# {} вместо null для leaf — важно, см. PR в batcher.
ENDPOINTS='
/show/version|[{"show":{"version":{}}}]
/show/system|[{"show":{"system":{}}}]
/show/interface/|[{"show":{"interface":{}}}]
/show/ip/hotspot|[{"show":{"ip":{"hotspot":{}}}}]
/show/ip/policy|[{"show":{"ip":{"policy":{}}}}]
/show/ip/route|[{"show":{"ip":{"route":{}}}}]
/show/rc/ip/host|[{"show":{"rc":{"ip":{"host":{}}}}}]
/show/rc/ip/policy|[{"show":{"rc":{"ip":{"policy":{}}}}}]
/show/rc/dns-proxy|[{"show":{"rc":{"dns-proxy":{}}}}]
/show/sc/dns-proxy/route|[{"show":{"sc":{"dns-proxy":{"route":{}}}}}]
/show/rc/object-group/fqdn|[{"show":{"rc":{"object-group":{"fqdn":{}}}}}]
/show/running-config|[{"show":{"running-config":{}}}]
'

# Список keys для unwrap batch-ответа в [{"show":{...}}] оболочке
unwrap_keys() {
    case "$1" in
        /show/version) echo "show version" ;;
        /show/system) echo "show system" ;;
        /show/interface/) echo "show interface" ;;
        /show/ip/hotspot) echo "show ip hotspot" ;;
        /show/ip/policy) echo "show ip policy" ;;
        /show/ip/route) echo "show ip route" ;;
        /show/rc/ip/host) echo "show rc ip host" ;;
        /show/rc/ip/policy) echo "show rc ip policy" ;;
        /show/rc/dns-proxy) echo "show rc dns-proxy" ;;
        /show/sc/dns-proxy/route) echo "show sc dns-proxy route" ;;
        /show/rc/object-group/fqdn) echo "show rc object-group fqdn" ;;
        /show/running-config) echo "show running-config" ;;
        *) echo "" ;;
    esac
}

# Временные файлы для одного endpoint'а (перезаписываются на каждый цикл).
TMP=$(mktemp -d -t rci-diag.XXXXXX 2>/dev/null || mktemp -d /tmp/rci-diag.XXXXXX)
trap 'rm -rf "$TMP"' EXIT INT TERM

fetch_get() {
    path="$1"; out="$2"
    curl -sS --max-time 10 -w '\nHTTP %{http_code}\n' "$RCI_BASE$path" >"$out" 2>&1
}

fetch_post() {
    payload="$1"; out="$2"
    curl -sS --max-time 10 -X POST \
        -H 'Content-Type: application/json' \
        --data-raw "$payload" \
        -w '\nHTTP %{http_code}\n' \
        "$RCI_POST" >"$out" 2>&1
}

unwrap_batch() {
    in="$1"; out="$2"; shift 2
    keys="$*"
    if [ -z "$keys" ]; then
        sed '$d' "$in" >"$out"
        return
    fi
    # Срезаем HTTP-tail и пытаемся unwrap через jq/python/ничего.
    sed '$d' "$in" >"$TMP/_body.json"
    if command -v jq >/dev/null 2>&1; then
        expr='.[0]'
        for k in $keys; do
            expr="$expr.\"$k\""
        done
        if jq -c "$expr" "$TMP/_body.json" >"$out" 2>/dev/null; then
            return
        fi
    fi
    if command -v python3 >/dev/null 2>&1 || command -v python >/dev/null 2>&1; then
        py=$(command -v python3 2>/dev/null || command -v python)
        "$py" -c '
import sys, json
keys = sys.argv[1].split()
with open(sys.argv[2]) as f: data = json.load(f)
v = data[0]
for k in keys: v = v[k]
print(json.dumps(v, indent=2, sort_keys=False, ensure_ascii=False))
' "$keys" "$TMP/_body.json" >"$out" 2>/dev/null && return
    fi
    # Нет jq/python — оставляем сырое тело, в шапке скажем что unwrap пропущен.
    cp "$TMP/_body.json" "$out"
}

# Заголовок отчёта.
{
    echo "RCI diag: $(date -Iseconds 2>/dev/null || date)"
    echo "GET base: $RCI_BASE"
    echo "POST url: $RCI_POST"
    echo ""
    printf '%-35s %-8s %-8s %-8s %s\n' "ENDPOINT" "GET" "POST" "DIFF" "SIZE(g/p)"
    printf '%-35s %-8s %-8s %-8s %s\n' "--------" "---" "----" "----" "---------"
} | tee -a "$OUT"

# Таблица + накопление details в памяти (через временный файл DETAILS).
DETAILS="$TMP/details.log"
: >"$DETAILS"

echo "$ENDPOINTS" | while IFS='|' read -r path payload; do
    [ -z "$path" ] && continue

    get_raw="$TMP/get.raw"
    post_raw="$TMP/post.raw"
    get_body="$TMP/get.body"
    post_unwrap="$TMP/post.unwrap"

    fetch_get  "$path"    "$get_raw"
    fetch_post "$payload" "$post_raw"

    get_status=$(tail -1 "$get_raw"  | awk '{print $2}')
    post_status=$(tail -1 "$post_raw" | awk '{print $2}')

    sed '$d' "$get_raw" >"$get_body"

    keys=$(unwrap_keys "$path")
    if [ -n "$keys" ]; then
        unwrap_batch "$post_raw" "$post_unwrap" $keys
    else
        sed '$d' "$post_raw" >"$post_unwrap"
    fi

    # Нормализуем обе стороны (canonical JSON, sorted keys) перед сравнением —
    # иначе разница в whitespace/индентации даёт ложные DIFF. Предпочитаем jq.
    get_norm="$TMP/get.norm"
    post_norm="$TMP/post.norm"
    if command -v jq >/dev/null 2>&1; then
        jq -S -c . "$get_body"   >"$get_norm"   2>/dev/null || cp "$get_body"   "$get_norm"
        jq -S -c . "$post_unwrap" >"$post_norm" 2>/dev/null || cp "$post_unwrap" "$post_norm"
    elif command -v python3 >/dev/null 2>&1 || command -v python >/dev/null 2>&1; then
        py=$(command -v python3 2>/dev/null || command -v python)
        "$py" -c 'import sys,json; print(json.dumps(json.load(open(sys.argv[1])), sort_keys=True, ensure_ascii=False))' "$get_body"   >"$get_norm"   2>/dev/null || cp "$get_body"   "$get_norm"
        "$py" -c 'import sys,json; print(json.dumps(json.load(open(sys.argv[1])), sort_keys=True, ensure_ascii=False))' "$post_unwrap" >"$post_norm" 2>/dev/null || cp "$post_unwrap" "$post_norm"
    else
        cp "$get_body"   "$get_norm"
        cp "$post_unwrap" "$post_norm"
    fi

    diff_status="-"
    diff_text=""
    if [ "$get_status" = "200" ] && [ "$post_status" = "200" ]; then
        if diff -q "$get_norm" "$post_norm" >/dev/null 2>&1; then
            diff_status="match"
        else
            diff_status="DIFF"
            diff_text=$(diff -u "$get_norm" "$post_norm" 2>&1 || true)
        fi
    fi

    get_size=$(wc -c <"$get_body" 2>/dev/null | tr -d ' ')
    post_size=$(wc -c <"$post_unwrap" 2>/dev/null | tr -d ' ')
    : "${get_size:=0}"; : "${post_size:=0}"

    printf '%-35s %-8s %-8s %-8s %s/%s\n' \
        "$path" "${get_status:--}" "${post_status:--}" "$diff_status" "$get_size" "$post_size" \
        | tee -a "$OUT"

    {
        echo ""
        echo "========================================================================"
        echo "ENDPOINT: $path"
        echo "  GET  status=$get_status size=$get_size"
        echo "  POST status=$post_status size=$post_size"
        echo "  DIFF: $diff_status"
        echo "  POST payload: $payload"
        echo "------------------------------------------------------------------------"
        echo "[GET body]"
        cat "$get_body"
        echo ""
        echo "------------------------------------------------------------------------"
        echo "[POST raw]"
        sed '$d' "$post_raw"
        echo ""
        if [ "$diff_status" = "DIFF" ]; then
            echo "------------------------------------------------------------------------"
            echo "[POST unwrap (by '$keys')]"
            cat "$post_unwrap"
            echo ""
            echo "------------------------------------------------------------------------"
            echo "[diff -u GET vs POST-unwrap]"
            echo "$diff_text"
            echo ""
        fi
    } >>"$DETAILS"
done

echo "" | tee -a "$OUT"
cat "$DETAILS" >>"$OUT"

echo ""
echo "Полный отчёт: $OUT"
echo "Размер: $(wc -c <"$OUT" 2>/dev/null) байт"
