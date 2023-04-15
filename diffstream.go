//
// copyright 2023 Scott Means Consulting, LLC DBA CloudTerm Partners
//

// Package diffstream provides support for generating
// real-time text diffs from multiple input streams.
package diffstream

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/labstack/gommon/log"
)

type DSChunk struct {
	ChannelMask *BitSet
	Builder     strings.Builder
	locked      bool
}

func (ck *DSChunk) String() string {
	return ck.Builder.String()
}

func (ck *DSChunk) Format(f fmt.State, verb rune) {
	f.Write([]byte(ck.String()))
}

func (ck *DSChunk) IsLocked() bool {
	return ck.locked
}

func (ck *DSChunk) Exit(c int) {
	ck.ChannelMask.Set(c)
	ck.locked = true
}

func (ck *DSChunk) MarshalJSON() (data []byte, err error) {
	cm := make([]byte, base64.StdEncoding.EncodedLen(len(ck.ChannelMask.Bits)))
	base64.StdEncoding.Encode(cm, ck.ChannelMask.Bits)
	type _DSChunk struct {
		ChannelMap string `json:"channel_map"`
		Value      string `json:"value"`
	}

	dco := _DSChunk{
		ChannelMap: string(cm),
		Value:      ck.Builder.String(),
	}

	return json.Marshal(dco)
}

func (ck *DSChunk) findToken(t string, sp int) int {
	s := ck.String()

	if len(s) < sp+len(t) {
		return -1
	}

	tt := tokenType(t)
	tra := []rune(t)
	lrt := RTTNone
	mc := 0
	lts := -1

	for i, r := range s[sp:] {
		if tra[mc] == r && (mc > 0 || lrt != tt) {
			if mc == 0 {
				lts = i
			}

			mc += 1
			if mc >= len(tra) {
				return sp + lts
			}
		} else {
			mc = 0
			lts = -1
		}

		lrt = runeTokenType(r)
	}

	return -1
}

type RuneTokenType int

const (
	RTTNone RuneTokenType = iota
	RTTWord
	RTTLineSep
	RTTSpace
	RTTPunct
	RTTSymbol
	RTTOther
)

func runeTokenType(r rune) RuneTokenType {
	switch {
	case unicode.In(r, unicode.L, unicode.M, unicode.N):
		return RTTWord
	case unicode.In(r, unicode.Zl):
		return RTTLineSep
	case unicode.In(r, unicode.Z):
		return RTTSpace
	case unicode.In(r, unicode.P):
		return RTTPunct
	case unicode.In(r, unicode.S):
		return RTTSymbol
	}

	return RTTOther
}

func tokenType(t string) RuneTokenType {
	for _, r := range t {
		return runeTokenType(r)
	}

	return RTTNone
}

type DSChannel struct {
	Parent     *DiffStream
	ChannelNum int
	Token      strings.Builder
	TokenType  RuneTokenType
	Chunk      *DSChunk
	Pos        int
}

// Returns true if ch is at the end of its current chunk.
func (ch *DSChannel) EOC() bool {
	return ch.Pos >= len(ch.Chunk.String())
}

type DiffStream struct {
	lock     sync.RWMutex
	channels []DSChannel
	chunks   []*DSChunk
}

func New(cc int) (ds *DiffStream) {
	ds = &DiffStream{
		channels: make([]DSChannel, cc),
		chunks:   make([]*DSChunk, 0),
	}

	ck := ds.NewChunk("")

	for i := range ds.channels {
		ch := &ds.channels[i]
		ch.Parent = ds
		ch.ChannelNum = i
		ch.Chunk = ck
	}

	ds.chunks = append(ds.chunks, ck)

	return ds
}

// Allocate a new chunk appropriate for this stream.
func (ds *DiffStream) NewChunk(s string) (ck *DSChunk) {
	ck = &DSChunk{
		ChannelMask: NewBitSet(ds.ChannelCount()),
	}

	ck.Builder.WriteString(s)

	return ck
}

func (ds *DiffStream) SplitChunk(lck *DSChunk, pos int) (rck *DSChunk) {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	return ds.splitChunk(lck, pos)
}

func (ds *DiffStream) InsertChunkAfter(lck *DSChunk, nck *DSChunk) {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	ds.insertChunkAfter(lck, nck)
}

func (ds *DiffStream) findChunk(ck *DSChunk) int {
	return findElement(ds.chunks, ck)
}

