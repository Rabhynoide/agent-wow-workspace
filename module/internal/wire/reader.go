// Package wire decodes little-endian WoW 3.3.5 packet payloads.
package wire

import (
	"encoding/binary"
	"math"
)

// Reader reads from a payload; after any short read Err is true and reads return zero values.
type Reader struct {
	B   []byte
	I   int
	Err bool
}

func NewReader(b []byte) *Reader { return &Reader{B: b} }

func (r *Reader) take(n int) []byte {
	if r.Err || n < 0 || r.I+n > len(r.B) {
		r.Err = true
		return nil
	}
	s := r.B[r.I : r.I+n]
	r.I += n
	return s
}

func (r *Reader) Skip(n int) { r.take(n) }
func (r *Reader) Left() int  { return len(r.B) - r.I }

func (r *Reader) U8() uint8 {
	if s := r.take(1); s != nil {
		return s[0]
	}
	return 0
}

func (r *Reader) U16() uint16 {
	if s := r.take(2); s != nil {
		return binary.LittleEndian.Uint16(s)
	}
	return 0
}

func (r *Reader) U32() uint32 {
	if s := r.take(4); s != nil {
		return binary.LittleEndian.Uint32(s)
	}
	return 0
}

func (r *Reader) U64() uint64 {
	if s := r.take(8); s != nil {
		return binary.LittleEndian.Uint64(s)
	}
	return 0
}

func (r *Reader) F32() float32 { return math.Float32frombits(r.U32()) }

// PackedGUID reads a mask byte followed by the non-zero bytes of a GUID.
func (r *Reader) PackedGUID() uint64 {
	mask := r.U8()
	var g uint64
	for i := 0; i < 8; i++ {
		if mask&(1<<i) != 0 {
			g |= uint64(r.U8()) << (8 * i)
		}
	}
	return g
}

// Writer builds client packets.
type Writer struct{ B []byte }

func (w *Writer) U8(v uint8)    { w.B = append(w.B, v) }
func (w *Writer) U16(v uint16)  { w.B = binary.LittleEndian.AppendUint16(w.B, v) }
func (w *Writer) U32(v uint32)  { w.B = binary.LittleEndian.AppendUint32(w.B, v) }
func (w *Writer) U64(v uint64)  { w.B = binary.LittleEndian.AppendUint64(w.B, v) }
func (w *Writer) F32(v float32) { w.U32(math.Float32bits(v)) }

func (w *Writer) PackedGUID(g uint64) {
	var mask uint8
	var data []byte
	for i := 0; i < 8; i++ {
		if b := uint8(g >> (8 * i)); b != 0 {
			mask |= 1 << i
			data = append(data, b)
		}
	}
	w.U8(mask)
	w.B = append(w.B, data...)
}
