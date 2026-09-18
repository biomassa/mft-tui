// Package ui is the Bubble Tea interface: a colour bank grid for picking a
// knob range and an editor table that applies any number of settings to it.
package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/biomassa/mft-tui/internal/mft"
)

type focus int

const (
	fGrid focus = iota
	fFrom
	fTo
	fTable
	fApply
	numFocus
)

type screen int

const (
	sMain screen = iota
	sGlobals
	sPicker  // open/save file browser
	sConfirm // y/n question
)

type mode int

const (
	keep mode = iota // don't touch this setting
	same             // one value for the whole range
	seq              // start + i*step
)

var modeNames = []string{"keep", "same", "sequence"}

// Table columns.
const (
	cMode = iota
	cValue
	cStep
)

type row struct {
	mode      mode
	val, step textinput.Model
	choice    int // Enum/Bool value
}

type Model struct {
	preset *mft.Preset
	base   *mft.Preset // last state read from / written to the device
	path   string

	dev  *mft.Device
	info mft.Info

	bank, cursor, anchor int // cursor/anchor are slots (1-based); anchor 0 = single knob
	focus                focus
	from, to             textinput.Model
	rows                 []row
	row, col             int
	fresh                bool // next digit replaces the focused field's content
	pickMode             bool // selection is a picked set instead of From–To
	picks                []int
	knobs                textinput.Model // pick mode: typed knob list

	screen    screen
	globalIdx int
	pick      picker
	height    int
	question  string
	onYes     func(Model) (Model, tea.Cmd)
	status    string
	busy      bool
}

func numInput() textinput.Model {
	t := textinput.New()
	t.Prompt = ""
	t.CharLimit = 4
	t.Width = 4
	return t
}

func New(preset *mft.Preset, path string) Model {
	m := Model{preset: preset, path: path, cursor: 1, from: numInput(), to: numInput(), knobs: textinput.New()}
	m.knobs.Prompt = ""
	m.knobs.Width = 40
	for range mft.EncoderFields {
		r := row{val: numInput(), step: numInput()}
		r.step.SetValue("1")
		m.rows = append(m.rows, r)
	}
	m.selectionChanged()
	if preset == nil {
		m.status = "No preset loaded: ctrl+r reads the device, ctrl+o opens a file"
	}
	return m
}

// Init reads the device when no file was given.
func (m Model) Init() tea.Cmd {
	if m.preset == nil {
		return readCmd(m.dev)
	}
	return nil
}

// ---- background work ----

type readMsg struct {
	dev    *mft.Device
	info   mft.Info
	preset *mft.Preset
	err    error
}

type writtenMsg struct {
	n   int
	err error
}

func readCmd(dev *mft.Device) tea.Cmd {
	return func() tea.Msg {
		var err error
		if dev == nil {
			if dev, err = mft.Open(); err != nil {
				return readMsg{err: err}
			}
		}
		info, err := dev.Info()
		if err != nil {
			return readMsg{dev: dev, err: err}
		}
		p, err := dev.PullPreset(info.Banks*16, nil)
		return readMsg{dev: dev, info: info, preset: p, err: err}
	}
}

func writeCmd(dev *mft.Device, p *mft.Preset, slots []int) tea.Cmd {
	return func() tea.Msg {
		return writtenMsg{n: len(slots), err: dev.Push(p, slots, nil)}
	}
}

// ---- helpers ----

func (m Model) slots() int {
	if m.preset == nil {
		return 64
	}
	return m.preset.Slots()
}

func (m Model) banks() int { return m.slots() / 16 }

func (m Model) rangeBounds() (int, int) {
	if m.anchor == 0 {
		return m.cursor, m.cursor
	}
	return min(m.anchor, m.cursor), max(m.anchor, m.cursor)
}

// typedRange returns From/To as typed, or ok=false if invalid.
func (m Model) typedRange() (int, int, bool) {
	a, errA := strconv.Atoi(m.from.Value())
	b, errB := strconv.Atoi(m.to.Value())
	ok := errA == nil && errB == nil && a >= 1 && b <= m.slots() && a <= b
	return a, b, ok
}

func (m Model) value(slot int, tag byte) int {
	v, _ := m.preset.Encoders[slot].Get(tag)
	return v
}

func (m Model) colorMap() int {
	if m.preset == nil {
		return 0
	}
	v, _ := m.preset.Globals.Get(33)
	return v
}

