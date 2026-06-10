package compslices

import (
	"fmt"
	"unsafe"
)

//go:generate go run gen/gendeltapack.go gen/genhelper.go
//go:generate gofmt -w deltapack_gen.go

const (
	groupSize = 64
)

type CompressedSlice[T PackType] struct {
	// compressed buffer
	buf []uint64
	// uncompressed tail
	tail []T
	// block offsets
	blockOffsets []int64
	// optional min and max values
	minMax []minMax[T]
}

func (cs *CompressedSlice[T]) Import(buf []uint64, tail []T, blockOffsets []int64, minMax []minMax[T]) {
	cs.buf = buf
	cs.tail = tail
	if len(tail) > 0 {
		v := tail[0]
		if constTail := getConstTail(v); constTail != nil {
			// tail may be a constant
			for i := 1; i < len(tail); i++ {
				if tail[i] != v {
					constTail = nil
					break
				}
			}
			if constTail != nil {
				cs.tail = constTail[:len(tail):len(tail)]
			}
		}
	}
	cs.blockOffsets = blockOffsets
}

func (cs *CompressedSlice[T]) Export() ([]uint64, []T, []int64, []minMax[T]) {
	return cs.buf, cs.tail, cs.blockOffsets, cs.minMax
}

func (cs *CompressedSlice[T]) WithMinMax(v bool) {
	if v {
		cs.minMax = make([]minMax[T], 0)
	} else {
		cs.minMax = nil
	}
}

func (cs *CompressedSlice[T]) Len() int {
	if cs.BlockCount() == 0 {
		return 0
	}
	if len(cs.tail) > 0 {
		return len(cs.tail) + cs.blockOffset(cs.BlockCount()-1)
	}
	return cs.blockOffset(cs.BlockCount())
}

func (cs *CompressedSlice[T]) CompressedSize() int {
	return len(cs.buf)*8 + len(cs.blockOffsets)*8 + len(cs.tail)*int(unsafe.Sizeof(T(0)))
}

func (cs *CompressedSlice[T]) MemSize() int {
	return int(unsafe.Sizeof(*cs)) + cap(cs.buf)*8 + cap(cs.tail)*int(unsafe.Sizeof(T(0))) + cap(cs.blockOffsets)*8 + cap(cs.minMax)*int(unsafe.Sizeof(minMax[T]{}))
}

func (cs *CompressedSlice[T]) FirstValue() T {
	if len(cs.blockOffsets) > 0 {
		return *(*T)(unsafe.Pointer(&cs.buf[0]))
	}
	if len(cs.tail) > 0 {
		return cs.tail[0]
	}
	panic("empty slice")
}

func (cs *CompressedSlice[T]) LastValue() T {
	if len(cs.tail) > 0 {
		return cs.tail[len(cs.tail)-1]
	}
	if len(cs.blockOffsets) > 0 {
		// need to uncompressed last block
		lastBlock, _ := cs.GetBlock(nil, len(cs.blockOffsets)-1)
		return lastBlock[len(lastBlock)-1]
	}
	panic("empty slice")
}

func (cs *CompressedSlice[T]) IsBlockCompressed(iBlock int) bool {
	return iBlock < len(cs.blockOffsets)
}

func (cs *CompressedSlice[T]) BlockCount() int {
	if len(cs.tail) > 0 {
		return len(cs.blockOffsets) + 1
	}
	return len(cs.blockOffsets)
}

// BlockNum returns the block (iBlock) given the index in the uncompressed slice
func (cs *CompressedSlice[T]) BlockNum(i int) int {
	var (
		iBlock   int
		groupNum int = i / groupSize
	)
	if len(cs.blockOffsets) > 0 {
		nbGroupsPerBlock := BlockHeader(cs.buf[1:3]).GroupCount()
		for groupNum >= nbGroupsPerBlock && nbGroupsPerBlock < MaxGroups {
			groupNum -= nbGroupsPerBlock
			nbGroupsPerBlock++
			iBlock++
		}
	}
	if groupNum < 0 {
		groupNum = 0
	}
	return iBlock + groupNum/MaxGroups
}

