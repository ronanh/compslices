package compslices

// BlockHeader is a uint64 used to store the number of bits in a group (7 bits) and the number of trailing zeros (6 bits).
// up to 4 groups can be stored in a block header.
// the number of groups are stored in the first 4 bits of the header.
type BlockHeader [2]uint64

const MaxGroups = 4 * len(BlockHeader{})

// GroupCount returns the number of groups in the block header.
func (bh BlockHeader) GroupCount() int {
	var res int
	for _, v := range bh {
		res += int(v >> 60)
	}
	return res
}

// AddGroup adds a group of bits to the block header.
// bitlen is the number of bits in the group, and ntz is the number of trailing zeros.
// bitlen and ntz are stored in the header as 7 and 6-bit values.
// adding a group increases the number of groups in the header by 1.
func (bh BlockHeader) AddGroup(bitlen, ntz int) BlockHeader {
	for i, v := range bh {
		if v>>60 == 4 {
			continue
		}
		groupNum := v >> 60
		bh[i] = (groupNum+1)<<60 | bh[i]&0x0fffffffffffffff | (uint64(bitlen)&0x7f<<6|uint64(ntz)&0x3f)<<(groupNum*13)
		return bh
	}
	panic("too many groups")
}

// GetGroup returns the bitlen and ntz of the group at index i.
func (bh BlockHeader) GetGroup(i int) (int, int) {
	iBlock, iGroup := i/4, i%4*13
	return int((bh[iBlock] >> (iGroup + 6)) & 0x7f), int((bh[iBlock] >> iGroup) & 0x3f)
}

// BlockLen returns the number of compressed uint64 in the block
// not including the header.
func (bh BlockHeader) BlockLen() int {
	var res int
	for i := 0; i < bh.GroupCount(); i++ {
		bitlen, ntz := bh.GetGroup(i)
		res += (bitlen - ntz)
	}
	return res
}
