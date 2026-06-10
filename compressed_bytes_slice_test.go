package compslices_test

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ronanh/compslices"
)

func TestCompress(t *testing.T) {
	testInput1, testInput2 := genTestInputs(1000), genTestInputs(1000)
	var cs compslices.CompressedBytesSlice

	enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1), zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		t.Fatalf("expected no error")
	}
	defer enc.Close()

	cs = cs.Append(testInput1, enc)
	checkExpectedInput(t, testInput1, cs)

	cs = cs.Append(testInput2, enc)
	expected2 := append(testInput1, testInput2...)
	checkExpectedInput(t, expected2, cs)

	dec, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
	if err != nil {
		t.Fatalf("expected no error")
	}
	defer dec.Close()

	for i := range expected2 {
		// check Get(i)
		got := cs.Get(i, dec)
		if !bytes.Equal(got, expected2[i]) {
			t.Fatalf("Get(%d): got %s, expected %s", i, got, expected2[i])
		}
	}
}

func TestCompressBytes(t *testing.T) {
	testInput1, testInput2 := genTestInputs(1000), genTestInputs(1000)
	var testInputBytes1, testInputBytes2 []byte
	var testInputOffsets1, testInputOffsets2 []int64
	var curOffset int64
	for _, v := range testInput1 {
		testInputOffsets1 = append(testInputOffsets1, curOffset)
		testInputBytes1 = append(testInputBytes1, v...)
		curOffset += int64(len(v))
	}
	curOffset = 0
	for _, v := range testInput2 {
		testInputOffsets2 = append(testInputOffsets2, curOffset)
		testInputBytes2 = append(testInputBytes2, v...)
		curOffset += int64(len(v))
	}

	var cs compslices.CompressedBytesSlice

	enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1), zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		t.Fatalf("expected no error")
	}
	defer enc.Close()

	cs = cs.AppendBytes(testInputBytes1, testInputOffsets1, enc)
	checkExpectedInput(t, testInput1, cs)

	cs = cs.AppendBytes(testInputBytes2, testInputOffsets2, enc)
	expectedInput := append(testInput1, testInput2...)
	checkExpectedInput(t, expectedInput, cs)

	var cs2 compslices.CompressedBytesSlice
	for _, in := range expectedInput {
		cs2 = cs2.AppendOne(in, enc)
	}
	checkExpectedInput(t, expectedInput, cs2)
}

func checkExpectedInput(t *testing.T, expectedInput [][]byte, res compslices.CompressedBytesSlice) {
	dec, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
	if err != nil {
		t.Fatalf("expected no error")
	}
	defer dec.Close()

	if res.Len() != len(expectedInput) {
		t.Fatalf("expected same len, got %d, expected %d", res.Len(), len(expectedInput))
	}
	var dstSlice compslices.BytesSlice
	var dstBuf []byte
	var dstBufOff []int64
	var values [][]byte

	// Decompression by block
	for i := 0; i < res.BlockCount(); i++ {
		dstSlice.Reset()
		var off int
		// check with DecompressBlock
		dstSlice, off = res.GetBlock(dstSlice, i, dec)
		for j := 0; j < dstSlice.Len(); j++ {
			if !bytes.Equal(dstSlice.Value(j), expectedInput[off+j]) {
				t.Fatalf("  got %s\n  expected %s", dstSlice.Value(j), expectedInput[off+j])
			}
		}
		// check block Values
		values = dstSlice.Values(values[:0])
		for j, v := range values {
			if !bytes.Equal(v, expectedInput[off+j]) {
				t.Fatalf("  got %s\n  expected %s", v, expectedInput[off+j])
			}
		}
		// check block Value(i)
		for j := 0; j < dstSlice.Len(); j++ {
			if !bytes.Equal(dstSlice.Value(j), dstSlice.Value(j)) {
				t.Fatalf("  got %s\n  expected %s", dstSlice.Value(j), dstSlice.Value(j))
			}
		}
		// check block ValuesBytes
		{
			dstBuf, dstBufOff := dstSlice.ValuesBytes()
			for j := 0; j < len(dstBufOff); j++ {
				endOff := int64(len(dstBuf))
				if j+1 < len(dstBufOff) {
					endOff = dstBufOff[j+1]
				}
				if !bytes.Equal(dstBuf[dstBufOff[j]:endOff], expectedInput[off+j]) {
					t.Fatalf("  got %s\n  expected %s", dstBuf[dstBufOff[j]:endOff], expectedInput[off+j])
				}
			}
		}

		// check with DecompressBlockBytes
		dstBuf, dstBufOff, off = res.GetBlockBytes(dstBuf[:0], dstBufOff[:0], i, dec)
		for j := 0; j < len(dstBufOff); j++ {
			endOff := int64(len(dstBuf))
			if j+1 < len(dstBufOff) {
				endOff = dstBufOff[j+1]
			}
			if !bytes.Equal(dstBuf[dstBufOff[j]:endOff], expectedInput[off+j]) {
				t.Fatalf("  got %s\n  expected %s", dstBuf[dstBufOff[j]:endOff], expectedInput[off+j])
			}
		}
	}

	// Decompression in one go
	dstSlice.Reset()
	dstSlice = res.GetAll(dstSlice, dec)
	// check len
	if dstSlice.Len() != len(expectedInput) {
		t.Fatalf("  got %d\n  expected %d", dstSlice.Len(), len(expectedInput))
	}
	for i := 0; i < dstSlice.Len(); i++ {
		if !bytes.Equal(dstSlice.Value(i), expectedInput[i]) {
			t.Fatalf("  got %s\n  expected %s", dstSlice.Value(i), expectedInput[i])
		}
	}

	// Decompression in one go with Bytes
	dstBuf, dstBufOff = res.GetBytes(dstBuf[:0], dstBufOff[:0], dec)
	// check len
	if len(dstBufOff) != len(expectedInput) {
		t.Fatalf("got %d, expected %d", len(dstBufOff), len(expectedInput))
	}
	for i := 0; i < len(dstBufOff); i++ {
		endOff := int64(len(dstBuf))
		if i+1 < len(dstBufOff) {
			endOff = dstBufOff[i+1]
		}
		if !bytes.Equal(dstBuf[dstBufOff[i]:endOff], expectedInput[i]) {
			t.Fatalf("  got %s\n  expected %s", dstBuf[dstBufOff[i]:endOff], expectedInput[i])
		}
	}
}