// selectionChanged syncs From/To with the grid and pre-fills untouched rows
// with the first knob's values, so editing starts from what is there.
func (m *Model) selectionChanged() {
	if m.pickMode {
		m.prefill(m.firstSelected())
		return
	}
	a, b := m.rangeBounds()
	m.from.SetValue(strconv.Itoa(a))
	m.to.SetValue(strconv.Itoa(b))
	m.prefill(a)
}

// resetRows puts every row back on keep and refills it from the current data.
// Called when the data underneath changes: device read, device write, file open.
// Picks beyond the new knob count are dropped.
func (m *Model) resetRows() {
	for i := range m.rows {
		m.rows[i].mode = keep
		m.rows[i].step.SetValue("1")
	}
	if m.pickMode {
		var keepPicks []int
		for _, n := range m.picks {
			if n <= m.slots() {
				keepPicks = append(keepPicks, n)
			}
		}
		m.setPicks(keepPicks)
	}
	m.clampCol()
	m.prefill(m.firstSelected())
}

func (m *Model) prefill(slot int) {
	if m.preset == nil {
		return
	}
	for i, f := range mft.EncoderFields {
		if m.rows[i].mode != keep {
			continue
		}
		v := m.value(slot, f.Tag)
		m.rows[i].val.SetValue(strconv.Itoa(v))
		m.rows[i].choice = v
	}
}

// rangeFromInputs moves the grid selection after From/To were typed.
func (m *Model) rangeFromInputs() {
	a, b, ok := m.typedRange()
	if !ok {
		return
	}
	m.anchor, m.cursor = a, b
	m.bank = (b - 1) / 16
	m.prefill(a)
}

func (m Model) changed() []int {
	if m.preset == nil {
		return nil
	}
	if m.base == nil {
		all := make([]int, 0, m.slots())
		for n := 1; n <= m.slots(); n++ {
			all = append(all, n)
		}
		return all
	}
	return m.preset.ChangedSlots(m.base)
}

func (m Model) globalsChanged() bool {
	return m.preset != nil && m.base != nil && !m.preset.Globals.Equal(m.base.Globals)
}

func utilityRunning() bool {
	return exec.Command("pgrep", "-f", "Midi Fighter Utility.app").Run() == nil
}

// duplicates counts knobs whose encoder (channel, number) is used more than once.
func duplicates(p *mft.Preset) int {
	seen := map[[2]int]int{}
	for _, ps := range p.Encoders {
		ch, _ := ps.Get(16)
		num, _ := ps.Get(17)
		seen[[2]int{ch, num}]++
	}
	d := 0
	for _, c := range seen {
		if c > 1 {
			d += c
		}
	}
	return d
}

// ---- apply ----

// rowValues computes the values a row would write to the given knobs.
func (m Model) rowValues(i int, knobs []int) ([]int, error) {
	f, r := mft.EncoderFields[i], m.rows[i]
	vals := make([]int, len(knobs))
	if !f.Sequenceable() {
		for k := range vals {
			vals[k] = r.choice
		}
		return vals, nil
	}
	start, err := strconv.Atoi(r.val.Value())
	if err != nil {
		return nil, fmt.Errorf("%s: value must be a number", f.Name)
	}
	step := 0
	if r.mode == seq {
		if step, err = strconv.Atoi(r.step.Value()); err != nil {
			return nil, fmt.Errorf("%s: step must be a number", f.Name)
		}
	}
	for k := range vals {
		vals[k] = start + k*step
		if vals[k] < f.Min() || vals[k] > f.Max() {
			return nil, fmt.Errorf("%s would reach %d on knob %d (allowed %d–%d)", f.Name, vals[k], knobs[k], f.Min(), f.Max())
		}
	}
	return vals, nil
}

