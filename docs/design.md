# Architecture & Concurrency Design

This document details the internal design of `compslices`, explaining how our adaptive bit-packing and XOR compression algorithms work, along with the strict zero-locking concurrency model.

---

## 1. Columnar Memory Model

Data is organized into fixed compression groups of 64 elements. Stored blocks are variable-sized and contain between 1 and 8 groups, so a block spans 64 to 512 elements depending on how far the writer has filled the current growth tier. Rather than maintaining fully uncompressed arrays, `compslices` employs two distinct areas:

1. **Compressed Blocks**: Once the current block reaches its target capacity, its 64-element groups are compressed into packed format using bit-packing or XOR transformation. These compressed sequences are cached inside a flat `[]uint64` buffer.
2. **Uncompressed Tail**: Elements appended after the last compiled block are held inside a flat uncompressed buffer (`tail`) matching the element's natural Go type.

For `CompressedSlice[T]`, block capacities grow in groups as `64, 128, 192, 256, 320, 384, 448, 512`, then remain capped at 512 elements per block.

This model is implemented in [compslices/compressed_slice.go](compressed_slice.go) and [compslices/compressed_bytes_slice.go](compressed_bytes_slice.go).

---

## 2. Adaptive Bit-Packing Compression

### Integer & Index packing (Delta-ZigZag)
For sequential integer sequences like timestamps, offsets, or monotonic indices, `compslices` calculates the relative difference (delta) from a preceding baseline element:

$$\delta_i = X_i - X_{i-1}$$

To support both positive and negative values efficiently, deltas are transformed using ZigZag encoding into unsigned integers:

$$ZZ(\delta_i) = (\delta_i \ll 1) \oplus (\delta_i \gg 63)$$

For each 64-element group, we find the maximum bit-width ($W$) required across that group. Each group is then packed continuously into a compact `[]uint64` with each element consuming exactly $W$ bits. A block header ([compslices/block_header.go](block_header.go)) records the metadata for each group in the block, including $W$ and any trailing zeros.

### Floating-Point Packing (Xor-Channing)
For timeseries containing floating-point numbers (float32, float64), delta calculations are not suitable due to IEEE 754 precision representation. Instead, we calculate the XOR bit-wise difference between successive float values:

$$\xi_i = F_i \oplus F_{i-1}$$

We extract the trailing zeros ($NTZ$) and leading zeros across the block, and pack the active significant fraction bits into our columnar structures, achieving high ratios on steady timeseries profiles.

---

## 3. Go Slice Concurrency Philosophy (Lock-Free Reader Guarantee)

`compslices` is optimized for database engines running high-frequency ingest (single-writer) and high-throughput real-time query parsing (multiple-readers) concurrently. To eliminate mutex locking overhead on hot read paths, the library implements the **Go Slice Concurrency Guarantee**.

### The Mechanism

In Go, slice headers are defined as a small, word-aligned structure carrying a backing array pointer, element count, and capacity:

```go
type slice struct {
    array unsafe.Pointer
    len   int
    cap   int
}
```

When a reader accesses a `CompressedSlice[T]` or `CompressedBytesSlice`, safety depends on it working from a snapshot copy of the structure's fields, or from previously copied slice headers derived from those fields:
- Backing buffers (`buf`, `tail`, `blockOffsets`) are copied as slice header values.
- Go specification guarantees that a slice copy evaluates safely to a consistent, captured viewport in memory.

As the single writer appends new elements or compiles new historical blocks:
1. It adds items to `tail` or appends historical blocks directly inside unused capacity bounds.
2. If the slice buffers exceed current capacity, the writer allocates completely new backing arrays and copies existing data across.
3. The original backing array is left unmodified, preserving the active view of older concurrent readers. Since Go GC only reclaims abandoned arrays once active references drop, concurrent readers can safely complete processing without page-faults or segment memory corruption.
4. The writer updates slice fields using atomic-like write increments. Future readers receive updated slice headers referencing the newly extended or completely newly allocated backing regions.

### Strict Guarantees

Under a single-writer, multiple-reader lifecycle, the following parameters remain safe to load without synchronization locks:
- `Len()`, `Get(i)`, `GetAll(...)`
- `GetBlock(...)`, `GetBlockBytes(...)`
- `Export()`

### Constraints

This execution model is strictly bounded by the following invariants:
1. **Single Writer Constraint**: Multiple threads must never attempt to call `Add`, `AddOne`, `AddBytes`, or `Truncate` concurrently. Writing must be serialized by an external lock or run from a dedicated ingest worker.
2. **No Value Mutations**: Decompressed buffers must never be altered once emitted. Methods return un-writable slice copies or explicit copy buffers to avoid data pollution on concurrent streams.
3. **Truncation Behavior**: `Truncate(i)` only changes the writer-visible slice headers and offsets. It shortens `buf`, `tail`, and block-offset slices with full-slice expressions that also reduce capacity to the new logical length. When truncation lands inside a compressed block, it first materializes a new truncated tail for the writer. Readers that already captured an older snapshot, meaning a copied struct value or copied slice headers, keep that original view and therefore continue to see the pre-truncate data. A reader that keeps dereferencing the same live mutable object is not holding an old snapshot; it is observing the writer's current headers. Future writes after truncation append into new backing storage instead of extending the previously published tail.
