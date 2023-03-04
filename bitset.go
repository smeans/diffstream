//
// copyright 2023 Scott Means Consulting, LLC DBA CloudTerm Partners
//

package diffstream

import (
	"fmt"
	"strings"
)

type BitSet struct {
	Len  int
	Bits []byte
}

func NewBitSet(l int) *BitSet {
	return &BitSet{
		Len:  l,
		Bits: make([]byte, l/8+int(min(l%8, 1))),
	}
}

func (bs *BitSet) Set(i int) {
	bs.Bits[i/8] |= 1 << (i % 8)
}

func (bs *BitSet) Clear(i int) {
	bs.Bits[i/8] &= ^(1 << (i % 8))
}

func (bs *BitSet) Get(i int) bool {
	return (bs.Bits[i/8] & (1 << (i % 8))) != 0
}

func (bs *BitSet) Clone() *BitSet {
	nbs := NewBitSet(bs.Len)
	copy(nbs.Bits, bs.Bits)

	return nbs
}

func (bs *BitSet) String() string {
	s := make([]byte, bs.Len)

	for i := 0; i < bs.Len; i++ {
		if bs.Get(i) {
			s[len(s)-i-1] = '1'
		} else {
			s[len(s)-i-1] = '0'
		}
	}

	return string(s)
}

func (bs *BitSet) Format(f fmt.State, verb rune) {
	if verb == 'v' && f.Flag('+') {
		var sb strings.Builder

		for i := bs.Len - 1; i >= 0; i-- {
			if bs.Get(i) {
				sb.WriteRune('1')
			} else {
				sb.WriteRune('0')
			}

			if i > 0 {
				sb.WriteRune(' ')
			}
		}

		f.Write([]byte(sb.String()))

		return
	}

	f.Write([]byte(bs.String()))
}