func (m *Model) apply() {
	if m.preset == nil {
		m.status = "Nothing to edit yet"
		return
	}
	if _, _, ok := m.typedRange(); !ok && !m.pickMode {
		m.status = fmt.Sprintf("From/To must be 1–%d with From ≤ To", m.slots())
		return
	}
	knobs := m.selected()
	if len(knobs) == 0 {
		m.status = "No knobs picked: s adds the knob under the cursor, or type a list"
		return
	}
	type change struct {
		tag  byte
		vals []int
	}
	var changes []change
	var names []string
	for i, f := range mft.EncoderFields {
		if m.rows[i].mode == keep {
			continue
		}
		vals, err := m.rowValues(i, knobs)
		if err != nil {
			m.status = "Not applied: " + err.Error()
			return
		}
		changes = append(changes, change{f.Tag, vals})
		names = append(names, f.Name)
	}
	if len(changes) == 0 {
		m.status = "Every row is on keep: set a mode (space) or type a value first"
		return
	}
	for k, n := range knobs {
		ps := m.preset.Encoders[n]
		for _, c := range changes {
			ps.Set(c.tag, c.vals[k])
		}
		m.preset.Encoders[n] = ps
	}
	m.status = fmt.Sprintf("Applied to knobs %s: %s", formatKnobs(knobs), strings.Join(names, ", "))
	if d := duplicates(m.preset); d > 0 {
		m.status += fmt.Sprintf(" · ⚠ %d knobs share an encoder channel/number", d)
	}
}

// ---- update ----

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
	case readMsg:
		m.busy = false
		if msg.dev != nil {
			m.dev = msg.dev
		}
		if msg.err != nil {
			m.status = "Read failed: " + msg.err.Error()
			return m, nil
		}
		m.info, m.preset, m.base, m.path = msg.info, msg.preset, msg.preset.Clone(), ""
		m.cursor, m.anchor, m.bank = min(m.cursor, m.slots()), 0, min(m.bank, m.banks()-1)
		m.selectionChanged()
		m.resetRows()
		m.status = fmt.Sprintf("Read %d knobs from %s (firmware %s)", m.slots(), m.dev.Port, m.info.Firmware)
	case writtenMsg:
		m.busy = false
		if msg.err != nil {
			m.status = "Write failed: " + msg.err.Error()
			return m, nil
		}
		m.base = m.preset.Clone()
		m.resetRows()
		m.status = fmt.Sprintf("Wrote %d knobs + globals to the device", msg.n)
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m Model) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	if s == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.screen {
	case sConfirm:
		switch s {
		case "y", "Y":
			m.screen = sMain
			return m.onYes(m)
		case "n", "N", "esc":
			m.screen, m.status = sMain, "Cancelled"
		}
		return m, nil
	case sPicker:
		res := m.pick.key(k)
		if res == nil {
			return m, nil
		}
		m.screen = sMain
		if res.cancel {
			return m, nil
		}
		return m.finishPick(res.path)
	}
	if m.busy {
		return m, nil
	}

	switch s {
	case "ctrl+q":
		return m.confirmQuit()
	case "ctrl+r":
		return m.startRead()
	case "ctrl+w":
		return m.startWrite()
	case "ctrl+o", "ctrl+s":
		save := s == "ctrl+s"
		if save && m.preset == nil {
			return m, nil
		}
		dir, name := "", ""
		if m.path != "" {
			dir, name = filepath.Dir(m.path), filepath.Base(m.path)
		}
		m.pick = newPicker(save, dir, name)
		m.screen = sPicker
		return m, textinput.Blink
	case "ctrl+g":
		if m.screen == sGlobals {
			m.screen = sMain
		} else if m.preset != nil {
			m.screen = sGlobals
		}
		return m, nil
	}
	if m.screen == sGlobals {
		return m.globalsKey(s)
	}
	return m.mainKey(k)
}

func (m Model) confirmQuit() (tea.Model, tea.Cmd) {
	if m.base == nil || len(m.changed()) == 0 && !m.globalsChanged() {
		return m, tea.Quit
	}
	m.screen, m.question = sConfirm, "Quit with changes not written to the device? (y/n)"
	m.onYes = func(m Model) (Model, tea.Cmd) { return m, tea.Quit }
	return m, nil
}

func (m Model) startRead() (tea.Model, tea.Cmd) {
	do := func(m Model) (Model, tea.Cmd) {
		m.busy, m.status = true, "Reading device…"
		return m, readCmd(m.dev)
	}
	if m.base != nil && (len(m.changed()) > 0 || m.globalsChanged()) {
		m.screen, m.question, m.onYes = sConfirm, "Reading replaces your unwritten changes. Continue? (y/n)", do
		return m, nil
	}
	mm, cmd := do(m)
	return mm, cmd
}

