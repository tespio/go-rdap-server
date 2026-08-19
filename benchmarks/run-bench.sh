#!/usr/bin/env bash
# Linux benchmark runner for CI. Mirrors benchmarks/run-bench.ps1.
# Usage: ./run-bench.sh [url] [duration_sec] [concurrency_csv] [outdir]
set -euo pipefail

URL="${1:-http://localhost:8443/domain/example.com}"
DUR="${2:-8}"
CONC="${3:-1,10,25,50,100,200}"
OUTDIR="${4:-benchmarks/results}"
HEY="${HEY:-hey}"

mkdir -p "$OUTDIR"

IFS=',' read -r -a concs <<< "$CONC"
echo "concurrency,requests,req_per_sec,p50_ms,p90_ms,p99_ms,max_ms,errors,mem_mb,cpu_sec" > "$OUTDIR/summary.csv"

for c in "${concs[@]}"; do
  echo "=== Concurrency = $c (${DUR}s) ==="

  # Baseline process metrics.
  mem0=$(grep VmHWM /proc/$(pgrep -f 'rdapd -config' | head -n1)/status 2>/dev/null | awk '{print $2}') || mem0=""
  mem0=${mem0:-0}

  # Run the load test.
  $HEY -z "${DUR}s" -c "$c" -o csv -m GET "$URL" > "$OUTDIR/hey-c$c.csv"

  # Post metrics.
  mem1=$(grep VmHWM /proc/$(pgrep -f 'rdapd -config' | head -n1)/status 2>/dev/null | awk '{print $2}') || mem1=""
  mem1=${mem1:-0}

  # Parse CSV. Columns: response-time,DNS+dialup,DNS,Request-write,Response-delay,Response-read,status-code,offset
  n=$(($(wc -l < "$OUTDIR/hey-c$c.csv") - 1))
  rps=$(awk -F, 'NR>1{s+=1}END{printf "%.1f", s/'$DUR'}' "$OUTDIR/hey-c$c.csv")
  errs=$(awk -F, 'NR>1 && $7>=400{s+=1}END{print s+0}' "$OUTDIR/hey-c$c.csv")

  # Latency percentiles (seconds -> ms).
  p50=$(awk -F, 'NR>1{a[NR]=$1}END{n=asort(a); printf "%.2f", a[int(n*0.50)]*1000}' "$OUTDIR/hey-c$c.csv")
  p90=$(awk -F, 'NR>1{a[NR]=$1}END{n=asort(a); printf "%.2f", a[int(n*0.90)]*1000}' "$OUTDIR/hey-c$c.csv")
  p99=$(awk -F, 'NR>1{a[NR]=$1}END{n=asort(a); printf "%.2f", a[int(n*0.99)]*1000}' "$OUTDIR/hey-c$c.csv")
  max_ms=$(awk -F, 'NR>1{if($1>m)m=$1}END{printf "%.2f", m*1000}' "$OUTDIR/hey-c$c.csv")

  cpu_sec=$(awk -F, 'NR>1{s+=$1}END{printf "%.2f", s}' "$OUTDIR/hey-c$c.csv")

  echo "$c,$n,$rps,$p50,$p90,$p99,$max_ms,$errs,$mem1,$cpu_sec" >> "$OUTDIR/summary.csv"
  echo "  $c conn | $n req | $rps rps | p50 ${p50}ms | p90 ${p90}ms | p99 ${p99}ms | err $errs | mem ${mem1}KB | cpu ${cpu_sec}s"
done

echo "Summary written to $OUTDIR/summary.csv"