func (cs *CompressedSlice[T]) BlockLen(iBlock int) int {
	if iBlock >= len(cs.blockOffsets) {
		if iBlock > len(cs.blockOffsets) || len(cs.tail) == 0 {
			panic("invalid block index")
		}
		return len(cs.tail)
	}
	nbGroupsPerBlock := BlockHeader(cs.buf[1:3]).GroupCount()
	for iBlock > 0 && nbGroupsPerBlock < MaxGroups {
		nbGroupsPerBlock++
		iBlock--
	}
	return nbGroupsPerBlock * groupSize
}

func (cs *CompressedSlice[T]) BlockFirstValue(iBlock int) T {
	if iBlock < len(cs.blockOffsets) {
		return *(*T)(unsafe.Pointer(&cs.buf[cs.blockOffsets[iBlock]]))
	}
	if iBlock == len(cs.blockOffsets) && len(cs.tail) > 0 {
		return cs.tail[0]
	}
	panic("invalid block index")
}

func (cs *CompressedSlice[T]) BlockMinMax(iBlock int) (T, T) {
	if cs.minMax == nil {
		panic("minmax not enabled")
	}
	if iBlock < len(cs.minMax) {
		return cs.minMax[iBlock].min, cs.minMax[iBlock].max
	}
	if iBlock == len(cs.minMax) && len(cs.tail) > 0 {
		// find min and max of tail
		min, max := cs.tail[0], cs.tail[0]
		for _, v := range cs.tail {
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
		}
		return min, max
	}
	panic("invalid block index")
}

func (cs CompressedSlice[T]) Append(src []T) CompressedSlice[T] {
	cs.add(src, 0)
	return cs
}

func (cs CompressedSlice[T]) AppendLossy(src []T, maxBits int) CompressedSlice[T] {
	minNtz := cs.fractionSize() - maxBits
	if minNtz < 1 {
		panic("maxBits too large")
	}
	cs.add(src, minNtz)
	return cs
}

func (cs CompressedSlice[T]) AppendOne(src T) CompressedSlice[T] {
	cs.addOne(src, 0)
	return cs
}

func (cs CompressedSlice[T]) AppendOneLossy(src T, maxBits int) CompressedSlice[T] {
	minNtz := cs.fractionSize() - maxBits
	if minNtz < 1 {
		panic("maxBits too large")
	}
	cs.addOne(src, minNtz)
	return cs
}

func (cs *CompressedSlice[T]) Add(src []T) {
	cs.add(src, 0)
}

func (cs *CompressedSlice[T]) AddLossy(src []T, maxBits int) {
	minNtz := cs.fractionSize() - maxBits
	if minNtz < 1 {
		panic("maxBits too large")
	}
	cs.add(src, minNtz)
}

func (cs *CompressedSlice[T]) AddOne(src T) {
	cs.addOne(src, 0)
}

func (cs *CompressedSlice[T]) AddOneLossy(src T, maxBits int) {
	minNtz := cs.fractionSize() - maxBits
	if minNtz < 1 {
		panic("maxBits too large")
	}
	cs.addOne(src, minNtz)
}

func (cs *CompressedSlice[T]) add(src []T, minNtz int) {
	nbGroupsPerBlock := 1
	if len(cs.blockOffsets) > 0 {
		gc := BlockHeader(cs.buf[1:3]).GroupCount()
		if gc > MaxGroups || gc < 1 {
			panic(fmt.Errorf("invalid GroupCount: %d", gc))
		}
		nbGroupsPerBlock = gc + len(cs.blockOffsets)
	}
	for len(src) > 0 {
		if nbGroupsPerBlock > MaxGroups {
			nbGroupsPerBlock = MaxGroups
		}
		if len(cs.tail) > 0 {
			appendTailLen := groupSize*nbGroupsPerBlock - len(cs.tail)
			if appendTailLen > len(src) {
				appendTailLen = len(src)
			} else if appendTailLen < 0 {
				panic(fmt.Errorf("appendTailLen:%d < 0, len(cs.tail): %d, nbGroupsPerBlock: %d, nbCompressedBlocks: %d, len(src): %d", appendTailLen, len(cs.tail), nbGroupsPerBlock, len(cs.blockOffsets), len(src)))
			}

			if constTail := cs.getConstTailForSlice(src[:appendTailLen]); constTail != nil {
				cs.tail = constTail[: len(cs.tail)+appendTailLen : len(cs.tail)+appendTailLen]
			} else {
				cs.tail = append(cs.tail, src[:appendTailLen]...)
			}
			// cs.tail = append(cs.tail, src[:appendTailLen]...)
			src = src[appendTailLen:]
			if len(cs.tail) == groupSize*nbGroupsPerBlock {
				// tail is full, compress it
				cs.addBlock(cs.tail, minNtz)
				cs.tail = nil
			}
		} else if len(src) >= groupSize*nbGroupsPerBlock {
			// compress a full block
			cs.addBlock(src[:groupSize*nbGroupsPerBlock], minNtz)
			src = src[groupSize*nbGroupsPerBlock:]
		} else {
			// append to tail
			if constTail := cs.getConstTailForSlice(src); constTail != nil {
				cs.tail = constTail[:len(src):len(src)]
			} else {
				cs.tail = append(cs.tail, src...)
			}
			// cs.tail = append(cs.tail, src...)
			src = nil
		}
		nbGroupsPerBlock++
	}
}

