param(
    [string]$Url = "http://localhost:8443/domain/example.com",
    [int]$Duration = 10,
    [int[]]$Concurrency = @(1, 10, 25, 50, 100, 200),
    [string]$OutDir = "benchmarks/results"
)

$ErrorActionPreference = "Stop"
$hey = "$env:USERPROFILE\go\bin\hey.exe"
$procName = "rdapd"

if (-not (Test-Path $OutDir)) { New-Item -ItemType Directory -Path $OutDir -Force | Out-Null }

$rows = @()

foreach ($c in $Concurrency) {
    Write-Host "`n=== Concurrency = $c (${Duration}s) ==="

    # Capture baseline process metrics before the run.
    $p0 = Get-Process -Name $procName -ErrorAction SilentlyContinue
    $mem0 = if ($p0) { $p0.WorkingSet64 } else { 0 }
    $cpu0 = if ($p0) { $p0.TotalProcessorTime.TotalMilliseconds } else { 0 }

    # Run the load test.
    $csv = Join-Path $OutDir "hey-c$c.csv"
    & $hey -z "${Duration}s" -c $c -o csv -m GET $Url | Out-File -FilePath $csv -Encoding utf8

    # Capture process metrics after the run.
    $p1 = Get-Process -Name $procName -ErrorAction SilentlyContinue
    $mem1 = if ($p1) { $p1.WorkingSet64 } else { 0 }
    $cpu1 = if ($p1) { $p1.TotalProcessorTime.TotalMilliseconds } else { 0 }

    # Parse the CSV summary (hey writes header + N data rows).
    $data = Import-Csv $csv
    $n = $data.Count
    $rps = 0.0
    $lat50 = 0.0; $lat90 = 0.0; $lat99 = 0.0; $latMax = 0.0
    $errs = 0
    if ($n -gt 0) {
        # RPS estimate: (total requests) / (duration seconds)
        $dur = $Duration
        $rps = $n / $dur
        # Latency percentiles from the response times (seconds)
        $lats = $data | ForEach-Object { [double]$_.'response-time' } | Sort-Object
        $lat50 = $lats[[int]([Math]::Floor($n * 0.50))]
        $lat90 = $lats[[int]([Math]::Floor($n * 0.90))]
        $lat99 = $lats[[int]([Math]::Floor($n * 0.99))]
        $latMax = $lats[$n - 1]
        $errs = ($data | Where-Object { [int]$_.'status-code' -ge 400 }).Count
    }

    $cpuUsed = if ($cpu1 -gt $cpu0) { ($cpu1 - $cpu0) / 1000 } else { 0 } # ms -> s
    $memMB = [Math]::Round($mem1 / 1MB, 2)
    $memDeltaMB = [Math]::Round(($mem1 - $mem0) / 1MB, 2)

    $row = [PSCustomObject]@{
        Concurrency      = $c
        Requests         = $n
        ReqPerSec        = [Math]::Round($rps, 1)
        P50_ms           = [Math]::Round($lat50 * 1000, 2)
        P90_ms           = [Math]::Round($lat90 * 1000, 2)
        P99_ms           = [Math]::Round($lat99 * 1000, 2)
        Max_ms           = [Math]::Round($latMax * 1000, 2)
        Errors           = $errs
        MemMB            = $memMB
        MemDeltaMB       = $memDeltaMB
        CPU_used_sec     = [Math]::Round($cpuUsed, 2)
    }
    $rows += $row
    Write-Host ("  {0,4} conn | {1,6} req | {2,8} rps | p50 {3,7}ms | p90 {4,7}ms | p99 {5,8}ms | err {6} | mem {7}MB (+{8}) | cpu {9}s" -f `
        $c, $n, [Math]::Round($rps,1), [Math]::Round($lat50*1000,2), [Math]::Round($lat90*1000,2), [Math]::Round($lat99*1000,2), $errs, $memMB, $memDeltaMB, [Math]::Round($cpuUsed,2))
}

$summary = Join-Path $OutDir "summary.csv"
$rows | Export-Csv -Path $summary -NoTypeInformation
Write-Host "`nSummary written to $summary"
