//go:build device

// Read-only checks against a connected Twister: go test -tags device ./internal/mft/
// MFT_BACKUP=path.mfs additionally compares the device with that file.
package mft

import (
	"os"
	"testing"
	"time"
)

func TestDeviceRead(t *testing.T) {
	d, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	info, err := d.Info()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("port %q firmware %s banks %d", d.Port, info.Firmware, info.Banks)
	start := time.Now()
	p, err := d.PullPreset(info.Banks*16, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("read %d slots in %v", p.Slots(), time.Since(start).Round(time.Millisecond))
	if path := os.Getenv("MFT_BACKUP"); path != "" {
		b, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if !p.Globals.Equal(b.Globals) {
			t.Errorf("globals differ from %s", path)
		}
		if ch := p.ChangedSlots(b); len(ch) > 0 {
			t.Errorf("slots differ from %s: %v", path, ch)
		}
	}
}
