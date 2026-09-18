package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/biomassa/mft-tui/internal/mft"
)

func TestParseAndFormatKnobs(t *testing.T) {
	for _, in := range []string{"1 3 5-8 17", "1,3,5-8,17", "1, 3, 8-5 ,17", "17 5 6 7 8 3 1 3"} {
		got, err := parseKnobs(in, 128)
		if err != nil || !reflect.DeepEqual(got, []int{1, 3, 5, 6, 7, 8, 17}) {
			t.Errorf("%q -> %v %v", in, got, err)
		}
	}
	if got := formatKnobs([]int{1, 3, 5, 6, 7, 8, 17}); got != "1, 3, 5-8, 17" {
		t.Errorf("format %q", got)
	}
	for _, bad := range []string{"0", "129", "3-x", "a"} {
		if _, err := parseKnobs(bad, 128); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// Pick 3 knobs across banks with s, apply a CC sequence in knob order.
func TestPickAcrossBanksAndSequence(t *testing.T) {
	m := load(t, "8bank.mfs")
	m = press(m, tea.KeyRight, tea.KeyRight, "s") // knob 3
	m = press(m, "2", "s")                        // bank 2: cursor 19
	m = press(m, "1", tea.KeyLeft, tea.KeyLeft, "s")
	if !m.pickMode || !reflect.DeepEqual(m.picks, []int{1, 3, 19}) {
		t.Fatalf("picks %v mode %v", m.picks, m.pickMode)
	}
	if !strings.Contains(m.View(), "Knobs:") || !strings.Contains(m.bankBar(), "2•") {
		t.Fatalf("pick UI missing:\n%s", m.bankBar())
	}
	m.rows[0].mode = seq
	m.rows[0].val.SetValue("40")
	m.rows[0].step.SetValue("2")
	m.apply()
	for n, want := range map[int]int{1: 40, 3: 42, 19: 44} {
		if got := get(m.preset, n, 17); got != want {
			t.Errorf("knob %d CC %d, want %d (%s)", n, got, want, m.status)
		}
	}
	if len(m.changed()) != 3 {
		t.Errorf("changed %v", m.changed())
	}
	// s on a picked knob removes it.
	m = press(m, "s")
	if !reflect.DeepEqual(m.picks, []int{3, 19}) {
		t.Errorf("after unpick: %v", m.picks)
	}
}

func TestTypedKnobList(t *testing.T) {
	m := load(t, "8bank.mfs")
	m = press(m, "s")                // enter pick mode with knob 1
	m = press(m, tea.KeyTab)         // Knobs field
	m = press(m, tea.KeyCtrlU)       // clear
	m = typeStr(m, "2, 4 10-12 100") // mixed separators
	if !reflect.DeepEqual(m.picks, []int{2, 4, 10, 11, 12, 100}) {
		t.Fatalf("picks %v (status %q)", m.picks, m.status)
	}
	m = typeStr(m, "0") // "1000" is out of range: status, picks unchanged
	if !strings.Contains(m.status, "outside") || len(m.picks) != 6 {
		t.Fatalf("status %q picks %v", m.status, m.picks)
	}
	if got := m.now(mft.EncoderFields[rowOf(t, "Encoder MIDI Number")]); got == "" {
		t.Error("now empty")
	}
}

func TestLeavePickMode(t *testing.T) {
	m := load(t, "4bank.mfs")
	m = press(m, "s", tea.KeyRight, "s")
	m = press(m, tea.KeyShiftRight) // leaves pick mode, range 2..3
	if m.pickMode {
		t.Fatal("still picking after shift+arrow")
	}
	if a, b := m.rangeBounds(); a != 2 || b != 3 {
		t.Fatalf("range %d–%d", a, b)
	}
	m = press(m, "s", tea.KeyEscape)
	if m.pickMode {
		t.Fatal("esc did not leave pick mode")
	}
}