func (cs *CompressedSlice[T]) addOne(src T, minNtz int) {
	nbGroupsPerBlock := 1
	if len(cs.blockOffsets) > 0 {
		nbGroupsPerBlock = BlockHeader(cs.buf[1:3]).GroupCount() + len(cs.blockOffsets)
		if nbGroupsPerBlock > MaxGroups {
			nbGroupsPerBlock = MaxGroups
		}
	}

	var hasConstTail bool
	if constTail := cs.getConstTailForV(src); constTail != nil {
		cs.tail = constTail[: len(cs.tail)+1 : len(cs.tail)+1]
		hasConstTail = true
	} else {
		cs.tail = append(cs.tail, src)
	}
	if len(cs.tail) == groupSize*nbGroupsPerBlock {
		// tail is full, compress it
		cs.addBlock(cs.tail, minNtz)
		if hasConstTail {
			cs.tail = nil
		} else {
			// reset tail
			// the client is using addOne to add one value at a time
			// so it's better to alloc preemtively a new tail to avoid
			// reallocations
			nbGroupsPerBlock++
			if nbGroupsPerBlock > MaxGroups {
				nbGroupsPerBlock = MaxGroups
			}
			cs.tail = make([]T, 0, groupSize*nbGroupsPerBlock)
		}
	}
}

func (cs *CompressedSlice[T]) addBlock(block []T, minNtz int) {
	blockOffsetvalue := block[0]
	var bh BlockHeader
	BlockHeaderPos := int64(len(cs.buf))
	// append block header (blockOffsetValue + block header)
	cs.buf = append(cs.buf, *(*uint64)(unsafe.Pointer(&blockOffsetvalue)))
	cs.buf = append(cs.buf, bh[:]...)
	if cs.minMax != nil {
		min, max := block[0], block[0]
		for _, v := range block {
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
		}
		cs.minMax = append(cs.minMax, minMax[T]{min, max})
	}
	for len(block) > 0 {
		var bitlen, ntz int
		group := (*[groupSize]T)(block)
		switch any(T(0)).(type) {
		case float32, float64:
			cs.buf, bitlen, ntz, _ = compressGroupXorAppend(cs.buf, group, blockOffsetvalue, minNtz)
		default:
			cs.buf, bitlen, ntz, _ = compressGroupDeltaAppend(cs.buf, group, blockOffsetvalue)
		}
		blockOffsetvalue = group[groupSize-1]
		bh = bh.AddGroup(bitlen, ntz)
		block = block[groupSize:]
	}
	*(*BlockHeader)(cs.buf[BlockHeaderPos+1:]) = bh
	cs.blockOffsets = append(cs.blockOffsets, BlockHeaderPos)
}

