package mft

import (
	"errors"
	"fmt"
	"os"
	"sort"
)

// Pair is one tag/value entry. Order is preserved so files round-trip.
type Pair struct{ Tag, Value byte }

type Pairs []Pair

func (ps Pairs) Get(tag byte) (int, bool) {
	for _, p := range ps {
		if p.Tag == tag {
			return int(p.Value), true
		}
	}
	return 0, false
}

func (ps *Pairs) Set(tag byte, v int) {
	for i, p := range *ps {
		if p.Tag == tag {
			(*ps)[i].Value = byte(v)
			return
		}
	}
	*ps = append(*ps, Pair{tag, byte(v)})
}

func (ps Pairs) Equal(o Pairs) bool {
	if len(ps) != len(o) {
		return false
	}
	for i := range ps {
		if ps[i] != o[i] {
			return false
		}
	}
	return true
}

func (ps Pairs) bytes() []byte {
	b := make([]byte, 0, 2*len(ps))
	for _, p := range ps {
		b = append(b, p.Tag, p.Value)
	}
	return b
}

func parsePairs(b []byte) (Pairs, error) {
	if len(b)%2 != 0 {
		return nil, errors.New("odd tag/value length")
	}
	ps := make(Pairs, 0, len(b)/2)
	for i := 0; i < len(b); i += 2 {
		ps = append(ps, Pair{b[i], b[i+1]})
	}
	return ps, nil
}

// Preset is a full device configuration: global table plus 64 or 128 encoder slots.
type Preset struct {
	Globals  Pairs
	Encoders map[int]Pairs   // slot 1..N -> pairs
	header   map[int][2]byte // slot -> (part, total) as found in a file
}

func NewPreset() *Preset {
	return &Preset{Encoders: map[int]Pairs{}, header: map[int][2]byte{}}
}

// Slots returns the number of encoder slots (64 = 4 banks, 128 = 8 banks).
func (p *Preset) Slots() int { return len(p.Encoders) }

func (p *Preset) Clone() *Preset {
	c := NewPreset()
	c.Globals = append(Pairs(nil), p.Globals...)
	for n, ps := range p.Encoders {
		c.Encoders[n] = append(Pairs(nil), ps...)
	}
	for n, h := range p.header {
		c.header[n] = h
	}
	return c
}

// ChangedSlots lists slots whose pairs differ from base (or are missing there).
func (p *Preset) ChangedSlots(base *Preset) []int {
	var out []int
	for n, ps := range p.Encoders {
		if b, ok := base.Encoders[n]; !ok || !ps.Equal(b) {
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out
}

// ParseMFS decodes a Midi Fighter Utility settings file.
//
//	global  := 0x00 LEN pair{LEN/2}
//	encoder := 0x00 SLOT PART TOTAL LEN pair{LEN/2}   (SLOT 1..128, raw byte)
func ParseMFS(d []byte) (*Preset, error) {
	if len(d) < 2 || d[0] != 0 {
		return nil, errors.New("not an .mfs file (expected 0x00 at offset 0)")
	}
	p := NewPreset()
	glen := int(d[1])
	if 2+glen > len(d) {
		return nil, errors.New("truncated global block")
	}
	var err error
	if p.Globals, err = parsePairs(d[2 : 2+glen]); err != nil {
		return nil, err
	}
	for i := 2 + glen; i < len(d); {
		if i+5 > len(d) || d[i] != 0 {
			return nil, fmt.Errorf("bad encoder record at offset %d", i)
		}
		slot, part, total, size := int(d[i+1]), d[i+2], d[i+3], int(d[i+4])
		if i+5+size > len(d) {
			return nil, fmt.Errorf("truncated encoder record at offset %d", i)
		}
		ps, err := parsePairs(d[i+5 : i+5+size])
		if err != nil {
			return nil, fmt.Errorf("slot %d: %w", slot, err)
		}
		p.Encoders[slot] = ps
		p.header[slot] = [2]byte{part, total}
		i += 5 + size
	}
	return p, nil
}

func (p *Preset) MFS() []byte {
	g := p.Globals.bytes()
	out := append([]byte{0, byte(len(g))}, g...)
	slots := make([]int, 0, len(p.Encoders))
	for n := range p.Encoders {
		slots = append(slots, n)
	}
	sort.Ints(slots)
	for _, n := range slots {
		body := p.Encoders[n].bytes()
		h, ok := p.header[n]
		if !ok {
			h = [2]byte{1, 0} // what the Utility writes
		}
		out = append(out, 0, byte(n), h[0], h[1], byte(len(body)))
		out = append(out, body...)
	}
	return out
}

func Load(path string) (*Preset, error) {
	d, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseMFS(d)
}

func (p *Preset) Save(path string) error { return os.WriteFile(path, p.MFS(), 0o644) }
