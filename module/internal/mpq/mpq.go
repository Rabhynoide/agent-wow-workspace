// Package mpq reads files from the MPQ archives of the 3.3.5 client (format versions 1 and 2: hash and
// block tables, the high block table of archives over 4 GB, zlib and bzip2 sectors, encrypted files).
// Offline tools only (cmd/roads): the game client's own data, read as the client does.
package mpq

import (
	"bytes"
	"compress/bzip2"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var cryptTable [0x500]uint32

func init() {
	seed := uint32(0x00100001)
	for i := 0; i < 0x100; i++ {
		for j, idx := 0, i; j < 5; j, idx = j+1, idx+0x100 {
			seed = (seed*125 + 3) % 0x2AAAAB
			t1 := (seed & 0xFFFF) << 16
			seed = (seed*125 + 3) % 0x2AAAAB
			cryptTable[idx] = t1 | (seed & 0xFFFF)
		}
	}
}

// hash is HashString of the MPQ format (names upper-cased, '/' as '\').
func hash(s string, typ uint32) uint32 {
	s1, s2 := uint32(0x7FED7FED), uint32(0xEEEEEEEE)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		if c == '/' {
			c = '\\'
		}
		s1 = cryptTable[typ*0x100+uint32(c)] ^ (s1 + s2)
		s2 = uint32(c) + s1 + s2 + (s2 << 5) + 3
	}
	return s1
}

func decrypt(data []uint32, key uint32) {
	seed := uint32(0xEEEEEEEE)
	for i := range data {
		seed += cryptTable[0x400+(key&0xFF)]
		ch := data[i] ^ (key + seed)
		key = ((^key << 0x15) + 0x11111111) | (key >> 0x0B)
		seed = ch + seed + (seed << 5) + 3
		data[i] = ch
	}
}

const (
	flagImplode    = 0x00000100
	flagCompress   = 0x00000200
	flagEncrypted  = 0x00010000
	flagFixKey     = 0x00020000
	flagSingleUnit = 0x01000000
	flagDeleted    = 0x02000000
	flagSectorCRC  = 0x04000000
	flagExists     = 0x80000000
)

type hashEntry struct {
	name1, name2 uint32
	locale       uint16
	block        uint32
}

type blockEntry struct {
	offset      int64
	csize, size uint32
	flags       uint32
}

// Archive is one open MPQ file.
type Archive struct {
	f          *os.File
	base       int64 // archive start in the file
	sectorSize uint32
	hashes     []hashEntry
	blocks     []blockEntry
}

func Open(path string) (*Archive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	a := &Archive{f: f}
	if err := a.readHeader(); err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return a, nil
}

func (a *Archive) Close() error { return a.f.Close() }

func (a *Archive) readHeader() error {
	buf := make([]byte, 44)
	for off := int64(0); ; off += 512 {
		if _, err := a.f.ReadAt(buf, off); err != nil {
			return errors.New("no MPQ header")
		}
		if string(buf[:4]) == "MPQ\x1a" {
			a.base = off
			break
		}
		if off > 1<<24 {
			return errors.New("no MPQ header")
		}
	}
	le := binary.LittleEndian
	version := le.Uint16(buf[12:])
	a.sectorSize = 512 << le.Uint16(buf[14:])
	hashPos, blockPos := int64(le.Uint32(buf[16:])), int64(le.Uint32(buf[20:]))
	hashN, blockN := le.Uint32(buf[24:]), le.Uint32(buf[28:])
	var hiBlockPos int64
	if version >= 1 {
		hiBlockPos = int64(le.Uint64(buf[32:]))
		hashPos |= int64(le.Uint16(buf[40:])) << 32
		blockPos |= int64(le.Uint16(buf[42:])) << 32
	}
	ht, err := a.table(hashPos, hashN, hash("(hash table)", 3))
	if err != nil {
		return err
	}
	for i := 0; i < int(hashN); i++ {
		a.hashes = append(a.hashes, hashEntry{ht[i*4], ht[i*4+1], uint16(ht[i*4+2]), ht[i*4+3]})
	}
	bt, err := a.table(blockPos, blockN, hash("(block table)", 3))
	if err != nil {
		return err
	}
	var hi []uint16
	if hiBlockPos != 0 {
		raw := make([]byte, 2*blockN)
		if _, err := a.f.ReadAt(raw, a.base+hiBlockPos); err != nil {
			return err
		}
		for i := range blockN {
			hi = append(hi, le.Uint16(raw[2*i:]))
		}
	}
	for i := 0; i < int(blockN); i++ {
		b := blockEntry{offset: int64(bt[i*4]), csize: bt[i*4+1], size: bt[i*4+2], flags: bt[i*4+3]}
		if hi != nil {
			b.offset |= int64(hi[i]) << 32
		}
		a.blocks = append(a.blocks, b)
	}
	return nil
}

