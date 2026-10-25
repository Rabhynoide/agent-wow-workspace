package roads

import (
	"encoding/binary"
	"fmt"
)

// Chunk is one MCNK of an ADT: its index and, per texture layer, the texture and its alpha map (64 × 64,
// 0-255, [row*64+col], rows along IY); layer 0 has no alpha map (it fills what the others leave).
type Chunk struct {
	IX, IY int
	Tex    []uint32
	Alpha  [][]byte
}

// ADT is the texturing of one terrain tile.
type ADT struct {
	Textures []string
	Chunks   []Chunk
}

// WDT reads a map's WDT: which tiles exist ([y*64+x] for <map>_x_y.adt) and whether alpha maps are
// 8-bit (MPHD flags 0x4 or 0x80).
func WDT(b []byte) (tiles [64 * 64]bool, bigAlpha bool, err error) {
	for o := 0; o+8 <= len(b); {
		id, size := string(rev(b[o:o+4])), int(binary.LittleEndian.Uint32(b[o+4:]))
		d := b[o+8 : min(o+8+size, len(b))]
		switch id {
		case "MPHD":
			if len(d) >= 4 {
				f := binary.LittleEndian.Uint32(d)
				bigAlpha = f&0x4 != 0 || f&0x80 != 0
			}
		case "MAIN":
			for i := 0; i < 64*64 && 8*i+4 <= len(d); i++ {
				tiles[i] = binary.LittleEndian.Uint32(d[8*i:])&1 != 0
			}
		}
		o += 8 + size
	}
	return
}

func rev(b []byte) []byte { return []byte{b[3], b[2], b[1], b[0]} }

// ParseADT reads the textures and alpha maps of a 3.3.5 ADT.
func ParseADT(b []byte, bigAlpha bool) (*ADT, error) {
	le := binary.LittleEndian
	a := &ADT{}
	for o := 0; o+8 <= len(b); {
		id, size := string(rev(b[o:o+4])), int(le.Uint32(b[o+4:]))
		if o+8+size > len(b) {
			return nil, fmt.Errorf("chunk %s past the end", id)
		}
		switch id {
		case "MTEX":
			d := b[o+8 : o+8+size]
			start := 0
			for i, c := range d {
				if c == 0 {
					if i > start {
						a.Textures = append(a.Textures, string(d[start:i]))
					}
					start = i + 1
				}
			}
		case "MCNK":
			c, err := parseMCNK(b[o:o+8+size], bigAlpha)
			if err != nil {
				return nil, err
			}
			a.Chunks = append(a.Chunks, c)
		}
		o += 8 + size
	}
	return a, nil
}

func parseMCNK(c []byte, bigAlpha bool) (Chunk, error) {
	le := binary.LittleEndian
	if len(c) < 8+128 {
		return Chunk{}, fmt.Errorf("short MCNK")
	}
	h := c[8:]
	flags := le.Uint32(h[0:])
	ch := Chunk{IX: int(le.Uint32(h[4:])), IY: int(le.Uint32(h[8:]))}
	n := int(le.Uint32(h[12:]))
	ofsLayer, ofsAlpha := int(le.Uint32(h[0x1C:])), int(le.Uint32(h[0x24:]))
	if n == 0 || ofsLayer+8+16*n > len(c) {
		return ch, nil
	}
	mcal := []byte(nil)
	if ofsAlpha > 0 && ofsAlpha+8 <= len(c) {
		size := int(le.Uint32(c[ofsAlpha+4:]))
		mcal = c[ofsAlpha+8 : min(ofsAlpha+8+size, len(c))]
	}
	for i := 0; i < n; i++ {
		l := c[ofsLayer+8+16*i:]
		tex, lflags, off := le.Uint32(l[0:]), le.Uint32(l[4:]), int(le.Uint32(l[8:]))
		ch.Tex = append(ch.Tex, tex)
		if i == 0 || lflags&0x100 == 0 || off >= len(mcal) {
			ch.Alpha = append(ch.Alpha, nil)
			continue
		}
		ch.Alpha = append(ch.Alpha, alphaMap(mcal[off:], lflags&0x200 != 0, bigAlpha, flags&0x8000 != 0))
	}
	return ch, nil
}

// alphaMap decodes one alpha map: compressed (RLE), 8-bit, or 4-bit (the last row and column copied
// from the previous ones unless the chunk says not to fix them).
func alphaMap(d []byte, compressed, big, noFix bool) []byte {
	out := make([]byte, 64*64)
	switch {
	case compressed:
		o, p := 0, 0
		for p < len(out) && o < len(d) {
			fill, count := d[o]&0x80 != 0, int(d[o]&0x7F)
			o++
			for k := 0; k < count && p < len(out); k++ {
				if o >= len(d) {
					break
				}
				out[p] = d[o]
				p++
				if !fill {
					o++
				}
			}
			if fill {
				o++
			}
		}
	case big:
		copy(out, d)
	default:
		for i := 0; i < 2048 && i < len(d); i++ {
			out[2*i] = (d[i] & 0x0F) * 17
			out[2*i+1] = (d[i] >> 4) * 17
		}
		if !noFix {
			for r := 0; r < 64; r++ {
				out[r*64+63] = out[r*64+62]
			}
			copy(out[63*64:], out[62*64:63*64])
		}
	}
	return out
}

// RoadCover is, for each of the 64 × 64 points of a chunk, how much road texture covers it (0-255).
func RoadCover(c Chunk, road []bool) []int {
	out := make([]int, 64*64)
	others := make([]int, 64*64)
	for l := 1; l < len(c.Tex); l++ {
		if c.Alpha[l] == nil {
			continue
		}
		isRoad := int(c.Tex[l]) < len(road) && road[c.Tex[l]]
		for p, v := range c.Alpha[l] {
			others[p] += int(v)
			if isRoad {
				out[p] += int(v)
			}
		}
	}
	if len(c.Tex) > 0 && int(c.Tex[0]) < len(road) && road[c.Tex[0]] {
		for p := range out {
			out[p] += max(0, 255-others[p])
		}
	}
	for p := range out {
		out[p] = min(out[p], 255)
	}
	return out
}
