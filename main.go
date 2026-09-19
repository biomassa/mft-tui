// mft-tui edits Midi Fighter Twister settings over knob ranges.
//
//	mft-tui                   read the connected Twister
//	mft-tui file.mfs          open a Utility settings file
//	mft-tui --dir FOLDER ...  folder the file picker opens
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/biomassa/mft-tui/internal/mft"
	"github.com/biomassa/mft-tui/internal/ui"
)

func main() {
	flag.StringVar(&ui.FolderFlag, "dir", "", "folder the file picker opens (overrides config.json)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: mft-tui [--dir FOLDER] [file.mfs]")
		flag.PrintDefaults()
	}
	flag.Parse()
	var preset *mft.Preset
	var path string
	if flag.NArg() > 0 {
		path = flag.Arg(0)
		var err error
		if preset, err = mft.Load(path); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	defer mft.CloseDriver()
	if _, err := tea.NewProgram(ui.New(preset, path), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
