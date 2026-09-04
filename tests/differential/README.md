# Differential corpus

`corpus.json` is a deterministic cross-language corpus. Its TOON column was
generated with `@toon-format/toon` v4.1.1 and checked against the pinned
`tests/spec` v4.1.1 fixtures. `differential_test.go` compares canonical Go
encoding with those artifacts and decodes the reference output back to the
JSON model.

The ordinary Go test graph intentionally has no JavaScript dependency. To
refresh this corpus, use an isolated checkout of the exact
`@toon-format/toon` v4.1.1 package, regenerate the TOON values, then run:

```sh
go test ./tests/differential
```

Comparisons exclude host-only values such as NaN, Infinity, arbitrary Go
types, and map encounter order. Go maps are normalized by sorted key, while
the corpus preserves only the shared JSON data model.