func (m Model) startWrite() (tea.Model, tea.Cmd) {
	if m.preset == nil {
		return m, nil
	}
	if m.dev == nil {
		m.status = "Read the device once first (ctrl+r) so the write can be checked against it"
		return m, nil
	}
	devSlots := m.info.Banks * 16
	var slots []int
	for _, n := range m.changed() {
		if n <= devSlots {
			slots = append(slots, n)
		}
	}
	q := fmt.Sprintf("Write %d changed knobs + globals to the device (EEPROM)?", len(slots))
	if m.slots() > devSlots {
		q += fmt.Sprintf(" Knobs above %d are skipped (device has %d banks).", devSlots, m.info.Banks)
	}
	if utilityRunning() {
		q += " ⚠ Midi Fighter Utility is running; quit it or it may overwrite this."
	}
	m.screen, m.question = sConfirm, q+" (y/n)"
	m.onYes = func(m Model) (Model, tea.Cmd) {
		m.busy, m.status = true, fmt.Sprintf("Writing %d knobs…", len(slots))
		return m, writeCmd(m.dev, m.preset, slots)
	}
	return m, nil
}

func (m Model) finishPick(p string) (tea.Model, tea.Cmd) {
	if m.pick.save {
		save := func(m Model) (Model, tea.Cmd) {
			if err := m.preset.Save(p); err != nil {
				m.status = "Save failed: " + err.Error()
			} else {
				m.path, m.status = p, "Saved "+p
				addRecent(p)
			}
			return m, nil
		}
		if _, err := os.Stat(p); err == nil {
			m.screen, m.question, m.onYes = sConfirm, fmt.Sprintf("Overwrite %s? (y/n)", filepath.Base(p)), save
			return m, nil
		}
		mm, cmd := save(m)
		return mm, cmd
	}
	pr, err := mft.Load(p)
	if err != nil {
		m.status = "Open failed: " + err.Error()
		return m, nil
	}
	m.preset, m.path = pr, p
	addRecent(p)
	m.cursor, m.anchor, m.bank = 1, 0, 0
	m.selectionChanged()
	m.resetRows()
	m.status = fmt.Sprintf("Opened %s (%d knobs)", filepath.Base(p), pr.Slots())
	if m.base != nil {
		m.status += fmt.Sprintf(" · %d knobs differ from the device", len(m.changed()))
	}
	return m, nil
}

func (m Model) globalsKey(s string) (tea.Model, tea.Cmd) {
	n := len(mft.GlobalFields)
	f := mft.GlobalFields[m.globalIdx]
	v, _ := m.preset.Globals.Get(f.Tag)
	delta := 0
	switch s {
	case "esc", "g":
		m.screen = sMain
	case "up":
		m.globalIdx = (m.globalIdx + n - 1) % n
	case "down":
		m.globalIdx = (m.globalIdx + 1) % n
	case "left", "-":
		delta = -1
	case "right", "+", "=", " ":
		delta = 1
	case "shift+left":
		delta = -10
	case "shift+right":
		delta = 10
	}
	if delta != 0 {
		nv := min(max(v+delta, f.Min()), f.Max())
		if s == " " && nv == v { // space wraps lists and switches
			nv = f.Min()
		}
		m.preset.Globals.Set(f.Tag, nv)
	}
	return m, nil
}

func (m Model) setFocus(f focus) Model {
	m.from.Blur()
	m.to.Blur()
	if m.pickMode && f == fTo { // single Knobs field: skip To
		if m.focus == fFrom {
			f = fTable
		} else {
			f = fFrom
		}
	}
	m.focus = f
	m.fresh = true
	switch f {
	case fFrom:
		m.from.Focus()
	case fTo:
		m.to.Focus()
	case fTable:
		m.clampCol()
	}
	return m
}

// editText sends a key to a number field: the first digit after focusing
// replaces the content.
func (m *Model) editText(in *textinput.Model, k tea.KeyMsg) bool {
	s := k.String()
	digit := len(s) == 1 && (s[0] >= '0' && s[0] <= '9' || s[0] == '-')
	if !digit && s != "backspace" && s != "delete" && s != "ctrl+u" {
		return false
	}
	if m.fresh && digit {
		in.SetValue("")
	}
	m.fresh = false
	focused := in.Focused()
	in.Focus()
	in.CursorEnd()
	*in, _ = in.Update(k)
	if !focused {
		in.Blur()
	}
	return true
}