func (ds *DiffStream) insertChunkAfter(lck *DSChunk, nck *DSChunk) {
	if lck == nil {
		ds.chunks = append([]*DSChunk{nck}, ds.chunks...)
	} else {
		ick := ds.findChunk(lck)
		assert(ick >= 0)
		ds.chunks = insertElement(ds.chunks, ick+1, nck)
	}
}

func (ds *DiffStream) MarshalJSON() (data []byte, err error) {
	ds.lock.RLock()
	defer ds.lock.RUnlock()

	type _DiffStream struct {
		Channels []DSChannel `json:"channels"`
		Chunks   []DSChunk   `json:"chunks"`
	}

	dso := _DiffStream{
		Channels: ds.channels,
		Chunks:   make([]DSChunk, len(ds.chunks)),
	}

	for i, ck := range ds.chunks {
		dso.Chunks[i] = *ck
	}

	return json.Marshal(dso)
}

func (ds *DiffStream) dumpChunks() string {
	var sb strings.Builder

	for _, ck := range ds.chunks {
		cch := filterSlice(ds.channels, func(i int, v DSChannel) bool { return v.Chunk == ck })

		lsc := "✓"
		if ck.locked {
			lsc = "╳"
		}
		sb.WriteString(fmt.Sprintf("[%s|%+v|", lsc, ck.ChannelMask))
		for i, r := range ck.String() {
			for _, ch := range cch {
				if ch.Pos == i {
					sb.WriteString(fmt.Sprintf("{%d}", ch.ChannelNum))
				}
			}
			sb.WriteRune(r)
		}
		for _, ch := range cch {
			if ch.Pos == len(ck.String()) {
				sb.WriteString(fmt.Sprintf("{%d}", ch.ChannelNum))
			}
		}
		sb.WriteString("]")
	}

	return sb.String()
}

func (ds *DiffStream) splitChunk(lck *DSChunk, pos int) (rck *DSChunk) {
	assert(pos < lck.Builder.Len())

	rck = &DSChunk{
		ChannelMask: lck.ChannelMask.Clone(),
	}
	rck.locked = lck.locked

	if pos < lck.Builder.Len() {
		rck.Builder.WriteString(lck.Builder.String()[pos:])
	}

	ls := lck.Builder.String()[:pos]
	lck.Builder.Reset()
	lck.Builder.WriteString(ls)

	// now, need to fixup all channel references
	// to this chunk
	ick := ds.findChunk(lck)
	if ick >= 0 {
		for i := range ds.channels {
			ch := &ds.channels[i]

			if ch.Chunk != lck {
				continue
			}

			// this channel refers to the split chunk

			// this channel refers to the right side of the
			// chunk to be split

			// first, exit the old chunk
			// if we've consumed any of it
			if ch.Pos > 0 {
				ch.Chunk.Exit(ch.ChannelNum)
			}

			// now, move to the right side chunk
			if ch.Pos >= pos {
				ch.Chunk = rck
				ch.Pos -= pos
			}
		}

		ds.chunks = insertElement(ds.chunks, ick+1, rck)
	}

	return rck
}

// Return number of channels for this stream.
func (ds *DiffStream) ChannelCount() int {
	return len(ds.channels)
}

// Return specified channel of this stream.
func (ds *DiffStream) Channel(i int) (ch *DSChannel) {
	if i < 0 || i >= ds.ChannelCount() {
		return nil
	}

	return &ds.channels[i]
}

func (ds *DiffStream) findNextToken(t string, ck *DSChunk, p int) (nck *DSChunk, np int) {
	var ick int

	if ck == nil {
		ick = 0
	} else {
		np = ck.findToken(t, p)
		if np >= 0 {
			return ck, np
		}

		ick = findElement(ds.chunks, ck) + 1
	}

	for _, nck = range ds.chunks[ick:] {
		np = nck.findToken(t, 0)

		if np >= 0 {
			return
		}
	}

	return nil, -1
}

func (ch *DSChannel) String() string {
	var sb strings.Builder

	for _, ck := range ch.Parent.chunks {
		if ck == ch.Chunk {
			sb.WriteString(ch.Chunk.String()[:ch.Pos])
		} else if ck.ChannelMask.Get(ch.ChannelNum) {
			sb.WriteString(ck.String())
		}
	}

	sb.WriteString(ch.Token.String())

	return sb.String()
}

