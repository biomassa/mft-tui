package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/biomassa/mft-tui/internal/mft"
)

// The picker is a full-screen .mfs browser used for open and save.

type entryKind int

const (
	ePlace entryKind = iota
	eParent
	eDir
	eFile
)

type entry struct {
	kind  entryKind
	label string // shown name
	path  string
	info  string // size · date · banks
}

type picker struct {
	save    bool
	dir     string
	entries []entry // everything in dir plus places
	filter  string
	idx     int
	name    textinput.Model // save: file name
	onName  bool            // save: focus is on the name field
	err     string
}

func home() string { h, _ := os.UserHomeDir(); return h }

// FolderFlag is set from the --dir command-line flag; it overrides config.json.
var FolderFlag string

func configDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = home()
	}
	return filepath.Join(d, "mft-tui")
}

// configuredFolder is the picker's preset folder: --dir, else "folder" in
// config.json, else "" (not set).
func configuredFolder() string {
	if FolderFlag != "" {
		return FolderFlag
	}
	var cfg struct {
		Folder string `json:"folder"`
	}
	if b, err := os.ReadFile(filepath.Join(configDir(), "config.json")); err == nil && json.Unmarshal(b, &cfg) == nil && cfg.Folder != "" {
		if strings.HasPrefix(cfg.Folder, "~/") {
			return filepath.Join(home(), cfg.Folder[2:])
		}
		return cfg.Folder
	}
	return ""
}

// startFolder is where the picker opens: the configured folder or the current directory.
func startFolder() string {
	if f := configuredFolder(); f != "" {
		return f
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return home()
}

func profilesDir() string {
	return filepath.Join(home(), "Library", "Application Support", "DJTechTools", "MFU", "profiles")
}

func recentFile() string { return filepath.Join(configDir(), "recent.json") }

func loadRecent() []string {
	var r []string
	if b, err := os.ReadFile(recentFile()); err == nil {
		json.Unmarshal(b, &r)
	}
	return r
}

// addRecent puts path first in the recent list (max 10).
func addRecent(path string) {
	r := []string{path}
	for _, p := range loadRecent() {
		if p != path && len(r) < 10 {
			r = append(r, p)
		}
	}
	os.MkdirAll(filepath.Dir(recentFile()), 0o755)
	if b, err := json.Marshal(r); err == nil {
		os.WriteFile(recentFile(), b, 0o644)
	}
}

// profileNames maps profile file names (<uuid>.mfs) to their Utility names.
func profileNames() map[string]string {
	var idx struct {
		Profiles []struct{ ID, Name string } `json:"profiles"`
	}
	names := map[string]string{}
	if b, err := os.ReadFile(filepath.Join(profilesDir(), "profiles.json")); err == nil && json.Unmarshal(b, &idx) == nil {
		for _, p := range idx.Profiles {
			names[p.ID+".mfs"] = p.Name
		}
	}
	return names
}

func fileInfo(path string, fi os.FileInfo) string {
	s := fmt.Sprintf("%5d B  %s", fi.Size(), fi.ModTime().Format("2006-01-02 15:04"))
	if fi.Size() == 0 {
		return s + "  (empty / not downloaded)"
	}
	if p, err := mft.Load(path); err == nil {
		return s + fmt.Sprintf("  %d banks", p.Slots()/16)
	}
	return s + "  (not a valid .mfs)"
}

func newPicker(save bool, start, name string) picker {
	p := picker{save: save, name: textinput.New()}
	p.name.Prompt = ""
	p.name.Width = 50
	p.name.SetValue(name)
	p.onName = save
	if save {
		p.name.Focus()
	}
	if start == "" {
		start = startFolder()
	}
	p.cd(start)
	return p
}

func (p *picker) cd(dir string) {
	p.dir, p.filter, p.idx, p.err = dir, "", 0, ""
	p.entries = nil
	// Places first.
	if f := configuredFolder(); f != "" {
		p.entries = append(p.entries, entry{kind: ePlace, label: "★ " + filepath.Base(f), path: f})
	}
	if isDir(profilesDir()) {
		p.entries = append(p.entries, entry{kind: ePlace, label: "★ Utility 3.0 profiles", path: profilesDir()})
	}
	for _, r := range loadRecent() {
		if fi, err := os.Stat(r); err == nil && !fi.IsDir() {
			p.entries = append(p.entries, entry{kind: ePlace, label: "↺ " + filepath.Base(r), path: r, info: filepath.Dir(r)})
		}
	}
	p.entries = append(p.entries, entry{kind: eParent, label: "..", path: filepath.Dir(dir)})

	ents, err := os.ReadDir(dir)
	if err != nil {
		p.err = err.Error()
		return
	}
	names := map[string]string{}
	if filepath.Clean(dir) == filepath.Clean(profilesDir()) {
		names = profileNames()
	}
	var dirs, files []entry
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(dir, e.Name())
		if e.IsDir() {
			dirs = append(dirs, entry{kind: eDir, label: e.Name() + "/", path: full})
			continue
		}
		if !strings.EqualFold(filepath.Ext(e.Name()), ".mfs") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		label := e.Name()
		if n, ok := names[e.Name()]; ok {
			label = n + "  " + sDim.Render("("+e.Name()+")")
		}
		files = append(files, entry{kind: eFile, label: label, path: full, info: fileInfo(full, fi)})
	}
	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i].label) < strings.ToLower(dirs[j].label) })
	sort.Slice(files, func(i, j int) bool { return strings.ToLower(files[i].label) < strings.ToLower(files[j].label) })
	p.entries = append(p.entries, dirs...)
	p.entries = append(p.entries, files...)
}

