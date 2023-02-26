//
// copyright 2023 Scott Means Consulting, LLC DBA CloudTerm Partners
//

// Package diffstream provides support for generating
// real-time text diffs from multiple input streams.
package diffstream

import (
	"fmt"
	"strings"
	"sync"

	"github.com/boljen/go-bitmap"
	"github.com/labstack/gommon/log"
)

type DSChunk struct {
	ChannelMask bitmap.Bitmap
	Builder     strings.Builder
}

func (ck *DSChunk) String() string {
	return ck.Builder.String()
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
	log.Debugf("NewChunk: %v", ds.dumpChunks())

	ck = &DSChunk{
		ChannelMask: bitmap.New(ds.ChannelCount()),
	}

	ck.Builder.WriteString(s)

	return ck
}

func (ds *DiffStream) SplitChunk(lck *DSChunk, pos int) (rck *DSChunk) {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	return ds.splitChunk(lck, pos, nil)
}

func (ds *DiffStream) dumpChunks() string {
	var sb strings.Builder

	for _, ck := range ds.chunks {
		sb.WriteString(fmt.Sprintf("[%v]", ck))
	}

	return sb.String()
}

func (ds *DiffStream) splitChunk(lck *DSChunk, pos int, iss []*DSChunk) (rck *DSChunk) {
	log.Debugf("splitting chunk %v: pos: %v iss: %v", lck, pos, iss)
	rck = &DSChunk{
		ChannelMask: bitmap.New(ds.ChannelCount()),
	}
	copy(rck.ChannelMask.Data(false), lck.ChannelMask.Data(false))

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

			if ch.Chunk != lck || ch.Pos < pos {
				continue
			}

			// this channel refers to the right side of the
			// chunk to be split
			ch.Chunk = rck
			ch.Pos -= pos
		}

		ncs := ds.chunks[:ick+1]
		if iss != nil {
			ncs = append(ncs, iss...)
		}
		ncs = append(ncs, rck)
		ncs = append(ncs, ds.chunks[ick+1:]...)
		log.Debugf("pre split chunks: %+v\npost split chunks: %+v", ds.chunks, ncs)
		ds.chunks = ncs
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
		if ck.ChannelMask.Get(ch.ChannelNum) {
			sb.WriteString(ck.String())
		}
	}

	sb.WriteString(ch.Chunk.String()[:ch.Pos])

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
		cb, _ = ch.Chunk.Builder.WriteRune(r)
		ch.Pos += cb

		return
	}

	log.Debugf("ch %d: %v", ch.ChannelNum, ds.dumpChunks())
	log.Debugf("ch %d: not-happy %v", ch.ChannelNum, ch)

	if nrck != nil {
		// there's a matching downstream rune
		log.Debugf("matching downstream: %v (%v)", nrck, nrp)

		ch.exitChunk(nil)

		if nrp == 0 {
			// the match is at the start of the chunk
			// we just need to exit the current chunk
			// and set our current chunk pointer to the
			// start of the new chunk

			log.Debugf("hit nrp == 0 case")
			ch.Chunk = nrck
		} else {
			ch.Chunk = ds.splitChunk(nrck, nrp, nil)
		}
		ch.Pos = 0

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
	return ch.EOC()
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
	lck = ch.Chunk

	if !ch.EOC() {
		// we're *not* at the end, so we need to split
		rck = ch.Parent.splitChunk(ch.Chunk, ch.Pos, iss)
	}

	lck.ChannelMask.Set(ch.ChannelNum, true)

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

// Write a single rune to the given channel.
func (ch *DSChannel) WriteRune(r rune) (n int, err error) {
	ch.Parent.WriteRune(ch, r)

	return
}

// Write a string rune-by-rune to the given channel.
func (ch *DSChannel) WriteString(s string) (n int, err error) {
	for _, r := range s {
		ch.Parent.WriteRune(ch, r)
	}

	return len(s), nil
}
