package mft

import (
	"bytes"
	"os"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		file  string
		slots int
	}{{"4bank.mfs", 64}, {"8bank.mfs", 128}, {"legacy.mfs", 64}} {
		d, err := os.ReadFile("testdata/" + tc.file)
		if err != nil {
			t.Fatal(err)
		}
		p, err := ParseMFS(d)
		if err != nil {
			t.Fatalf("%s: %v", tc.file, err)
		}
		if p.Slots() != tc.slots {
			t.Errorf("%s: %d slots, want %d", tc.file, p.Slots(), tc.slots)
		}
		if !bytes.Equal(p.MFS(), d) {
			t.Errorf("%s: rebuilt file differs", tc.file)
		}
	}
}

func TestSetAndDiff(t *testing.T) {
	p, err := Load("testdata/4bank.mfs")
	if err != nil {
		t.Fatal(err)
	}
	base := p.Clone()
	e := p.Encoders[5]
	e.Set(17, 99)
	p.Encoders[5] = e
	if got := p.ChangedSlots(base); len(got) != 1 || got[0] != 5 {
		t.Fatalf("changed = %v, want [5]", got)
	}
	if v, _ := base.Encoders[5].Get(17); v == 99 {
		t.Fatal("Clone shares encoder storage")
	}
}

func TestPushSplit(t *testing.T) {
	p, _ := Load("testdata/8bank.mfs")
	msgs := msgsPushEncoder(128, p.Encoders[128])
	if len(msgs) != 2 {
		t.Fatalf("%d parts, want 2", len(msgs))
	}
	// 00 01 79 04 | 00 tag part total size
	if h := msgs[0][4:9]; !bytes.Equal(h, []byte{0, 0, 1, 2, 24}) {
		t.Errorf("part 1 header % x", h)
	}
	if h := msgs[1][4:9]; !bytes.Equal(h, []byte{0, 0, 2, 2, 6}) {
		t.Errorf("part 2 header % x", h)
	}
}
