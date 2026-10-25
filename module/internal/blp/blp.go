// Package blp decodes the BLP2 textures of the 3.3.5 client (palettized, DXT1/3/5, raw BGRA), first
// mip level only. Offline tools (cmd/minimap).
package blp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
)

// Decode returns the first mip level of a BLP2 file.
func Decode(b []byte) (*image.NRGBA, error) {
	if len(b) < 148 || string(b[:4]) != "BLP2" {
		return nil, errors.New("not a BLP2 file")
	}
	le := binary.LittleEndian
	comp, alphaDepth, alphaType := b[8], b[9], b[10]
	w, h := int(le.Uint32(b[12:])), int(le.Uint32(b[16:]))
	off, size := int(le.Uint32(b[20:])), int(le.Uint32(b[84:]))
	if off+size > len(b) || w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		return nil, errors.New("bad BLP mip 0")
	}
	d := b[off : off+size]
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	switch comp {
	case 1: // palette + alpha
		pal := b[148 : 148+1024]
		for i := 0; i < w*h && i < len(d); i++ {
			p := pal[4*int(d[i]):]
			a := byte(255)
			ai := w*h + i*int(alphaDepth)/8
			switch alphaDepth {
			case 8:
				if ai < len(d) {
					a = d[ai]
				}
			case 1:
				if w*h+i/8 < len(d) {
					a = 255 * (d[w*h+i/8] >> (i % 8) & 1)
				}
			case 4:
				if w*h+i/2 < len(d) {
					a = 17 * (d[w*h+i/2] >> (4 * (i % 2)) & 0xF)
				}
			}
			img.Pix[4*i], img.Pix[4*i+1], img.Pix[4*i+2], img.Pix[4*i+3] = p[2], p[1], p[0], a
		}
	case 2:
		blockSize := 16
		if alphaType == 0 {
			blockSize = 8
		}
		bw, bh := (w+3)/4, (h+3)/4
		if len(d) < bw*bh*blockSize {
			return nil, fmt.Errorf("short DXT data")
		}
		for by := 0; by < bh; by++ {
			for bx := 0; bx < bw; bx++ {
				blk := d[(by*bw+bx)*blockSize:]
				var px [16]color.NRGBA
				switch alphaType {
				case 0:
					colors(blk, &px, alphaDepth > 0)
				case 1:
					colors(blk[8:], &px, false)
					for i := 0; i < 16; i++ {
						px[i].A = 17 * (blk[i/2] >> (4 * (i % 2)) & 0xF)
					}
				default:
					colors(blk[8:], &px, false)
					dxt5Alpha(blk, &px)
				}
				for i := 0; i < 16; i++ {
					x, y := bx*4+i%4, by*4+i/4
					if x < w && y < h {
						img.SetNRGBA(x, y, px[i])
					}
				}
			}
		}
	case 3:
		for i := 0; i < w*h && 4*i+3 < len(d); i++ {
			img.Pix[4*i], img.Pix[4*i+1], img.Pix[4*i+2], img.Pix[4*i+3] = d[4*i+2], d[4*i+1], d[4*i], d[4*i+3]
		}
	default:
		return nil, fmt.Errorf("BLP compression %d not supported", comp)
	}
	return img, nil
}

func rgb565(c uint16) color.NRGBA {
	r, g, b := byte(c>>11&31), byte(c>>5&63), byte(c&31)
	return color.NRGBA{r<<3 | r>>2, g<<2 | g>>4, b<<3 | b>>2, 255}
}

// colors decodes the colour part of a DXT block (DXT1 with 1-bit alpha when punch).
func colors(blk []byte, px *[16]color.NRGBA, punch bool) {
	c0, c1 := binary.LittleEndian.Uint16(blk), binary.LittleEndian.Uint16(blk[2:])
	a, b := rgb565(c0), rgb565(c1)
	var pal [4]color.NRGBA
	pal[0], pal[1] = a, b
	mix := func(x, y byte, wx, wy, t int) byte { return byte((int(x)*wx + int(y)*wy) / t) }
	if c0 > c1 || !punch {
		pal[2] = color.NRGBA{mix(a.R, b.R, 2, 1, 3), mix(a.G, b.G, 2, 1, 3), mix(a.B, b.B, 2, 1, 3), 255}
		pal[3] = color.NRGBA{mix(a.R, b.R, 1, 2, 3), mix(a.G, b.G, 1, 2, 3), mix(a.B, b.B, 1, 2, 3), 255}
	} else {
		pal[2] = color.NRGBA{mix(a.R, b.R, 1, 1, 2), mix(a.G, b.G, 1, 1, 2), mix(a.B, b.B, 1, 1, 2), 255}
		pal[3] = color.NRGBA{0, 0, 0, 0}
	}
	bits := binary.LittleEndian.Uint32(blk[4:])
	for i := 0; i < 16; i++ {
		px[i] = pal[bits>>(2*i)&3]
	}
}

func dxt5Alpha(blk []byte, px *[16]color.NRGBA) {
	a0, a1 := int(blk[0]), int(blk[1])
	var pal [8]int
	pal[0], pal[1] = a0, a1
	if a0 > a1 {
		for i := 1; i < 7; i++ {
			pal[i+1] = ((7-i)*a0 + i*a1) / 7
		}
	} else {
		for i := 1; i < 5; i++ {
			pal[i+1] = ((5-i)*a0 + i*a1) / 5
		}
		pal[6], pal[7] = 0, 255
	}
	var bits uint64
	for i := 0; i < 6; i++ {
		bits |= uint64(blk[2+i]) << (8 * i)
	}
	for i := 0; i < 16; i++ {
		px[i].A = byte(pal[bits>>(3*i)&7])
	}
}
