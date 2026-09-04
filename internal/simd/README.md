# internal/simd: Multi-Architecture Comparative Benchmark and Profiling Harness

This package hosts the SIMD and SWAR acceleration kernels for TOON encoding and decoding, along with comparative benchmarking harnesses and CPU profiling tools.

## Comparative Sub-Benchmarks

`BenchmarkDelimScan` measures delimiter scanning throughput on identical synthetic tabular input buffers across multiple algorithm implementations and parameterized buffer sizes:

- **Algorithms:**
  - `Scalar`: Byte-by-byte baseline loop.
  - `SWAR`: 64-bit pure-Go SIMD-Within-A-Register using `math/bits`.
  - `AVX2`: 256-bit vector implementation (AMD64).
  - `NEON`: 128-bit vector implementation (ARM64).
- **Buffer Sizes:**
  - `64B`, `256B`, `1KB`, `16KB`, `64KB` (demonstrating startup overhead versus vector saturation).

## Running Benchmarks

Run the complete comparative suite:

```bash
go test -run=^$ -bench=BenchmarkDelimScan -benchtime=100ms ./internal/simd
```

Run a specific algorithm and buffer size:

```bash
go test -run=^$ -bench=BenchmarkDelimScan/SWAR/64KB -benchtime=500ms ./internal/simd
go test -run=^$ -bench=BenchmarkDelimScan/Scalar/64KB -benchtime=500ms ./internal/simd
```

## CPU Profiling with `-cpuprofile`

### Quick Script Runner

Use the included helper script:

```bash
# Capture and report profile for SWAR on 64KB buffers
./internal/simd/profile.sh SWAR 64KB

# Capture profile for Scalar on 16KB buffers
./internal/simd/profile.sh Scalar 16KB
```

### Manual Profiling Commands

Capture a profile:

```bash
go test -run=^$ -bench=^BenchmarkDelimScan/SWAR/64KB$ -benchtime=3s -cpuprofile=cpu_swar_64k.pprof ./internal/simd
```

Analyze in terminal:

```bash
go tool pprof -top cpu_swar_64k.pprof
```

Launch interactive flamegraph visualization:

```bash
go tool pprof -http=:8080 cpu_swar_64k.pprof
```

Compare two implementations side-by-side:

```bash
go test -run=^$ -bench=^BenchmarkDelimScan/Scalar/64KB$ -benchtime=3s -cpuprofile=cpu_scalar_64k.pprof ./internal/simd
go tool pprof -base cpu_scalar_64k.pprof cpu_swar_64k.pprof
```
