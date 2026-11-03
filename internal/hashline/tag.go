package hashline

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/bits"
)

// Tag returns the short hash of the BOM-free, LF-normalized, right-trimmed view.
func Tag(normalized []byte) string {
	input := bytes.TrimPrefix(normalized, []byte("\xef\xbb\xbf"))
	view := make([]byte, 0, len(input))
	for len(input) > 0 {
		end := bytes.IndexByte(input, '\n')
		if end < 0 {
			view = append(view, bytes.TrimRight(input, " \t\r")...)
			break
		}
		view = append(view, bytes.TrimRight(input[:end], " \t\r")...)
		view = append(view, '\n')
		input = input[end+1:]
	}
	return fmt.Sprintf("%04X", xxh32(view)&0xffff)
}

func xxh32(input []byte) uint32 {
	const (
		prime1 uint32 = 0x9e3779b1
		prime2 uint32 = 0x85ebca77
		prime3 uint32 = 0xc2b2ae3d
		prime4 uint32 = 0x27d4eb2f
		prime5 uint32 = 0x165667b1
	)
	round := func(acc uint32, lane []byte) uint32 {
		return bits.RotateLeft32(acc+binary.LittleEndian.Uint32(lane)*prime2, 13) * prime1
	}
	var acc uint32
	length := len(input)
	if length >= 16 {
		v1 := prime1
		v1 += prime2
		v2, v3 := prime2, uint32(0)
		v4 := uint32(0)
		v4 -= prime1
		for len(input) >= 16 {
			v1 = round(v1, input[0:4])
			v2 = round(v2, input[4:8])
			v3 = round(v3, input[8:12])
			v4 = round(v4, input[12:16])
			input = input[16:]
		}
		acc = bits.RotateLeft32(v1, 1) + bits.RotateLeft32(v2, 7) + bits.RotateLeft32(v3, 12) + bits.RotateLeft32(v4, 18)
	} else {
		acc = prime5
	}
	acc += uint32(length)
	for len(input) >= 4 {
		acc = bits.RotateLeft32(acc+binary.LittleEndian.Uint32(input[:4])*prime3, 17) * prime4
		input = input[4:]
	}
	for _, b := range input {
		acc = bits.RotateLeft32(acc+uint32(b)*prime5, 11) * prime1
	}
	acc ^= acc >> 15
	acc *= prime2
	acc ^= acc >> 13
	acc *= prime3
	acc ^= acc >> 16
	return acc
}