func (m Model) mainKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch s {
	case "tab":
		return m.setFocus((m.focus + 1) % numFocus), nil
	case "shift+tab":
		return m.setFocus((m.focus + numFocus - 1) % numFocus), nil
	case "enter":
		m.apply()
		return m, nil
	case "esc":
		if m.pickMode && m.focus == fGrid {
			m.exitPick()
			m.status = "Pick mode off"
			return m, nil
		}
		return m.setFocus(fGrid), nil
	case "q":
		if m.focus != fFrom && m.focus != fTo {
			return m.confirmQuit()
		}
	}
	if m.preset == nil {
		return m, nil
	}
	switch m.focus {
	case fGrid:
		m.gridKey(s)
	case fFrom, fTo:
		if m.pickMode {
			m.listEdit(s)
			return m, nil
		}
		in := &m.from
		if m.focus == fTo {
			in = &m.to
		}
		if m.editText(in, k) {
			m.rangeFromInputs()
		}
	case fTable:
		m.tableKey(k)
	case fApply:
		if s == " " {
			m.apply()
		}
	}
	return m, nil
}

// stepList moves a list row through keep, first value … last value, keep, …
func (m *Model) stepList(forward bool) {
	r, last := &m.rows[m.row], mft.EncoderFields[m.row].Max()
	switch {
	case r.mode == keep && forward:
		r.mode, r.choice = same, 0
	case r.mode == keep:
		r.mode, r.choice = same, last
	case forward && r.choice == last, !forward && r.choice == 0:
		r.mode = keep
		m.prefill(m.firstSelected())
	case forward:
		r.choice++
	default:
		r.choice--
	}
}

func (m *Model) maxCol(i int) int {
	if !mft.EncoderFields[i].Sequenceable() || m.rows[i].mode != seq {
		return cValue
	}
	return cStep
}

// minCol: list and on/off rows have only the value cell.
func (m *Model) minCol(i int) int {
	if !mft.EncoderFields[i].Sequenceable() {
		return cValue
	}
	return cMode
}

func (m *Model) clampCol() { m.col = min(max(m.col, m.minCol(m.row)), m.maxCol(m.row)) }

// nudge adds d to a number field's text value.
func nudge(in *textinput.Model, d, lo, hi int) {
	v, err := strconv.Atoi(in.Value())
	if err != nil {
		v = lo
	}
	in.SetValue(strconv.Itoa(min(max(v+d, lo), hi)))
}

func (m *Model) tableKey(k tea.KeyMsg) {
	s := k.String()
	f := mft.EncoderFields[m.row]
	r := &m.rows[m.row]
	switch s {
	case "up":
		m.row = max(m.row-1, 0)
		m.fresh = true
		m.clampCol()
		return
	case "down":
		m.row = min(m.row+1, len(m.rows)-1)
		m.fresh = true
		m.clampCol()
		return
	case "left", "right":
		if !f.Sequenceable() { // list rows: arrows pick the value
			m.stepList(s == "right")
			return
		}
		if s == "left" {
			m.col = max(m.col-1, cMode)
		} else {
			m.col = min(m.col+1, m.maxCol(m.row))
		}
		m.fresh = true
		return
	}
	switch m.col {
	case cMode:
		if s == " " {
			switch {
			case r.mode == keep:
				r.mode = same
			case r.mode == same && f.Sequenceable():
				r.mode = seq
			default:
				r.mode = keep
				m.prefill(m.firstSelected())
			}
		}
	case cValue:
		if f.Sequenceable() {
			if s == "+" || s == "=" || s == "-" {
				d := map[bool]int{true: -1, false: 1}[s == "-"]
				nudge(&r.val, d, f.Min(), f.Max())
				if r.mode == keep {
					r.mode = same
				}
				return
			}
			if m.editText(&r.val, k) && r.mode == keep {
				r.mode = same
			}
			return
		}
		switch s {
		case " ", "+", "=":
			m.stepList(true)
		case "-":
			m.stepList(false)
		}
	case cStep:
		if s == "+" || s == "=" {
			nudge(&r.step, 1, -127, 127)
			return
		}
		m.editText(&r.step, k)
	}
}