func TestCompressBytes2(t *testing.T) {
	rand.Seed(1) //nolint

	enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1), zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		t.Fatalf("expected no error")
	}
	defer enc.Close()

	for j := 0; j < 100; j++ {
		maxInputSize := rand.Intn(2000) + 1
		var cs compslices.CompressedBytesSlice
		const nbCompress = 100
		for i := 0; i < nbCompress; i++ {
			testInput := genTestInputs(rand.Intn(maxInputSize))

			var testInputBytes []byte
			var testInputOffsets []int64
			var curOffset int64
			for _, v := range testInput {
				testInputOffsets = append(testInputOffsets, curOffset)
				testInputBytes = append(testInputBytes, v...)
				curOffset += int64(len(v))
			}
			cs = cs.AppendBytes(testInputBytes, testInputOffsets, enc)
		}
	}
}

func TestImportExportBytesSlice(t *testing.T) {
	testInput1 := genTestInputs(1000)
	var cs compslices.CompressedBytesSlice

	enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1), zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		t.Fatalf("expected no error")
	}
	defer enc.Close()

	cs = cs.Append(testInput1, enc)
	checkExpectedInput(t, testInput1, cs)

	var cs2 compslices.CompressedBytesSlice
	cs2.Import(cs.Export())

	checkExpectedInput(t, testInput1, cs2)
}

func TestTruncateBytesSlice(t *testing.T) {
	testInput1 := genTestInputs(1000)
	var cs compslices.CompressedBytesSlice

	enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1), zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		t.Fatalf("expected no error")
	}
	defer enc.Close()
	dec, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
	if err != nil {
		t.Fatalf("expected no error")
	}
	defer dec.Close()

	cs = cs.Append(testInput1, enc)

	for i := range testInput1 {
		// t.Logf("truncate %d", i)
		cs2 := cs
		cs2.Truncate(i, dec)
		cs2.AddOne([]byte("test"), enc)

		checkExpectedInput(t, append(testInput1[:i:i], []byte("test")), cs2)
	}

}

// generate random strings (alphanum) of length 0-1000
// converted to []byte
func genTestInputs(n int) [][]byte {
	res := make([][]byte, n)
	for i := range res {
		res[i] = make([]byte, rand.Intn(1000))
		for j := range res[i] {
			// 48-57, 65-90, 97-122
			res[i][j] = byte(rand.Intn(74) + 48)
		}
	}
	return res
}
