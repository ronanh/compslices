package main

import (
	"fmt"
)

type deltapack struct {
}

func (dp *deltapack) Dpn() []dpN {
	dpn := make([]dpN, 64)
	for i := range dpn {
		dpn[i] = dpN{dp, i}
	}

	return dpn
}

type dpN struct {
	dp *deltapack
	N  int
}

func (dpn *dpN) DVal32() string {
	return dpn.dVal32(dpn.N)
}

func (dpn *dpN) dVal32(n int) string {
	if n == 0 {
		return "int64(*(*int32)(unsafe.Pointer(&in[0])) - *(*int32)(unsafe.Pointer(&blockOffsetValue)))"
	}
	return fmt.Sprintf("int64(*(*int32)(unsafe.Pointer(&in[%d])) - *(*int32)(unsafe.Pointer(&in[%d])))", n, n-1)
}

func (dpn *dpN) DVal64() string {
	return dpn.dVal64(dpn.N)
}

func (dpn *dpN) dVal64(n int) string {
	if n == 0 {
		return "*(*int64)(unsafe.Pointer(&in[0])) - *(*int64)(unsafe.Pointer(&blockOffsetValue))"
	}
	return fmt.Sprintf("*(*int64)(unsafe.Pointer(&in[%d])) - *(*int64)(unsafe.Pointer(&in[%d]))", n, n-1)
}

func (dpn *dpN) ZZDVar() string {
	return fmt.Sprintf("zzd%02d", dpn.N)
}

func (dpn *dpN) ZZDVal32() string {
	return dpn.zzdVal32(dpn.N)
}

func (dpn *dpN) zzdVal32(n int) string {
	return fmt.Sprintf("uint64(((%s) << 1) ^ ((%s) >> 63))", dpn.dVal32(n), dpn.dVal32(n))
}

func (dpn *dpN) ZZDVal64() string {
	return dpn.zzdVal64(dpn.N)
}

func (dpn *dpN) zzdVal64(n int) string {
	return fmt.Sprintf("uint64(((%s) << 1) ^ ((%s) >> 63))", dpn.dVal64(n), dpn.dVal64(n))
}

func (dpn *dpN) XORVar() string {
	return fmt.Sprintf("xor%02d", dpn.N)
}

func (dpn *dpN) XORVal32() string {
	return dpn.xorVal32(dpn.N)
}

func (dpn *dpN) xorVal32(n int) string {
	if n == 0 {
		return "uint64(uint32(*(*int32)(unsafe.Pointer(&blockOffsetValue)) ^ *(*int32)(unsafe.Pointer(&in[0]))))"
	}
	return fmt.Sprintf("uint64(uint32(*(*int32)(unsafe.Pointer(&in[%d])) ^ *(*int32)(unsafe.Pointer(&in[%d]))))", n-1, n)
}

func (dpn *dpN) XORVal64() string {
	return dpn.xorVal64(dpn.N)
}

func (dpn *dpN) xorVal64(n int) string {
	if n == 0 {
		return "uint64(*(*int64)(unsafe.Pointer(&blockOffsetValue)) ^ *(*int64)(unsafe.Pointer(&in[0])))"
	}
	return fmt.Sprintf("uint64(*(*int64)(unsafe.Pointer(&in[%d])) ^ *(*int64)(unsafe.Pointer(&in[%d])))", n-1, n)
}

func (dpn *dpN) Comma() string {
	if dpn.N == 0 {
		return ""
	}
	return ","
}

func (dpn *dpN) NbBytes() int {
	return dpn.N
}

func (dpn *dpN) Dpnb() []dpNByte {
	dpnb := make([]dpNByte, dpn.N)
	for i := range dpnb {
		dpnb[i] = dpNByte{dpn, i}
	}
	return dpnb
}

func (dpn *dpN) Dunb() []duNByte {
	dunb := make([]duNByte, 64)
	for i := range dunb {
		dunb[i] = duNByte{dpn, i}
	}
	return dunb
}

type dpNByte struct {
	dpn *dpN
	I   int
}

func (dpnb *dpNByte) PackLinesZigZag() []string {
	return dpnb.packLinesZigZag(false)
}

func (dpnb *dpNByte) PackLinesZigZagNtz() []string {
	return dpnb.packLinesZigZag(true)
}

func (dpnb *dpNByte) packLinesZigZag(ntz bool) []string {
	var lines []string
	nbBytes := dpnb.dpn.NbBytes()
	if nbBytes == 0 {
		return nil
	}
	var overlapping bool
	minOffset := 64 * dpnb.I
	maxOffset := 64 * (dpnb.I + 1)
	if minOffset%nbBytes != 0 {
		minOffset -= nbBytes
		overlapping = true
	}
	for i := 0; i < 64; i++ {
		offset := i * nbBytes
		if offset < minOffset || offset >= maxOffset {
			continue
		}

		ishift := offset - minOffset
		if overlapping {
			ishift -= nbBytes
		}

		diff := fmt.Sprintf("zzd%02d", i)

		var line string
		switch {
		case ishift < 0:
			if ntz {
				line = fmt.Sprintf("%s >> ntz >> %d", diff, -ishift)
			} else {
				line = fmt.Sprintf("%s >> %d", diff, -ishift)
			}
		case ishift > 0:
			if ntz {
				line = fmt.Sprintf("%s >> ntz << %d", diff, ishift)
			} else {
				line = fmt.Sprintf("%s << %d", diff, ishift)
			}
		default:
			if ntz {
				line = fmt.Sprintf("%s >> ntz", diff)
			} else {
				line = diff
			}
		}
		if offset+nbBytes < maxOffset {
			line += " | "
		}

		lines = append(lines, line)
	}
	return lines
}