// gridKey moves in knob order like a text cursor: left/right wrap across
// rows and banks, up/down move 4 knobs; with shift the range extends.
func (m *Model) gridKey(s string) {
	idx := (m.cursor - 1) % 16
	extend := strings.HasPrefix(s, "shift+")
	delta := map[string]int{"up": -4, "down": 4, "left": -1, "right": 1}[strings.TrimPrefix(s, "shift+")]
	if delta != 0 {
		next := m.cursor + delta
		if next < 1 || next > m.slots() {
			return
		}
		if m.pickMode && !extend { // pick mode: arrows only move the cursor
			m.cursor, m.bank = next, (next-1)/16
			return
		}
		if m.pickMode && extend { // shift+arrow leaves pick mode, starts a range here
			m.exitPick()
			m.status = "Pick mode off"
		}
		if extend && m.anchor == 0 {
			m.anchor = m.cursor
		} else if !extend {
			m.anchor = 0
		}
		m.cursor = next
		m.bank = (next - 1) / 16
		m.selectionChanged()
		return
	}
	switch {
	case s == "[" || s == "pgup":
		m.bank = max(m.bank-1, 0)
	case s == "]" || s == "pgdown":
		m.bank = min(m.bank+1, m.banks()-1)
	case len(s) == 1 && s[0] >= '1' && s[0] <= '8' && int(s[0]-'1') < m.banks():
		m.bank = int(s[0] - '1')
	case s == "s": // pick mode: toggle the knob under the cursor
		m.togglePick(m.cursor)
		return
	case s == "a": // whole bank: range, or toggle it in pick mode
		if m.pickMode {
			var bank []int
			for n := m.bank*16 + 1; n <= m.bank*16+16; n++ {
				bank = append(bank, n)
			}
			m.togglePick(bank...)
			return
		}
		m.anchor, m.cursor = m.bank*16+1, m.bank*16+16
		m.selectionChanged()
		return
	default:
		return
	}
	if m.pickMode {
		m.cursor = m.bank*16 + idx + 1
		return
	}
	m.cursor, m.anchor = m.bank*16+idx+1, 0
	m.selectionChanged()
}

// ---- view ----

var (
	sTitle   = lipgloss.NewStyle().Bold(true)
	sDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	sCell    = lipgloss.NewStyle().Width(13).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))
	sSel     = sCell.Background(lipgloss.Color("24"))
	cursorBC = lipgloss.Color("14")
	sFocused = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
	sEdit    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	sWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	sErr     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	sBox     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	sKey     = lipgloss.NewStyle().Foreground(lipgloss.Color("#fab387")) // peach: keys and actionable labels
)

var typeShort = []string{"note", "cc", "rel", "bvel", "mouse", "scrl", "pc"}

func hex(c [3]uint8) lipgloss.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", c[0], c[1], c[2]))
}

func swatch(c [3]uint8, w int) string {
	return lipgloss.NewStyle().Foreground(hex(c)).Render(strings.Repeat("█", w))
}

func (m Model) cell(slot int) string {
	ps := m.preset.Encoders[slot]
	get := func(t byte) int { v, _ := ps.Get(t); return v }
	t := "?"
	if et := get(18); et < len(typeShort) {
		t = typeShort[et]
	}
	mark := " "
	if m.base != nil {
		if b, ok := m.base.Encoders[slot]; !ok || !ps.Equal(b) {
			mark = sWarn.Render("*")
		}
	}
	cm := m.colorMap()
	strip := swatch(mft.PaletteRGB(cm, get(20)), 5) + " " + swatch(mft.PaletteRGB(cm, get(19)), 3) + " " + swatch(mft.DetentRGB(get(21)), 1)
	return fmt.Sprintf("#%d%s\n%d:%s%d\nb%d:%d\n%s", slot, mark, get(16), t, get(17), get(13), get(14), strip)
}

