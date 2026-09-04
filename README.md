# TOON Format for Go

[![Go Reference](https://pkg.go.dev/badge/github.com/toon-format/toon-go.svg)](https://pkg.go.dev/github.com/toon-format/toon-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/toon-format/toon-go)](https://goreportcard.com/report/github.com/toon-format/toon-go)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)

**Token-Oriented Object Notation** is a compact, human-readable format designed for passing structured data to Large Language Models with significantly reduced token usage.

## SIMD performance

Measured on Apple M1 with synthetic TOON workloads; results vary by CPU and document shape.

| Path | Before | Now | Gain |
|---|---:|---:|---:|
| Tabular marshal, 1k rows | 2.01 ms | 880 µs | **2.28× faster** |
| Tabular unmarshal, 1k rows | 3.21 ms | 1.66 ms | **1.93× faster** |
| Inline split, 1k clean fields | 147 µs | 16.9 µs | **8.7× faster** |
| `NeedsQuoting`, clean 500B | 602 ns | 57 ns | **10.5× faster** |
| Scan 10k LF lines | 518 µs | 204 µs | **2.5× faster** |

### 1,000-Row Tabular Workload Breakdown

| Metric | Before | Now | Delta |
|---|---:|---:|---:|
| **Marshal Latency** | 1.97 ms | 0.88 ms | **2.24× faster** (sub-millisecond) |
| **Marshal Throughput** | 102 MB/s | 229 MB/s | **+124%** |
| **Marshal Allocations** | 26,872 allocs/op | 6 allocs/op | **-99.98%** (4,478× fewer) |
| **Marshal Memory** | 1,250 KB/op | 254 KB/op | **-79.7%** |
| **Unmarshal Latency** | 3.21 ms | 1.66 ms | **1.93× faster** |
| **Unmarshal Allocations** | 38,885 allocs/op | 15,084 allocs/op | **-61.2%** |

This implementation targets `toon-spec: 4.1` and is tested against the
official specification and fixture revision `v4.1.1` (submodule commit
`62f16b369408180f1faf1cba7da1b46d1f336f12`).

This library started out as a fork of https://github.com/toon-format/toon-go, as it had become unmaintained rather quickly after just 9 commits and seemingly abandoned.

The goal is to have a high performance implementation of TOON for Go, which leverages modern hardware features such as SIMD by using tools such as avo while still maintaining portability between platforms. It includes assembly optimizations for both x86_64 and ARM64 but also specific features just for Apple Silicon. 

All of these platform implementations are 100% compatible and have to pass all the same tests, just some can achieve a higher velocity than others.

## Example

**JSON** (verbose):
```json
{
  "users": [
    { "id": 1, "name": "Alice", "role": "admin" },
    { "id": 2, "name": "Bob", "role": "user" }
  ]
}
```

**TOON** (compact):
```
users[2]{id,name,role}:
  1,Alice,admin
  2,Bob,user
```

## Usage

### Marshal and Unmarshal

```go
package main

import (
    "fmt"

    "github.com/rx-bob/toon-go"
)

type User struct {
    ID    int    `toon:"id"`
    Name  string `toon:"name"`
    Role  string `toon:"role"`
}

type Payload struct {
    Users []User `toon:"users"`
}

func main() {
    in := Payload{
        Users: []User{
            {ID: 1, Name: "Alice", Role: "admin"},
            {ID: 2, Name: "Bob", Role: "user"},
        },
    }

    encoded, err := toon.Marshal(in)
    if err != nil {
        panic(err)
    }
    fmt.Println(string(encoded))

    var out Payload
    if err := toon.Unmarshal(encoded, &out); err != nil {
        panic(err)
    }
    fmt.Printf("first user: %+v\n", out.Users[0])
}
```

### Unmarshal into Maps

`Unmarshal` can populate dynamic maps, mimicking the `encoding/json` package:

```go
var doc map[string]any
if err := toon.Unmarshal(encoded, &doc); err != nil {
    panic(err)
}
fmt.Printf("users: %#v\n", doc["users"])
```

Go map iteration does not preserve TOON encounter order. Use `toon.Object` and
`toon.NewObject` when field order matters during encoding; `Object` preserves
its field order recursively. Decoding into maps follows normal Go map
semantics, while tabular and keyed decoding use their declared field order
when materializing values.

### Decode Without Structs

If you do not have a destination struct, use `Decode` for a dynamic representation:

```go
package main

import (
    "fmt"
    "github.com/rx-bob/toon-go"
)

func main() {
    raw := []byte("users[2]{id,name,role}:\n  1,Alice,admin\n  2,Bob,user\n")
    decoded, err := toon.Decode(raw)
    if err != nil {
        panic(err)
    }
    fmt.Printf("%+v\n", decoded)
}
```

For more runnable samples, explore the programs in `./examples`.

### Numeric policy

`json.Number` values are compared as exact decimal values before encoding. Values
that are exactly representable as `float64` use canonical formatting; large
integers, precise decimals, and out-of-range exponents retain their valid
numeric lexeme instead of being rounded. Invalid `json.Number` lexemes are
encoded as strings. Native integers outside the safe `float64` integer range
remain strings for lossless decoding.

Decoding is strict by default. Use `WithStrictMode(false)` for documented
non-strict policies, including last-write-wins duplicate keys. `WithDelimiter`
sets the delimiter for all emitted array scopes. `WithDocumentDelimiter`,
`WithArrayDelimiter`, and `WithLengthMarkers` remain available only as
deprecated compatibility aliases; length markers are ignored in v4.1.

The supported public API is `Marshal`/`MarshalString`, `Unmarshal`/
`UnmarshalString`, `Decode`/`DecodeString`, `NewEncoder`, `NewDecoder`,
`Object`/`NewObject`, and the documented encoder and decoder options. Dynamic
decoding uses `map[string]any`, `[]any`, `float64`, `string`, `bool`, and `nil`.
Use `Object` when encoded field order must be preserved; Go maps are sorted
for deterministic encoding and cannot preserve decode encounter order.

### Decoder safeguards

The decoder rejects recursive input deeper than 64 structural levels and array
headers longer than 64 KiB. These conservative implementation safeguards
prevent stack exhaustion and excessive header work; they do not change the
TOON format's logical nesting or document-size model. Declared array counts
are validated but are never used to preallocate result storage, so a huge
declared count with little input remains bounded by the input actually read.

## Resources

- [TOON Specification](https://github.com/toon-format/spec/blob/main/SPEC.md)
- [Main Repository](https://github.com/toon-format/toon)
- [Benchmarks & Performance](https://github.com/toon-format/toon#benchmarks)
- [Other Language Implementations](https://github.com/toon-format/toon#other-implementations)

## License

MIT License © 2025-PRESENT [Johann Schopplich](https://github.com/johannschopplich) for the original toon-format/toon-go

MIT License © 2026-PRESENT [Bob](https://github.com/rx-bob) for this very fork
