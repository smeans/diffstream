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

func (ck *DSChunk) PeekRune(p int) (cb int, r rune) {
	s := ck.Builder.String()

	var i int
	var nr rune

	for i, nr = range s[p:] {
		if r == 0 {
			r = nr
		} else {
			break
		}
	}

	if i > 0 {
		return i, r
	} else {
		return len(s) - p, r
	}
}

func (ck *DSChunk) IsLocked() bool {
	return ck.locked
}

func (ck *DSChunk) Exit(c int) {
	ck.ChannelMask.Set(c)
	ck.locked = true
}

func (ck *DSChunk) WriteRune(r rune) (n int, err error) {
	assert(!ck.IsLocked())
	return ck.Builder.WriteRune(r)
}

func (ck *DSChunk) WriteString(s string) (n int, err error) {
	assert(!ck.IsLocked())
	return ck.Builder.WriteString(s)
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

type DSChannel struct {
	Parent     *DiffStream
	ChannelNum int
	Chunk      *DSChunk
	Pos        int
}

// Returns true if ch is at the end of its current chunk.
func (ch *DSChannel) EOC() bool {
	return ch.Pos >= ch.Chunk.Builder.Len()
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

	return ds.splitChunk(lck, pos, nil)
}

func (ds *DiffStream) MarshalJSON() (data []byte, err error) {
	ds.lock.RLock()
	defer ds.lock.RUnlock()

	type _DiffStream struct {
		ChannelCount int       `json:"channel_count"`
		Chunks       []DSChunk `json:"chunks"`
	}

	dso := _DiffStream{
		ChannelCount: len(ds.channels),
		Chunks:       make([]DSChunk, len(ds.chunks)),
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

func (ds *DiffStream) splitChunk(lck *DSChunk, pos int, iss []*DSChunk) (rck *DSChunk) {
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
	ick := findElement(ds.chunks, lck)
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

		if iss != nil {
			ds.chunks = insertSlice(ds.chunks, ick+1, append(iss, rck))
		} else {
			ds.chunks = insertElement(ds.chunks, ick+1, rck)
		}
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

func (ch *DSChannel) String() string {
	var sb strings.Builder

	for _, ck := range ch.Parent.chunks {
		if ck == ch.Chunk {
			sb.WriteString(ch.Chunk.String()[:ch.Pos])
		} else if ck.ChannelMask.Get(ch.ChannelNum) {
			sb.WriteString(ck.String())
		}
	}

	return sb.String()
}

func (ch *DSChannel) consumeRune(r rune) {
	ds := ch.Parent
	// if r appears at current channel position
	cb, nr := ch.Chunk.PeekRune(ch.Pos)

	if cb > 0 && r == nr {
		// happy path, content matches current chunk
		ch.Pos += cb

		return
	}

	// either the current rune doesn't match or
	// we're at the end of our current chunk

	// either way, we need to see if there is a matching
	// rune further downstream

	nrck, nrp := ch.findNextRunePos(r)

	if nrck == nil && ch.canAppendToChunk() {
		// no downstream matches, and we can write
		cb, _ = ch.Chunk.WriteRune(r)
		ch.Pos += cb

		return
	}

	if nrck != nil {
		// there's a matching downstream rune in a chunk
		// which might actually be this chunk

		lck, rck := ch.exitChunk(nil)

		if nrck == lck {
			ch.Chunk = rck
			ch.Pos = 0
			nrck, nrp = ch.findNextRunePos(r)
			ch.Chunk = nil
			ch.Pos = -1
		}

		if nrp == 0 {
			// the match is at the start of the chunk
			ch.Chunk = nrck
		} else {
			ch.Chunk = ds.splitChunk(nrck, nrp, nil)
		}

		cb, nr = ch.Chunk.PeekRune(0)

		assert(nr == r)

		ch.Pos = cb

		return
	}

	// no downstream matches and we can't write
	// to the current chunk, so we need to
	// exit the current chunk while inserting
	// a new chunk between the old chunk
	// and the new right-hand chunk
	iss := []*DSChunk{ds.NewChunk(string(r))}
	ch.exitChunk(iss)
	ch.Chunk = iss[0]
	ch.Pos = ch.Chunk.Builder.Len()
}

func (ch *DSChannel) canAppendToChunk() bool {
	return !ch.Chunk.IsLocked() && ch.EOC()
}

func (ch *DSChannel) findNextRunePos(r rune) (nrck *DSChunk, nrp int) {
	ds := ch.Parent

	// check the current chunk first
	nrp = strings.IndexRune(ch.Chunk.String()[ch.Pos:], r)
	if nrp >= 0 {
		return ch.Chunk, nrp
	}

	cp := findElement(ds.chunks, ch.Chunk)
	for _, nrck := range ds.chunks[cp+1:] {
		nrp = strings.IndexRune(nrck.Builder.String(), r)
		if nrp >= 0 {
			return nrck, nrp
		}
	}

	return nil, -1
}

// Safely closes out this channel's relationship with
// its current chunk.
func (ch *DSChannel) exitChunk(iss []*DSChunk) (lck *DSChunk, rck *DSChunk) {
	ds := ch.Parent
	lck = ch.Chunk

	if !ch.EOC() {
		// we're *not* at the end, so we need to split
		rck = ds.splitChunk(ch.Chunk, ch.Pos, iss)
	} else if iss != nil {
		// we're at the end, but need to insert this chunk
		ick := findElement(ds.chunks, lck)
		ds.chunks = insertSlice(ds.chunks, ick+1, iss)
	}

	lck.Exit(ch.ChannelNum)

	ch.Chunk = nil
	ch.Pos = -1

	return
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