func (a *Archive) table(pos int64, n, key uint32) ([]uint32, error) {
	raw := make([]byte, 16*n)
	if _, err := a.f.ReadAt(raw, a.base+pos); err != nil {
		return nil, err
	}
	out := make([]uint32, 4*n)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(raw[4*i:])
	}
	decrypt(out, key)
	return out, nil
}

func (a *Archive) find(name string) (blockEntry, bool) {
	n := uint32(len(a.hashes))
	if n == 0 {
		return blockEntry{}, false
	}
	h0, h1, h2 := hash(name, 0), hash(name, 1), hash(name, 2)
	for i, start := h0%n, h0%n; ; {
		e := a.hashes[i]
		if e.block == 0xFFFFFFFF {
			return blockEntry{}, false
		}
		if e.name1 == h1 && e.name2 == h2 && e.block != 0xFFFFFFFE && int(e.block) < len(a.blocks) {
			b := a.blocks[e.block]
			if b.flags&flagExists != 0 && b.flags&flagDeleted == 0 {
				return b, true
			}
		}
		if i = (i + 1) % n; i == start {
			return blockEntry{}, false
		}
	}
}

// Has tells whether the archive holds the file.
func (a *Archive) Has(name string) bool {
	_, ok := a.find(name)
	return ok
}

// Read returns a file of the archive.
func (a *Archive) Read(name string) ([]byte, error) {
	b, ok := a.find(name)
	if !ok {
		return nil, os.ErrNotExist
	}
	if b.flags&flagImplode != 0 {
		return nil, fmt.Errorf("%s: imploded files not supported", name)
	}
	raw := make([]byte, b.csize)
	if _, err := a.f.ReadAt(raw, a.base+b.offset); err != nil {
		return nil, err
	}
	var key uint32
	if b.flags&flagEncrypted != 0 {
		base := name
		if i := strings.LastIndexAny(base, `\/`); i >= 0 {
			base = base[i+1:]
		}
		key = hash(base, 3)
		if b.flags&flagFixKey != 0 {
			key = (key + uint32(b.offset)) ^ b.size
		}
	}
	if b.flags&flagSingleUnit != 0 {
		if b.flags&flagEncrypted != 0 {
			decryptBytes(raw, key)
		}
		if b.flags&flagCompress != 0 && b.csize < b.size {
			return decompress(raw, b.size)
		}
		return raw[:b.size], nil
	}
	nsec := (b.size + a.sectorSize - 1) / a.sectorSize
	if b.flags&flagCompress == 0 {
		// Plain sectors (encrypted one by one).
		if b.flags&flagEncrypted != 0 {
			for s := uint32(0); s < nsec; s++ {
				lo, hi := s*a.sectorSize, min((s+1)*a.sectorSize, b.size)
				decryptBytes(raw[lo:hi], key+s)
			}
		}
		return raw[:b.size], nil
	}
	ntab := nsec + 1
	if b.flags&flagSectorCRC != 0 {
		ntab++
	}
	if len(raw) < int(4*ntab) {
		return nil, fmt.Errorf("%s: short sector table", name)
	}
	offs := make([]uint32, ntab)
	for i := range offs {
		offs[i] = binary.LittleEndian.Uint32(raw[4*i:])
	}
	if b.flags&flagEncrypted != 0 {
		decrypt(offs, key-1)
	}
	out := make([]byte, 0, b.size)
	for s := uint32(0); s < nsec; s++ {
		lo, hi := offs[s], offs[s+1]
		if hi > uint32(len(raw)) || lo > hi {
			return nil, fmt.Errorf("%s: bad sector %d", name, s)
		}
		sec := append([]byte(nil), raw[lo:hi]...)
		if b.flags&flagEncrypted != 0 {
			decryptBytes(sec, key+s)
		}
		want := min(a.sectorSize, b.size-s*a.sectorSize)
		if uint32(len(sec)) < want {
			d, err := decompress(sec, want)
			if err != nil {
				return nil, fmt.Errorf("%s: sector %d: %w", name, s, err)
			}
			sec = d
		}
		out = append(out, sec...)
	}
	return out, nil
}

