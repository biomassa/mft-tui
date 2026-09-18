package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Selection: either a From–To range (default) or a picked set of knobs
// (pick mode, entered with s). Everything that edits or summarises knobs
// works on selected(), always in knob-number order.

// parseKnobs reads "1 3 5-8, 17" (spaces and/or commas, ranges with -).
func parseKnobs(s string, max int) ([]int, error) {
	seen := map[int]bool{}
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
	for _, f := range fields {
		lo, hi := f, f
		if i := strings.Index(f[1:], "-"); i >= 0 { // not a leading minus
			lo, hi = f[:i+1], f[i+2:]
		}
		a, errA := strconv.Atoi(lo)
		b, errB := strconv.Atoi(hi)
		if errA != nil || errB != nil {
			return nil, fmt.Errorf("%q is not a knob number or range", f)
		}
		if a > b {
			a, b = b, a
		}
		if a < 1 || b > max {
			return nil, fmt.Errorf("%q is outside 1–%d", f, max)
		}
		for n := a; n <= b; n++ {
			seen[n] = true
		}
	}
	out := make([]int, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Ints(out)
	return out, nil
}

// formatKnobs writes a sorted set compactly: "1, 3, 5-8, 17".
func formatKnobs(ks []int) string {
	var parts []string
	for i := 0; i < len(ks); {
		j := i
		for j+1 < len(ks) && ks[j+1] == ks[j]+1 {
			j++
		}
		if j > i {
			parts = append(parts, fmt.Sprintf("%d-%d", ks[i], ks[j]))
		} else {
			parts = append(parts, strconv.Itoa(ks[i]))
		}
		i = j + 1
	}
	return strings.Join(parts, ", ")
}

// selected returns the knobs an Apply would touch, in knob order.
func (m Model) selected() []int {
	if m.pickMode {
		return m.picks
	}
	a, b, ok := m.typedRange()
	if !ok {
		a, b = m.rangeBounds()
	}
	out := make([]int, 0, b-a+1)
	for n := a; n <= b; n++ {
		out = append(out, n)
	}
	return out
}

func (m Model) isSelected(slot int) bool {
	for _, n := range m.selected() {
		if n == slot {
			return true
		}
	}
	return false
}

// firstSelected is the knob whose values pre-fill untouched rows.
func (m Model) firstSelected() int {
	if s := m.selected(); len(s) > 0 {
		return s[0]
	}
	return m.cursor
}

func (m *Model) setPicks(ks []int) {
	m.picks = ks
	m.knobs.SetValue(formatKnobs(ks))
	m.prefill(m.firstSelected())
}

// togglePick adds or removes knobs; entering pick mode starts an empty set.
func (m *Model) togglePick(ks ...int) {
	if !m.pickMode {
		m.pickMode, m.picks, m.anchor = true, nil, 0
	}
	set := map[int]bool{}
	for _, n := range m.picks {
		set[n] = true
	}
	all := true
	for _, n := range ks {
		all = all && set[n]
	}
	for _, n := range ks {
		set[n] = !all // all present -> remove them, otherwise add
	}
	var out []int
	for n, on := range set {
		if on {
			out = append(out, n)
		}
	}
	sort.Ints(out)
	m.setPicks(out)
}

func (m *Model) exitPick() {
	m.pickMode, m.picks = false, nil
	m.knobs.SetValue("")
	if m.focus == fFrom {
		m.focus = fGrid
	}
	m.selectionChanged()
}

// listEdit handles typing in the Knobs field.
func (m *Model) listEdit(s string) bool {
	v := m.knobs.Value()
	switch {
	case s == "backspace":
		if len(v) > 0 {
			v = v[:len(v)-1]
		}
	case s == "ctrl+u":
		v = ""
	case len(s) == 1 && strings.ContainsAny(s, "0123456789-, "):
		v += s
	default:
		return false
	}
	m.knobs.SetValue(v)
	ks, err := parseKnobs(v, m.slots())
	if err != nil {
		m.status = "Knobs: " + err.Error()
		return true
	}
	m.status = ""
	m.picks = ks
	m.prefill(m.firstSelected())
	return true
}
