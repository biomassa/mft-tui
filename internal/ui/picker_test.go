package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// pickerEnv makes a temp HOME with a fighter_twister folder holding one preset.
func pickerEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "Dropbox", "! modular", "fighter_twister")
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	b, err := os.ReadFile("../mft/testdata/8bank.mfs")
	if os.IsNotExist(err) {
		t.Skip("testdata/8bank.mfs not present (presets are not in the repository)")
	}
	os.WriteFile(filepath.Join(dir, "eight.mfs"), b, 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644)
	// Point the picker at it the way a user would: config.json.
	os.MkdirAll(configDir(), 0o755)
	os.WriteFile(filepath.Join(configDir(), "config.json"), []byte(`{"folder": "`+dir+`"}`), 0o644)
	return dir
}

func TestPickerOpen(t *testing.T) {
	dir := pickerEnv(t)
	m := load(t, "4bank.mfs")
	m.path = ""
	m = press(m, tea.KeyCtrlO)
	if m.screen != sPicker || m.pick.dir != dir {
		t.Fatalf("screen %d dir %q", m.screen, m.pick.dir)
	}
	v := m.View()
	if !strings.Contains(v, "eight.mfs") || !strings.Contains(v, "8 banks") || strings.Contains(v, "notes.txt") {
		t.Fatalf("listing wrong:\n%s", v)
	}
	m = typeStr(m, "eig") // filter to the one file
	m = press(m, tea.KeyEnter)
	if m.screen != sMain || m.preset.Slots() != 128 || filepath.Base(m.path) != "eight.mfs" {
		t.Fatalf("not opened: screen %d path %q status %q", m.screen, m.path, m.status)
	}
	if r := loadRecent(); len(r) == 0 || r[0] != m.path {
		t.Fatalf("recent = %v", r)
	}
}

func TestPickerSaveAndOverwrite(t *testing.T) {
	dir := pickerEnv(t)
	m := load(t, "4bank.mfs")
	m.path = ""
	m = press(m, tea.KeyCtrlS)
	if !m.pick.onName {
		t.Fatal("save should start on the name field")
	}
	m = typeStr(m, "mine")
	m = press(m, tea.KeyEnter)
	if _, err := os.Stat(filepath.Join(dir, "mine.mfs")); err != nil {
		t.Fatalf("not saved: %v (%s)", err, m.status)
	}
	// Save again onto the same name: must ask first.
	m = press(m, tea.KeyCtrlS, tea.KeyEnter)
	if m.screen != sConfirm || !strings.Contains(m.question, "Overwrite mine.mfs") {
		t.Fatalf("no overwrite question: screen %d %q", m.screen, m.question)
	}
	m = press(m, "y")
	if !strings.HasPrefix(m.status, "Saved") {
		t.Fatalf("status %q", m.status)
	}
}

func TestPickerProfilesOpenOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(profilesDir(), 0o755)
	p := newPicker(true, profilesDir(), "x")
	if res := p.key(tea.KeyMsg{Type: tea.KeyEnter}); res != nil || !strings.Contains(p.err, "open-only") {
		t.Fatalf("res %v err %q", res, p.err)
	}
}

func TestStartFolderFallsBackToCwd(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no config.json
	FolderFlag = ""
	wd, _ := os.Getwd()
	if got := startFolder(); got != wd {
		t.Fatalf("start %q, want cwd %q", got, wd)
	}
	FolderFlag = "/tmp"
	defer func() { FolderFlag = "" }()
	if got := startFolder(); got != "/tmp" {
		t.Fatalf("flag ignored: %q", got)
	}
}
