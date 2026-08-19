# Benchmarks

Synthetic load benchmarks for the RDAP server: throughput, latency, memory
footprint, and CPU usage across concurrency levels.

## Quick summary (local, single machine, in-memory store, rate limiting off)

| Concurrency | Req/s | p50 | p90 | p99 | Memory (MB) |
|-----------:|------:|----:|----:|----:|------------:|
| 1          | 1,135 | 0.8ms | 1.1ms | 1.6ms | 18.7 |
| 10         | 6,459 | 1.4ms | 2.0ms | 3.5ms | 21.6 |
| 25         | 5,831 | 3.7ms | 6.9ms | 12.8ms | 28.0 |
| 50         | 5,965 | 6.5ms | 15.9ms | 27.0ms | 29.7 |
| 100        | 6,082 | 8.0ms | 26.6ms | 270.6ms | 30.9 |
| 200        | 7,884 | 18.2ms | 43.5ms | 85.1ms | 36.6 |
| 400        | 6,899 | 16.4ms | 80.9ms | 991.7ms | 45.5 |

Observations:
- **Throughput** peaks around 200 concurrent connections (~7,900 req/s) on this
  machine; the server is IO/CPU-bound well before memory becomes a constraint.
- **Latency** stays sub-10ms at the 50th percentile up to ~100 connections; tail
  latency (p99) grows with concurrency as expected under contention.
- **Memory footprint is tiny** — ~19 MB idle, ~45 MB at 400 concurrent
  connections. The server is very memory-efficient.
- At 0 errors across all runs (with rate limiting off).

> Note: these are **synthetic, local-machine** numbers and will vary by hardware,
> store backend (Postgres/MySQL are slower than the in-memory store), and load
> generator. The in-memory store is used here to isolate server overhead.

## Running locally (Windows)

```powershell
# 1. Start the server with rate limiting off
.\rdapd.exe -config benchmarks\config-bench.yaml

# 2. Run the benchmark
.\benchmarks\run-bench.ps1 -Duration 8 -Concurrency @(1,10,25,50,100,200) -OutDir benchmarks\results

# 3. Generate charts
python benchmarks\chart.py benchmarks\results\summary.csv
```

## Running in CI

The `Benchmark` workflow (`.github/workflows/benchmark.yml`) runs on every push/PR
on `ubuntu-latest`, executes the load benchmark, generates charts, and uploads
`benchmarks/results/` (CSV + PNGs) as a downloadable artifact. It is
**informational only** — it does not gate the build, because shared CI runners
have variable performance.

## Files

```
benchmarks/
├── config-bench.yaml   # server config with rate limiting disabled
├── run-bench.ps1       # Windows/PowerShell runner
├── run-bench.sh        # Linux/CI runner
├── chart.py            # generates charts (matplotlib) from summary.csv
└── results/            # generated: summary.csv + throughput/latency/memory/cpu.png
```