func decryptBytes(b []byte, key uint32) {
	n := len(b) / 4
	w := make([]uint32, n)
	for i := range w {
		w[i] = binary.LittleEndian.Uint32(b[4*i:])
	}
	decrypt(w, key)
	for i := range w {
		binary.LittleEndian.PutUint32(b[4*i:], w[i])
	}
}

// decompress: first byte = compression mask (0x02 zlib, 0x10 bzip2; bzip2 is undone first).
func decompress(b []byte, size uint32) ([]byte, error) {
	if len(b) == 0 {
		return nil, errors.New("empty sector")
	}
	mask, data := b[0], b[1:]
	if mask&^0x12 != 0 {
		return nil, fmt.Errorf("compression 0x%02x not supported", mask)
	}
	if mask&0x10 != 0 {
		d, err := io.ReadAll(bzip2.NewReader(bytes.NewReader(data)))
		if err != nil {
			return nil, err
		}
		data = d
	}
	if mask&0x02 != 0 {
		r, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		d, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		data = d
	}
	return data, nil
}

// Set is the client's archives in load order: a file is read from the last archive that has it
// (patch-3 over patch-2 over patch over the base archives).
type Set struct{ archives []*Archive }

// OpenClient opens the archives of a client Data directory (base, then patches, then the locale ones
// of each sub-directory, like the 3.3.5 client).
func OpenClient(dir string) (*Set, error) {
	order := []string{"common.MPQ", "common-2.MPQ", "expansion.MPQ", "lichking.MPQ", "patch.MPQ", "patch-2.MPQ", "patch-3.MPQ"}
	s := &Set{}
	for _, n := range order {
		p := filepath.Join(dir, n)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		a, err := Open(p)
		if err != nil {
			return nil, err
		}
		s.archives = append(s.archives, a)
	}
	locs, _ := filepath.Glob(filepath.Join(dir, "*", "*.MPQ"))
	sort.Slice(locs, func(i, j int) bool { return localeRank(locs[i]) < localeRank(locs[j]) })
	for _, p := range locs {
		a, err := Open(p)
		if err != nil {
			return nil, err
		}
		s.archives = append(s.archives, a)
	}
	if len(s.archives) == 0 {
		return nil, fmt.Errorf("no MPQ in %s", dir)
	}
	return s, nil
}

// localeRank: locale, expansion-locale, lichking-locale, then patch-xxXX, -2, -3.
func localeRank(p string) int {
	b := strings.ToLower(filepath.Base(p))
	switch {
	case strings.HasPrefix(b, "locale-"):
		return 0
	case strings.HasPrefix(b, "expansion-locale"):
		return 1
	case strings.HasPrefix(b, "lichking-locale"):
		return 2
	case strings.HasPrefix(b, "patch-") && strings.Contains(b, "-3"):
		return 6
	case strings.HasPrefix(b, "patch-") && strings.Contains(b, "-2"):
		return 5
	case strings.HasPrefix(b, "patch-"):
		return 4
	}
	return 3
}

func (s *Set) Read(name string) ([]byte, error) {
	for i := len(s.archives) - 1; i >= 0; i-- {
		if s.archives[i].Has(name) {
			return s.archives[i].Read(name)
		}
	}
	return nil, os.ErrNotExist
}

func (s *Set) Close() {
	for _, a := range s.archives {
		a.Close()
	}
}
