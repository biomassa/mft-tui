package mft

import (
	"bytes"
	"os"
	"testing"
)

// fixture reads a preset from testdata. The presets are the author's own
// device dumps and are not in the repository; tests skip without them.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	d, err := os.ReadFile("testdata/" + name)
	if os.IsNotExist(err) {
		t.Skipf("testdata/%s not present (presets are not in the repository)", name)
	}
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func fixturePreset(t *testing.T, name string) *Preset {
	t.Helper()
	p, err := ParseMFS(fixture(t, name))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		file  string
		slots int
	}{{"4bank.mfs", 64}, {"8bank.mfs", 128}, {"legacy.mfs", 64}} {
		d := fixture(t, tc.file)
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
	p := fixturePreset(t, "4bank.mfs")
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
	p := fixturePreset(t, "8bank.mfs")
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
