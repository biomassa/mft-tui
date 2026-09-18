package ui

import (
	"os"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/biomassa/mft-tui/internal/mft"
)

func load(t *testing.T, file string) Model {
	t.Helper()
	p, err := mft.Load("../mft/testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	m := New(p, file)
	m.base = p.Clone() // pretend it came from the device
	return m
}

func press(m Model, keys ...any) Model {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k := k.(type) {
		case tea.KeyType:
			msg = tea.KeyMsg{Type: k}
		case string:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		out, _ := m.Update(msg)
		m = out.(Model)
	}
	return m
}

func typeStr(m Model, s string) Model {
	for _, r := range s {
		m = press(m, string(r))
	}
	return m
}

func rowOf(t *testing.T, name string) int {
	for i, f := range mft.EncoderFields {
		if f.Name == name {
			return i
		}
	}
	t.Fatalf("no field %q", name)
	return -1
}

func get(p *mft.Preset, slot int, tag byte) int {
	v, _ := p.Encoders[slot].Get(tag)
	return v
}

// Several settings in one apply: CC sequence, channel, indicator.
func TestMultiRowApply(t *testing.T) {
	m := load(t, "4bank.mfs")
	m = press(m, "a") // whole bank 1
	if m.from.Value() != "1" || m.to.Value() != "16" {
		t.Fatalf("range %s–%s", m.from.Value(), m.to.Value())
	}
	m = press(m, tea.KeyTab, tea.KeyTab, tea.KeyTab) // -> table
	if m.focus != fTable {
		t.Fatalf("focus %d", m.focus)
	}
	// Row 0 Encoder MIDI Number: mode keep -> same -> sequence, start 20, step 2.
	m = press(m, " ", " ", tea.KeyRight)
	m = typeStr(m, "20")
	m = press(m, tea.KeyRight)
	m = typeStr(m, "2")
	// Encoder MIDI Channel: type 5 in value column (auto switches to same).
	m = press(m, tea.KeyDown, tea.KeyLeft, tea.KeyRight)
	m = typeStr(m, "5")
	// Indicator: value column, space -> next indicator after the current one.
	ind := rowOf(t, "Indicator")
	for m.row < ind {
		m = press(m, tea.KeyDown)
	}
	m.col = cValue
	m.rows[ind].choice = 2
	m = press(m, " ") // -> Spread (3), mode becomes same
	m = press(m, tea.KeyEnter)

	for n := 1; n <= 16; n++ {
		if got := get(m.preset, n, 17); got != 20+(n-1)*2 {
			t.Errorf("knob %d CC %d, want %d (%s)", n, got, 20+(n-1)*2, m.status)
		}
		if got := get(m.preset, n, 16); got != 5 {
			t.Errorf("knob %d channel %d, want 5", n, got)
		}
		if got := get(m.preset, n, 22); got != 3 {
			t.Errorf("knob %d indicator %d, want 3", n, got)
		}
		if get(m.preset, n, 13) != get(m.base, n, 13) {
			t.Errorf("knob %d button channel changed although row was keep", n)
		}
	}
	if got := len(m.changed()); got != 16 {
		t.Errorf("changed %d, want 16", got)
	}
	if get(m.preset, 17, 17) != get(m.base, 17, 17) {
		t.Error("knob 17 changed")
	}
}

func TestTypedRangeAcrossBanks(t *testing.T) {
	m := load(t, "8bank.mfs")
	m = press(m, tea.KeyTab)
	m = typeStr(m, "12")
	m = press(m, tea.KeyTab)
	m = typeStr(m, "20")
	if a, b := m.rangeBounds(); a != 12 || b != 20 || m.bank != 1 {
		t.Fatalf("range %d–%d bank %d", a, b, m.bank)
	}
}

func TestOutOfRangeRejectsWholeApply(t *testing.T) {
	m := load(t, "4bank.mfs")
	m = press(m, "a")
	m.rows[0].mode, m.rows[1].mode = seq, same
	m.rows[0].val.SetValue("120")
	m.rows[1].val.SetValue("3")
	m.apply()
	if !strings.Contains(m.status, "would reach") || len(m.changed()) != 0 {
		t.Fatalf("status %q, changed %d", m.status, len(m.changed()))
	}
}

func TestViewRenders(t *testing.T) {
	m := load(t, "8bank.mfs")
	m = press(m, tea.KeyShiftRight, tea.KeyShiftDown, tea.KeyTab, tea.KeyTab, tea.KeyTab, " ")
	v := m.View()
	for _, want := range []string{"Bank", "#1", "From knob", "Encoder MIDI Number", "Result"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q", want)
		}
	}
	if p := os.Getenv("MFT_PRINT"); p != "" {
		os.WriteFile(p, []byte(v), 0o644)
	}
	if !strings.Contains(press(m, tea.KeyCtrlG).View(), "Global settings") {
		t.Error("globals screen not shown")
	}
}

func TestListRowArrowsPickValue(t *testing.T) {
	m := load(t, "4bank.mfs")
	m = press(m, tea.KeyTab, tea.KeyTab, tea.KeyTab) // table
	at := rowOf(t, "Encoder Action Type")
	for m.row < at {
		m = press(m, tea.KeyDown)
	}
	if m.col != cValue {
		t.Fatalf("col %d, want value cell on a list row", m.col)
	}
	start := m.rows[at].choice
	m = press(m, tea.KeyRight, tea.KeyRight)
	if got := m.rows[at].choice; got != (start+2)%7 || m.rows[at].mode != same {
		t.Fatalf("choice %d mode %d", got, m.rows[at].mode)
	}
	m = press(m, tea.KeyLeft)
	if got := m.rows[at].choice; got != (start+1)%7 {
		t.Fatalf("after left: choice %d", got)
	}
	m = press(m, "x")
	if m.rows[at].mode != keep || m.rows[at].choice != start {
		t.Fatal("x did not reset the row")
	}
}

func TestShiftArrowsWrapRowsAndBanks(t *testing.T) {
	m := load(t, "8bank.mfs")
	m = press(m, tea.KeyRight, tea.KeyRight, tea.KeyRight) // knob 4, end of row 1
	m = press(m, tea.KeyShiftRight)                        // wraps to knob 5
	if a, b := m.rangeBounds(); a != 4 || b != 5 {
		t.Fatalf("range %d–%d, want 4–5", a, b)
	}
	for i := 0; i < 3; i++ { // 5 -> 17 via shift+down x3 (9, 13, 17)
		m = press(m, tea.KeyShiftDown)
	}
	if a, b := m.rangeBounds(); a != 4 || b != 17 || m.bank != 1 {
		t.Fatalf("range %d–%d bank %d, want 4–17 in bank 2", a, b, m.bank)
	}
	m = press(m, tea.KeyLeft) // plain move clears the range, 17 -> 16 back in bank 1
	if a, b := m.rangeBounds(); a != 16 || b != 16 || m.bank != 0 {
		t.Fatalf("after left: %d–%d bank %d", a, b, m.bank)
	}
}

func TestNumberNudge(t *testing.T) {
	m := load(t, "4bank.mfs")
	m = press(m, tea.KeyTab, tea.KeyTab, tea.KeyTab, tea.KeyRight) // CC row, value cell
	before := m.rows[0].val.Value()
	m = press(m, "+", "+", "-")
	if m.rows[0].mode != same {
		t.Fatal("nudge did not switch row to same")
	}
	if b, _ := strconv.Atoi(before); m.rows[0].val.Value() != strconv.Itoa(b+1) {
		t.Fatalf("value %s, want %d", m.rows[0].val.Value(), b+1)
	}
}
