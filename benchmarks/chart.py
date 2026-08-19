#!/usr/bin/env python3
"""Generate charts from the benchmark summary CSV.

Usage: python benchmarks/chart.py benchmarks/results/summary.csv
"""
import csv
import sys
import os

import matplotlib
matplotlib.use("Agg")  # headless
import matplotlib.pyplot as plt


def load(path):
    with open(path, newline="", encoding="utf-8-sig") as f:
        rows = list(csv.DictReader(f))
    for r in rows:
        r["Concurrency"] = int(r["Concurrency"])
        r["ReqPerSec"] = float(r["ReqPerSec"])
        r["P50_ms"] = float(r["P50_ms"])
        r["P90_ms"] = float(r["P90_ms"])
        r["P99_ms"] = float(r["P99_ms"])
        r["Max_ms"] = float(r["Max_ms"])
        r["MemMB"] = float(r["MemMB"])
        r["CPU_used_sec"] = float(r["CPU_used_sec"])
    return rows


def main(path):
    rows = load(path)
    conc = [r["Concurrency"] for r in rows]
    outdir = os.path.dirname(path) or "."

    # 1. Throughput vs concurrency
    fig, ax = plt.subplots(figsize=(8, 5))
    ax.plot(conc, [r["ReqPerSec"] for r in rows], marker="o", linewidth=2)
    ax.set_xlabel("Concurrency (connections)")
    ax.set_ylabel("Requests per second")
    ax.set_title("Throughput vs Concurrency")
    ax.grid(True, alpha=0.3)
    fig.tight_layout()
    fig.savefig(os.path.join(outdir, "throughput.png"), dpi=150)
    plt.close(fig)

    # 2. Latency percentiles vs concurrency
    fig, ax = plt.subplots(figsize=(8, 5))
    ax.plot(conc, [r["P50_ms"] for r in rows], marker="o", label="p50")
    ax.plot(conc, [r["P90_ms"] for r in rows], marker="s", label="p90")
    ax.plot(conc, [r["P99_ms"] for r in rows], marker="^", label="p99")
    ax.plot(conc, [r["Max_ms"] for r in rows], marker="x", label="max", alpha=0.7)
    ax.set_xlabel("Concurrency (connections)")
    ax.set_ylabel("Latency (ms)")
    ax.set_title("Latency vs Concurrency")
    ax.legend()
    ax.grid(True, alpha=0.3)
    fig.tight_layout()
    fig.savefig(os.path.join(outdir, "latency.png"), dpi=150)
    plt.close(fig)

    # 3. Memory footprint vs concurrency
    fig, ax = plt.subplots(figsize=(8, 5))
    ax.bar([str(c) for c in conc], [r["MemMB"] for r in rows])
    ax.set_xlabel("Concurrency (connections)")
    ax.set_ylabel("Working set (MB)")
    ax.set_title("Server Memory Footprint vs Concurrency")
    ax.grid(True, alpha=0.3, axis="y")
    fig.tight_layout()
    fig.savefig(os.path.join(outdir, "memory.png"), dpi=150)
    plt.close(fig)

    # 4. CPU used per run vs concurrency
    fig, ax = plt.subplots(figsize=(8, 5))
    ax.plot(conc, [r["CPU_used_sec"] for r in rows], marker="o", linewidth=2)
    ax.set_xlabel("Concurrency (connections)")
    ax.set_ylabel("CPU time consumed (s)")
    ax.set_title("CPU Usage vs Concurrency")
    ax.grid(True, alpha=0.3)
    fig.tight_layout()
    fig.savefig(os.path.join(outdir, "cpu.png"), dpi=150)
    plt.close(fig)

    print("Charts written to", outdir)


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("usage: chart.py <summary.csv>")
        sys.exit(1)
    main(sys.argv[1])