func (m Model) grid() string {
	var rows []string
	for r := 0; r < 4; r++ {
		var cells []string
		for c := 0; c < 4; c++ {
			slot := m.bank*16 + r*4 + c + 1
			st := sCell
			if m.isSelected(slot) {
				st = sSel
			}
			if slot == m.cursor {
				st = st.BorderForeground(cursorBC)
			}
			cells = append(cells, st.Render(m.cell(slot)))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}
	legend := sDim.Render("#knob · encoder ch:type+number · b button ch:number\nstrip: off colour · on colour · detent colour")
	return lipgloss.JoinVertical(lipgloss.Left, append(rows, legend)...)
}

func (m Model) bankBar() string {
	var parts []string
	has := map[int]bool{} // banks holding selected knobs
	for _, n := range m.selected() {
		has[(n-1)/16] = true
	}
	for b := 0; b < m.banks(); b++ {
		mark := " "
		if has[b] {
			mark = "•"
		}
		if b == m.bank {
			parts = append(parts, sKey.Bold(true).Render(fmt.Sprintf("[%d]", b+1))+sKey.Render(mark))
		} else {
			parts = append(parts, sKey.Render(fmt.Sprintf(" %d%s", b+1, mark)))
		}
	}
	return "Bank " + strings.Join(parts, "")
}

// now summarises a setting over the selected range.
func (m Model) now(f mft.Field) string {
	sel := m.selected()
	if len(sel) == 0 {
		return "—"
	}
	lo, hi := 1<<30, -1
	for _, n := range sel {
		v := m.value(n, f.Tag)
		lo, hi = min(lo, v), max(hi, v)
	}
	if lo == hi {
		return f.Format(lo)
	}
	if f.Kind == mft.Enum || f.Kind == mft.Bool {
		return "mixed"
	}
	return fmt.Sprintf("%d…%d", lo, hi)
}

func pad(s string, w int) string {
	if n := lipgloss.Width(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

func (m Model) cellText(i, col int, text string) string {
	if m.focus == fTable && m.row == i && m.col == col {
		return sFocused.Render("‹" + text + "›")
	}
	return " " + text + " "
}

func (m Model) colorOf(tag byte, v int) [3]uint8 {
	if tag == 21 {
		return mft.DetentRGB(v)
	}
	return mft.PaletteRGB(m.colorMap(), v)
}

func isColor(tag byte) bool { return tag == 19 || tag == 20 || tag == 21 }

func (m Model) table() string {
	sel := m.selected()
	first := m.firstSelected()
	var lines []string
	head := "  " + pad("Setting", 22) + pad("Now", 20) + pad("Mode", 11) + pad("Value", 12) + pad("Step", 7) + "Result"
	lines = append(lines, sDim.Render(head))
	for i, f := range mft.EncoderFields {
		r := m.rows[i]
		marker := "  "
		if m.focus == fTable && m.row == i {
			marker = sFocused.Render("▸ ")
		}
		name := f.Name
		if r.mode != keep {
			name = sEdit.Render(name)
		}
		nowS := m.now(f)
		if isColor(f.Tag) {
			nowS = swatch(m.colorOf(f.Tag, m.value(first, f.Tag)), 2) + " " + nowS
		}
		modeS := m.cellText(i, cMode, modeNames[r.mode])
		var valS, stepS, result string
		switch {
		case r.mode == keep:
			valS = sDim.Render(m.cellText(i, cValue, "—"))
		case f.Sequenceable():
			label := "Value"
			if r.mode == seq {
				label = "Start"
			}
			valS = m.cellText(i, cValue, sDim.Render(label)+" "+pad(r.val.Value(), 3))
			if r.mode == seq {
				stepS = m.cellText(i, cStep, pad(r.step.Value(), 3))
			}
			if len(sel) == 0 {
				result = ""
			} else if vals, err := m.rowValues(i, sel); err != nil {
				result = sErr.Render("✗ out of range")
			} else {
				result = fmt.Sprintf("→ %d", vals[0])
				if vals[0] != vals[len(vals)-1] {
					result = fmt.Sprintf("→ %d…%d", vals[0], vals[len(vals)-1])
				}
				if isColor(f.Tag) {
					result = swatch(m.colorOf(f.Tag, vals[0]), 2) + " " + result
				}
			}
		default:
			valS = m.cellText(i, cValue, f.Format(r.choice))
		}
		lines = append(lines, marker+pad(name, 22)+pad(nowS, 20)+pad(modeS, 11)+pad(valS, 12)+pad(stepS, 7)+result)
	}
	return strings.Join(lines, "\n")
}

func (m Model) rangeLine() string {
	lbl := func(f focus, t string) string {
		if m.focus == f {
			return sFocused.Render("▸ " + t)
		}
		return "  " + sKey.Render(t)
	}
	n := len(m.selected())
	knobs := "knobs"
	if n == 1 {
		knobs = "knob"
	}
	if m.pickMode {
		list := m.knobs.Value()
		if list == "" {
			list = sDim.Render("(none: s adds, or type 1 3 5-8)")
		}
		return fmt.Sprintf("%s %s  %s", lbl(fFrom, "Knobs:"), list, sDim.Render(fmt.Sprintf("(%d %s · esc or ⇧arrow ends picking)", n, knobs)))
	}
	return fmt.Sprintf("%s %s %s %s  %s", lbl(fFrom, "From knob"), pad(m.from.Value(), 4), lbl(fTo, "To knob"), pad(m.to.Value(), 4),
		sDim.Render(fmt.Sprintf("(%d %s)", n, knobs)))
}

func (m Model) applyLine() string {
	apply := "  " + sKey.Render("[ Apply ]")
	if m.focus == fApply {
		apply = sFocused.Render("▸ [ Apply ]")
	}
	return apply + "  " + sDim.Render("enter applies all rows not on keep")
}

func (m Model) globalsView() string {
	var lines []string
	lines = append(lines, sTitle.Render("Global settings")+sDim.Render("   ↑↓ select · ←→ or -/+ change · shift+←→ ±10 · esc back"))
	for i, f := range mft.GlobalFields {
		v, _ := m.preset.Globals.Get(f.Tag)
		mark := " "
		if m.base != nil {
			if bv, _ := m.base.Globals.Get(f.Tag); bv != v {
				mark = "*"
			}
		}
		line := fmt.Sprintf("%-28s %s%s", f.Name, f.Format(v), mark)
		if i == m.globalIdx {
			line = sFocused.Render("▸ " + line)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m Model) header() string {
	src := "no source"
	switch {
	case m.path != "":
		src = filepath.Base(m.path)
	case m.base != nil:
		src = "device"
	}
	h := sTitle.Render("mft-tui") + "  " + src
	if m.info.Firmware != "" {
		h += sDim.Render(fmt.Sprintf("  · fw %s · %d banks", m.info.Firmware, m.info.Banks))
	}
	if m.base != nil {
		n := len(m.changed())
		g := ""
		if m.globalsChanged() {
			g = " + globals"
		}
		if n > 0 || g != "" {
			h += sWarn.Render(fmt.Sprintf("  · %d knobs%s not written", n, g))
		}
	}
	return h
}

// help lines: pairs of key, description.
var helpLines = [][][2]string{
	{{"tab/⇧tab", "grid → from → to → program → apply"}, {"arrows", "move"}, {"⇧arrows", "select range"}, {"[ ] 1–8", "bank"}, {"a", "whole bank"}, {"s", "pick/unpick knob"}},
	{{"↑↓", "row"}, {"←→/space", "list rows: pick value · number rows: mode/value/step"}, {"space", "cycles mode"}, {"digits -/+", "value"}, {"enter", "apply"}},
	{{"^r", "read device"}, {"^w", "write device"}, {"^o", "open"}, {"^s", "save .mfs"}, {"^g", "globals"}, {"q/^q", "quit"}},
}

func helpView() string {
	var lines []string
	for _, l := range helpLines {
		var parts []string
		for _, kv := range l {
			parts = append(parts, sKey.Render(kv[0])+" "+sDim.Render(kv[1]))
		}
		lines = append(lines, strings.Join(parts, sDim.Render(" · ")))
	}
	return strings.Join(lines, "\n")
}

func (m Model) View() string {
	status := m.status
	if m.screen == sConfirm {
		status = sWarn.Render(m.question)
	}
	var body string
	switch {
	case m.preset == nil:
		body = "\n  (no preset)\n\n" + status
	case m.screen == sPicker:
		return m.header() + "\n\n" + sBox.Render(m.pick.view(m.height))
	case m.screen == sGlobals:
		body = sBox.Render(m.globalsView()) + "\n" + status
	default:
		left := lipgloss.JoinVertical(lipgloss.Left, m.bankBar(), m.grid())
		// One line down so the panel's top border lines up with the first knob row.
		panel := sBox.Render(lipgloss.JoinVertical(lipgloss.Left, m.rangeLine(), "", m.table(), "", m.applyLine()))
		// Status and questions sit under the panel, wrapped to its width.
		msg := lipgloss.NewStyle().Width(lipgloss.Width(panel)).PaddingLeft(1).Render(status)
		right := "\n" + lipgloss.JoinVertical(lipgloss.Left, panel, msg)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	}
	// The line between body and help stays empty.
	return m.header() + "\n\n" + body + "\n\n" + helpView()
}
