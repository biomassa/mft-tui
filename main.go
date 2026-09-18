// mft-tui edits Midi Fighter Twister settings over knob ranges.
//
//	mft-tui            read the connected Twister
//	mft-tui file.mfs   open a Utility settings file
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/biomassa/mft-tui/internal/mft"
	"github.com/biomassa/mft-tui/internal/ui"
)

func main() {
	var preset *mft.Preset
	var path string
	if len(os.Args) > 1 {
		path = os.Args[1]
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