// visible returns entries matching the filter (places and .. always shown when unfiltered).
func (p picker) visible() []entry {
	if p.filter == "" {
		return p.entries
	}
	var out []entry
	f := strings.ToLower(p.filter)
	for _, e := range p.entries {
		if (e.kind == eDir || e.kind == eFile) && strings.Contains(strings.ToLower(e.label), f) {
			out = append(out, e)
		}
	}
	return out
}

func (p picker) inProfiles() bool { return filepath.Clean(p.dir) == filepath.Clean(profilesDir()) }

// pickerResult is what a key press asks the main model to do.
type pickerResult struct {
	cancel bool
	path   string // chosen file (open) or target (save)
}

func (p *picker) key(k tea.KeyMsg) (res *pickerResult) {
	s := k.String()
	vis := p.visible()
	switch s {
	case "esc":
		if p.filter != "" && !p.onName {
			p.filter, p.idx = "", 0
			return nil
		}
		return &pickerResult{cancel: true}
	case "tab", "shift+tab":
		if p.save {
			p.onName = !p.onName
			if p.onName {
				p.name.Focus()
			} else {
				p.name.Blur()
			}
		}
		return nil
	}
	if p.onName {
		if s == "enter" {
			name := strings.TrimSpace(p.name.Value())
			if name == "" {
				p.err = "Type a file name"
				return nil
			}
			if p.inProfiles() {
				p.err = "The Utility's profiles folder is open-only (its index would not know the file)"
				return nil
			}
			if !strings.EqualFold(filepath.Ext(name), ".mfs") {
				name += ".mfs"
			}
			return &pickerResult{path: filepath.Join(p.dir, name)}
		}
		p.name, _ = p.name.Update(k)
		return nil
	}
	switch s {
	case "up", "ctrl+p":
		p.idx = max(p.idx-1, 0)
	case "down", "ctrl+n":
		p.idx = min(p.idx+1, len(vis)-1)
	case "pgup":
		p.idx = max(p.idx-10, 0)
	case "pgdown":
		p.idx = min(p.idx+10, len(vis)-1)
	case "backspace":
		if p.filter != "" {
			p.filter = p.filter[:len(p.filter)-1]
			p.idx = 0
		} else {
			p.cd(filepath.Dir(p.dir))
		}
	case "enter", "right":
		if len(vis) == 0 {
			return nil
		}
		e := vis[p.idx]
		switch {
		case e.kind == eFile || e.kind == ePlace && !isDir(e.path):
			if p.save {
				p.cd(filepath.Dir(e.path))
				p.name.SetValue(filepath.Base(e.path))
				p.onName = true
				p.name.Focus()
				p.name.CursorEnd()
				return nil
			}
			return &pickerResult{path: e.path}
		default:
			p.cd(e.path)
		}
	case "left":
		p.cd(filepath.Dir(p.dir))
	default:
		if r := k.Runes; k.Type == tea.KeyRunes && len(r) > 0 {
			p.filter += string(r)
			p.idx = 0
		}
	}
	return nil
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func (p picker) view(height int) string {
	var b strings.Builder
	title := "Open preset"
	if p.save {
		title = "Save preset"
	}
	fmt.Fprintf(&b, "%s  %s\n", sTitle.Render(title), sDim.Render(p.dir))
	if p.filter != "" {
		fmt.Fprintf(&b, "%s %s\n", sKey.Render("filter:"), p.filter)
	} else {
		b.WriteString(sDim.Render("type to filter") + "\n")
	}
	vis := p.visible()
	rows := max(height-8, 5)
	top := max(0, min(p.idx-rows/2, len(vis)-rows))
	for i := top; i < min(len(vis), top+rows); i++ {
		e := vis[i]
		line := pad(e.label, 48) + " " + sDim.Render(e.info)
		if e.kind == ePlace {
			line = sKey.Render(e.label) + "  " + sDim.Render(e.info)
		}
		if i == p.idx && !p.onName {
			line = sFocused.Render("▸ ") + line
		} else {
			line = "  " + line
		}
		b.WriteString(line + "\n")
	}
	if len(vis) == 0 {
		b.WriteString(sDim.Render("  (nothing matches)") + "\n")
	}
	if p.save {
		lbl := sKey.Render("File name")
		if p.onName {
			lbl = sFocused.Render("▸ File name")
		}
		fmt.Fprintf(&b, "\n%s %s\n", lbl, p.name.View())
	}
	if p.err != "" {
		b.WriteString(sErr.Render(p.err) + "\n")
	}
	keys := [][2]string{{"↑↓", "move"}, {"enter/→", "open folder or pick"}, {"←/backspace", "parent"}, {"type", "filter"}, {"esc", "cancel"}}
	if p.save {
		keys = append(keys, [2]string{"tab", "list ↔ name"}, [2]string{"enter on name", "save"})
	}
	var parts []string
	for _, kv := range keys {
		parts = append(parts, sKey.Render(kv[0])+" "+sDim.Render(kv[1]))
	}
	b.WriteString(strings.Join(parts, sDim.Render(" · ")))
	return b.String()
}