func (dpnb *dpNByte) PackLinesXor() []string {
	var lines []string
	nbBytes := dpnb.dpn.NbBytes()
	if nbBytes == 0 {
		return nil
	}
	var overlapping bool
	minOffset := 64 * dpnb.I
	maxOffset := 64 * (dpnb.I + 1)
	if minOffset%nbBytes != 0 {
		minOffset -= nbBytes
		overlapping = true
	}
	for i := 0; i < 64; i++ {
		offset := i * nbBytes
		if offset < minOffset || offset >= maxOffset {
			continue
		}

		ishift := offset - minOffset
		if overlapping {
			ishift -= nbBytes
		}

		xor := fmt.Sprintf("xor%02d", i)

		var line string
		switch {
		case ishift < 0:
			line = fmt.Sprintf("%s >> ntz >> %d", xor, -ishift)
		case ishift > 0:
			line = fmt.Sprintf("%s >> ntz << %d", xor, ishift)
		default:
			line = fmt.Sprintf("%s >> ntz", xor)
		}
		if offset+nbBytes < maxOffset {
			line += " | "
		}

		lines = append(lines, line)
	}
	return lines
}

type duNByte struct {
	dpn *dpN
	I   int
}

func (dunb *duNByte) UnpackLineZigZagNtz() string {
	return dunb.unpackLineZigZag(true)
}

func (dunb *duNByte) UnpackLineZigZag() string {
	return dunb.unpackLineZigZag(false)
}

func (dunb *duNByte) unpackLineZigZag(ntz bool) string {
	if dunb.dpn.N == 0 {
		return "blockOffsetValue"
	}
	nbBytes := dunb.dpn.NbBytes()
	startByte := dunb.I * nbBytes / 64
	startBit := dunb.I * nbBytes % 64
	endByte := (dunb.I + 1) * nbBytes / 64
	endBit := (dunb.I + 1) * nbBytes % 64

	var startMask, endMask int
	in1 := fmt.Sprintf("in2[%d]", startByte)
	in2 := fmt.Sprintf("in2[%d]", endByte)

	var val string
	if startByte == endByte {
		for i := startBit; i < endBit; i++ {
			startMask <<= 1
			startMask |= 1
		}
		val = fmt.Sprintf("((%s >> %d) & 0x%X)", in1, startBit, startMask)
	} else {
		for i := startBit; i < 64; i++ {
			startMask <<= 1
			startMask |= 1
		}
		for i := 0; i < endBit; i++ {
			endMask <<= 1
			endMask |= 1
		}
		val = fmt.Sprintf("(%s >> %d)", in1, startBit)
		if endBit > 0 {
			val = fmt.Sprintf("(%s | ((%s & 0x%X) << %d))", val, in2, endMask, nbBytes-endBit)
		}
	}
	if ntz {
		val = fmt.Sprintf("(%s << ntz)", val)
	}
	return fmt.Sprintf("IT((-(%s & 1))^(%s>>1))", val, val)
}

func (dunb *duNByte) UnpackLineXor() string {
	if dunb.dpn.N == 0 {
		return "blockOffsetValue"
	}
	nbBytes := dunb.dpn.NbBytes()
	startByte := dunb.I * nbBytes / 64
	startBit := dunb.I * nbBytes % 64
	endByte := (dunb.I + 1) * nbBytes / 64
	endBit := (dunb.I + 1) * nbBytes % 64

	var startMask, endMask int
	in1 := fmt.Sprintf("in2[%d]", startByte)
	in2 := fmt.Sprintf("in2[%d]", endByte)

	var val string
	if startByte == endByte {
		for i := startBit; i < endBit; i++ {
			startMask <<= 1
			startMask |= 1
		}
		val = fmt.Sprintf("((%s >> %d) & 0x%X)", in1, startBit, startMask)
	} else {
		for i := startBit; i < 64; i++ {
			startMask <<= 1
			startMask |= 1
		}
		for i := 0; i < endBit; i++ {
			endMask <<= 1
			endMask |= 1
		}
		val = fmt.Sprintf("(%s >> %d)", in1, startBit)
		if endBit > 0 {
			val = fmt.Sprintf("(%s | ((%s & 0x%X) << %d))", val, in2, endMask, nbBytes-endBit)
		}
	}
	return fmt.Sprintf("IT(%s << ntz)", val)
}
