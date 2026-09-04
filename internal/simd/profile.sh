#!/usr/bin/env bash
# Scriptable benchmark and profiling runner for internal/simd
# Usage: ./profile.sh [algorithm] [size]
# Examples:
#   ./profile.sh SWAR 64KB
#   ./profile.sh Scalar 16KB
#   ./profile.sh NEON 64KB
#   ./profile.sh all

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ALGO="${1:-SWAR}"
SIZE="${2:-64KB}"
PROFILE_DIR="${SCRIPT_DIR}/profiles"

mkdir -p "${PROFILE_DIR}"

if [ "${ALGO}" = "all" ]; then
    echo "Running all comparative delimiter scan benchmarks..."
    go test -run=^$ -bench=BenchmarkDelimScan -benchtime=100ms "${SCRIPT_DIR}"
else
    BENCH_NAME="BenchmarkDelimScan/${ALGO}/${SIZE}"
    PPROF_FILE="${PROFILE_DIR}/cpu_${ALGO}_${SIZE}.pprof"
    echo "Benchmarking ${BENCH_NAME} and capturing CPU profile to ${PPROF_FILE}..."
    go test -run=^$ -bench="^${BENCH_NAME}$" -benchtime=1s -cpuprofile="${PPROF_FILE}" "${SCRIPT_DIR}"
    echo ""
    echo "Profile generated: ${PPROF_FILE}"
    echo "To view top routines:"
    echo "  go tool pprof -top ${PPROF_FILE}"
    echo "To view interactive browser UI with flame graphs:"
    echo "  go tool pprof -http=:8080 ${PPROF_FILE}"
fi
