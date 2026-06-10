# compslices

Go high-performance columnar compression for telemetry, logs, and database storage.

`compslices` is a production-grade, highly-optimized bit-packed columnar compression library. It is designed specifically for stream-oriented storage engines, timeseries systems, and log aggregation servers that require fast sequential reading, append-friendly writing, and concurrent query execution without locking overhead.

## Key Features

- **Adaptive Group Bit-Packing**: Compresses integers using delta-zigzag packing in groups of 64 elements, supporting bit-widths from 0 to 64.
- **Fast Append Paths**: Allows appending single records or batches directly to uncompressed tails without rewriting compiled historical blocks.
- **Lock-Free Concurrency**: Modeled around Go native slices, enabling a single writer and multiple concurrent readers to access historical blocks with zero-locking synchronization and zero thread contention.
- **Zero-Copy Architecture**: Uses standard memory alignments and direct byte slice views to avoid unnecessary heap allocations during both reading and writing.

## Installation

```bash
go get github.com/ronanh/compslices
```

## Quick Start

### 1. Compressed Numeric Slice

Manage compressed integer streams (timestamps, offsets, indices) using [compslices/compressed_slice.go](compslices/compressed_slice.go):

```go
package main

import (
	"fmt"
	"github.com/ronanh/compslices"
)

func main() {
	var slice compslices.CompressedSlice[int64]

	// Add integer metrics or timestamps
	slice.Add([]int64{100, 101, 102, 105, 110, 120})
	slice.AddOne(135)

	fmt.Printf("Total elements: %d\n", slice.Len())
	fmt.Printf("First value: %d\n", slice.FirstValue())
	fmt.Printf("Last value: %d\n", slice.LastValue())

	// Read individuals or retrieve all
	all := slice.GetAll(nil)
	fmt.Println("Decompressed:", all)
}
```

### 2. Compressed Bytes Slice

Efficiently pack variable-length logs or raw trace messages utilizing [compslices/compressed_bytes_slice.go](compslices/compressed_bytes_slice.go):

```go
package main

import (
	"fmt"
	"bytes"
	"github.com/klauspost/compress/zstd"
	"github.com/ronanh/compslices"
)

func main() {
	var cBytes compslices.CompressedBytesSlice

	// Initialize reusable compressors (e.g. zstd)
	enc, _ := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	defer enc.Close()

	// Append list of messages
	messages := [][]byte{
		[]byte("info: server starting"),
		[]byte("info: connecting to database"),
		[]byte("warn: slow connection detected"),
	}
	cBytes.Add(messages, enc)

	// Fetch single value
	dec, _ := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
	defer dec.Close()

	val := cBytes.Get(1, dec)
	fmt.Printf("Message at position 1: %s\n", val)
}
```

## Documentation

For a detailed technical specification of our adaptive packing algorithms, physical storage models, and strict concurrent-read/write bounds, see the [compslices/docs/design.md](docs/design.md) specification file.

## License

MIT License. See LICENSE for details.