func (cs *CompressedSlice[T]) LShiftBlocks(nBlocks int) {
	if nBlocks < 0 {
		panic("nBlocks should be positive")
	}
	if nBlocks == len(cs.blockOffsets)+1 && len(cs.tail) > 0 {
		// truncate tail
		cs.tail = nil
		nBlocks--
	}
	if nBlocks == 0 {
		return
	}
	if nBlocks > len(cs.blockOffsets) {
		panic("nBlocks too large")
	}

	// copy+truncate to shift blocks
	shiftBlockOffset := int64(len(cs.buf))
	if nBlocks < len(cs.blockOffsets) {
		shiftBlockOffset = cs.blockOffsets[nBlocks]
	}
	copy(cs.buf, cs.buf[shiftBlockOffset:])
	cs.buf = cs.buf[:len(cs.buf)-int(shiftBlockOffset)]
	copy(cs.blockOffsets, cs.blockOffsets[nBlocks:])
	cs.blockOffsets = cs.blockOffsets[:len(cs.blockOffsets)-nBlocks]
	for i := range cs.blockOffsets {
		cs.blockOffsets[i] -= shiftBlockOffset
	}
	if cs.minMax != nil {
		copy(cs.minMax, cs.minMax[nBlocks:])
		cs.minMax = cs.minMax[:len(cs.minMax)-nBlocks]
	}
	if len(cs.tail) > 0 {
		// check that tail is not bigger than next block
		// if so, re-add tail to ensure proper state
		nbGroupsPerBlock := 1
		if len(cs.blockOffsets) > 0 {
			nbGroupsPerBlock = BlockHeader(cs.buf[1:3]).GroupCount() + len(cs.blockOffsets)
			if nbGroupsPerBlock > MaxGroups {
				nbGroupsPerBlock = MaxGroups
			}
		}
		if len(cs.tail) >= groupSize*nbGroupsPerBlock {
			buf := cs.tail
			cs.tail = nil
			cs.add(buf, 0)
		}

	}
}

func (cs *CompressedSlice[T]) Get(i int) T {
	iBlock := cs.BlockNum(i)
	if !cs.IsBlockCompressed(iBlock) {
		return cs.tail[i-cs.blockOffset(iBlock)]
	}
	block, blockOff := cs.GetBlock(nil, iBlock)
	return block[i-blockOff]
}

func (cs *CompressedSlice[T]) GetAll(dst []T) []T {
	if cap(dst) == 0 {
		dst = make([]T, 0, cs.Len())
	}
	blockCount := cs.BlockCount()
	for i := 0; i < blockCount; i++ {
		dst, _ = cs.GetBlock(dst, i)
	}
	return dst
}

func (cs *CompressedSlice[T]) GetBlock(dst []T, iBlock int) ([]T, int) {
	if cap(dst) == 0 {
		sz := cs.Len()
		if sz > MaxGroups*groupSize {
			sz = MaxGroups * groupSize
		}
		dst = make([]T, 0, sz)
	}
	if iBlock < len(cs.blockOffsets) {
		blockOffset := cs.blockOffsets[iBlock]
		blockOffsetvalue := *(*T)(unsafe.Pointer(&cs.buf[blockOffset]))
		bh := *(*BlockHeader)(cs.buf[blockOffset+1:])
		in := cs.buf[cs.blockOffsets[iBlock]+1+int64(len(bh)):]
		nbGroups := bh.GroupCount()
		for i := 0; i < nbGroups; i++ {
			bitlen, ntz := bh.GetGroup(i)
			switch any(T(0)).(type) {
			case float32, float64:
				dst = decompressGroupXorAppend(dst, in, blockOffsetvalue, bitlen, ntz)
			default:
				dst = decompressGroupDeltaAppend(dst, in, blockOffsetvalue, bitlen, ntz)
			}
			blockOffsetvalue = dst[len(dst)-1]
			in = in[bitlen-ntz:]
		}
		return dst, cs.blockOffset(iBlock)
	}
	if iBlock == len(cs.blockOffsets) && len(cs.tail) > 0 {
		return append(dst, cs.tail...), cs.blockOffset(iBlock)
	}
	panic("invalid block index")
}

// DeepCopy returns a deep copy of the CompressedSlice
// mainly designed to be used in conjunction Truncate
func (cs *CompressedSlice[T]) DeepCopy() (copy CompressedSlice[T]) {
	copy.blockOffsets = append(copy.blockOffsets, cs.blockOffsets...)
	copy.buf = append(copy.buf, cs.buf...)
	copy.tail = append(copy.tail, cs.tail...)
	if cs.minMax != nil {
		copy.minMax = append(copy.minMax, cs.minMax...)
	}
	return
}