func (ch *DSChannel) MarshalJSON() (data []byte, err error) {
	type _DSChannel struct {
		ChannelNum int    `json:"channel_num"`
		Token      string `json:"token"`
		ChunkNum   int    `json:"chunk_num"`
		Pos        int    `json:"pos"`
	}

	dco := _DSChannel{
		ChannelNum: ch.ChannelNum,
		Token:      ch.Token.String(),
		ChunkNum:   ch.Parent.findChunk(ch.Chunk),
		Pos:        ch.Pos,
	}

	return json.Marshal(dco)
}

func (ch *DSChannel) consumeRune(r rune) {
	assert(r != unicode.ReplacementChar)

	// empty token, append and return
	if ch.Token.Len() <= 0 {
		ch.TokenType = runeTokenType(r)
		ch.Token.WriteRune(r)

		return
	}

	// can we append this character to the current token
	if runeTokenType(r) == ch.TokenType {
		ch.Token.WriteRune(r)

		return
	}

	// we need to flush the current token
	ch.flushToken()

	// and recurse
	ch.consumeRune(r)
}

func (ch *DSChannel) flushToken() {
	ds := ch.Parent
	t := ch.Token.String()
	ch.Token.Reset()
	ch.TokenType = RTTNone

	nck, np := ch.Parent.findNextToken(t, ch.Chunk, ch.Pos)

	if nck == ch.Chunk {
		if np == ch.Pos {
			// just advance our position
			ch.Pos += len(t)

			return
		}

		assert(np > ch.Pos)
		// we need to exit & re-enter
		_, lck := ch.exitChunk()
		nck, np = ch.Parent.findNextToken(t, lck, 0)

		assert(np > 0)
		rck := ds.splitChunk(lck, np)
		ch.enterChunk(rck, t)

		return
	}

	if nck == nil {
		// we need to write the token
		if ch.canAppendToChunk() {
			ch.appendString(t)
		} else {
			// need a new chunk
			lck, _ := ch.exitChunk()
			nck = ds.NewChunk(t)
			ds.insertChunkAfter(lck, nck)
			ch.enterChunk(nck, t)
		}

		return
	}

	// need to enter a new chunk

	// first, exit our current chunk
	ch.exitChunk()

	if np == 0 {
		// token is at the start of the chunk
		ch.enterChunk(nck, t)

		return
	}

	rck := ds.splitChunk(nck, np)
	ch.enterChunk(rck, t)
}

func (ch *DSChannel) canAppendToChunk() bool {
	return !ch.Chunk.IsLocked() && ch.EOC()
}

func (ch *DSChannel) appendString(s string) {
	ch.Chunk.Builder.Write([]byte(s))
	ch.Pos += len(s)
}

// Safely closes out this channel's relationship with
// its current chunk.
func (ch *DSChannel) exitChunk() (lck *DSChunk, rck *DSChunk) {
	defer func() {
		ch.Chunk = nil
		ch.Pos = -1
	}()

	ds := ch.Parent
	lck = ch.Chunk

	if !ch.EOC() {
		// we're *not* at the end, so we need to split
		if ch.Pos > 0 {
			rck = ds.splitChunk(ch.Chunk, ch.Pos)
		} else {
			return
		}
	}

	lck.Exit(ch.ChannelNum)

	return
}

func (ch *DSChannel) enterChunk(ck *DSChunk, t string) {
	assert(ch.Chunk == nil)
	assert(ch.Pos == -1)
	assert(ck != nil)
	if strings.Index(ck.String(), t) != 0 {
		log.Errorf("enterChunk ck: '%v' t: '%v'", ck.String(), t)
	}
	assert(strings.Index(ck.String(), t) == 0)

	ch.Chunk = ck
	ch.Pos = len(t)
}

// Write a single rune to the given channel.
func (ds *DiffStream) WriteRune(ch *DSChannel, r rune) {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	ch.consumeRune(r)
}

// Write a string to the given channel.
func (ds *DiffStream) WriteString(ch *DSChannel, s string) {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	for _, r := range s {
		ch.consumeRune(r)
	}
}

// Write a single rune to the given channel.
func (ch *DSChannel) WriteRune(r rune) (n int, err error) {
	ch.Parent.WriteRune(ch, r)

	return
}

// Write a string rune-by-rune to the given channel.
func (ch *DSChannel) WriteString(s string) (n int, err error) {
	ch.Parent.WriteString(ch, s)

	return len(s), nil
}