func (cs *CompressedSlice[T]) Truncate(i int) {
	if i == 0 {
		cs.buf = nil
		cs.tail = nil
		cs.blockOffsets = nil
		if cs.minMax != nil {
			cs.minMax = make([]minMax[T], 0)
		}
		return
	}
	if i >= cs.Len() {
		return
	}
	iBlock := cs.BlockNum(i)
	tailLen := i - cs.blockOffset(iBlock)
	if iBlock < len(cs.blockOffsets) {
		if tailLen > 0 {
			// extract tail from compressed iBlock
			// and truncate to tailLen
			cs.tail, _ = cs.GetBlock(nil, iBlock)
			cs.tail = cs.tail[:tailLen:tailLen]
		} else {
			cs.tail = nil
		}
		// truncate compressed block
		cs.buf = cs.buf[:cs.blockOffsets[iBlock]:cs.blockOffsets[iBlock]]
		cs.blockOffsets = cs.blockOffsets[:iBlock:iBlock]
		if cs.minMax != nil {
			cs.minMax = cs.minMax[:iBlock:iBlock]
		}
	} else {
		// truncate tail
		cs.tail = cs.tail[:tailLen:tailLen]
	}
}

func (cs *CompressedSlice[T]) RemainingCapBytes() int {
	return cap(cs.buf) - len(cs.buf) + (cap(cs.tail)-len(cs.tail))*int(unsafe.Sizeof(T(0)))
}

func (cs *CompressedSlice[T]) blockOffset(iBlock int) int {
	var nbGroupsTotal int
	if len(cs.blockOffsets) > 0 {
		nbGroupsPerBlock := BlockHeader(cs.buf[1:3]).GroupCount()
		for iBlock > 0 && nbGroupsPerBlock < MaxGroups {
			nbGroupsTotal += nbGroupsPerBlock
			nbGroupsPerBlock++
			iBlock--
		}
	}
	nbGroupsTotal += iBlock * MaxGroups
	return nbGroupsTotal * groupSize
}

func (cs CompressedSlice[T]) fractionSize() int {
	switch any(T(0)).(type) {
	case float32:
		// size of float32 fraction is 23 bits
		// https://en.wikipedia.org/wiki/Single-precision_floating-point_format
		return 23
	case float64:
		// size of float64 fraction is 52 bits
		// https://en.wikipedia.org/wiki/Double-precision_floating-point_format
		return 52
	default:
		panic("fractionSize applies only to float types")
	}
}

func (cs CompressedSlice[T]) getConstTailForV(v T) *[MaxGroups * groupSize]T {
	if len(cs.tail) != cap(cs.tail) || (len(cs.tail) > 0 && cs.tail[0] != v) {
		// not a constant tail
		return nil
	}
	if constTail := getConstTail(v); constTail != nil && unsafe.SliceData(cs.tail) == &constTail[0] {
		return constTail
	}
	return nil
}

func (cs CompressedSlice[T]) getConstTailForSlice(src []T) *[MaxGroups * groupSize]T {
	v := src[0]
	if len(cs.tail) != cap(cs.tail) || (len(cs.tail) > 0 && cs.tail[0] != v) {
		// not a constant tail
		return nil
	}
	constTail := getConstTail(v)
	if constTail == nil || unsafe.SliceData(cs.tail) != &constTail[0] {
		return constTail
	}
	for _, v2 := range src {
		if v != v2 {
			// not a constant tail
			return nil
		}
	}
	return constTail
}

type minMax[T PackType] struct {
	min T
	max T
}

var (
	tailConsts []struct {
		v    any
		tail unsafe.Pointer
	}
)

func AddTailConst[T PackType](v T) {
	tail := new([MaxGroups * groupSize]T)
	for i := range tail {
		tail[i] = v
	}
	tailConsts = append(tailConsts, struct {
		v    any
		tail unsafe.Pointer
	}{v, unsafe.Pointer(tail)})
}

func getConstTail[T PackType](v T) *[MaxGroups * groupSize]T {
	for _, tc := range tailConsts {
		if tc.v == any(v) {
			return (*[MaxGroups * groupSize]T)(tc.tail)
		}
	}
	return nil
}
