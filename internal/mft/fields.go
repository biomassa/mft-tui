// Package mft reads and writes Midi Fighter Twister presets (.mfs) and talks
// to the device over MIDI SysEx. Format notes: MFT-protocol.md.
package mft

import "fmt"

// Kind says how a field's value is edited and shown.
type Kind int

const (
	Number  Kind = iota // 0..127
	Channel             // 1..16
	Bool                // 0/1
	Enum                // index into Labels
)

// Field describes one tag of the global or encoder table.
type Field struct {
	Tag    byte
	Name   string
	Kind   Kind
	Labels []string // Enum only
}

func (f Field) Min() int {
	if f.Kind == Channel {
		return 1
	}
	return 0
}

func (f Field) Max() int {
	switch f.Kind {
	case Channel:
		return 16
	case Bool:
		return 1
	case Enum:
		return len(f.Labels) - 1
	}
	return 127
}

// Sequenceable fields can take "start + i*step" over a knob range.
func (f Field) Sequenceable() bool { return f.Kind == Number || f.Kind == Channel }

func (f Field) Format(v int) string {
	switch f.Kind {
	case Bool:
		if v != 0 {
			return "On"
		}
		return "Off"
	case Enum:
		if v >= 0 && v < len(f.Labels) {
			return f.Labels[v]
		}
		return fmt.Sprintf("? (%d)", v)
	}
	return fmt.Sprint(v)
}

// Encoder fields, in the order the Utility shows them. Tag 15 (deprecated
// switch MIDI type) is kept in presets but not offered for editing.
var EncoderFields = []Field{
	{17, "Encoder MIDI Number", Number, nil},
	{16, "Encoder MIDI Channel", Channel, nil},
	{18, "Encoder Action Type", Enum, []string{"Note", "CC", "Relative (3FH/41H)", "Button Velocity Control", "Mouse Move", "Mouse Scroll", "Program Change"}},
	{11, "Sensitivity", Enum, []string{"360", "Responsive", "Velocity Sensitive"}},
	{24, "Shift MIDI Channel", Channel, nil},
	{14, "Button MIDI Number", Number, nil},
	{13, "Button MIDI Channel", Channel, nil},
	{12, "Button Action Type", Enum, []string{"CC Hold", "CC Toggle", "Note Hold", "Note Toggle", "Reset Encoder Value (0)", "Reset Encoder Value (127)", "Encoder Fine Adjust", "Shift Encoder (Hold)", "Shift Encoder (Toggle)"}},
	{19, "On Color", Number, nil},
	{20, "Off Color", Number, nil},
	{22, "Indicator", Enum, []string{"Dot", "Bar", "Blended Bar", "Spread"}},
	{10, "Center Detent", Bool, nil},
	{21, "Detent Color", Number, nil},
	{23, "Super Knob", Bool, nil},
}

var sideFunctions = []string{"CC Hold", "CC Toggle", "Note Hold", "Note Toggle", "Shift Page A", "Shift Page B",
	"Shift Page A (Toggle)", "Shift Page B (Toggle)", "Next Bank", "Previous Bank", "Bank Select",
	"Bank 1", "Bank 2", "Bank 3", "Bank 4", "Bank 5", "Bank 6", "Bank 7", "Bank 8", "Cycle Bank"}

var GlobalFields = []Field{
	{0, "System Channel", Channel, nil},
	{34, "Encoders Animation Channel", Channel, nil},
	{35, "Button Animation Channel", Channel, nil},
	{8, "Super Knob Start", Number, nil},
	{9, "Super Knob End", Number, nil},
	{33, "Color Map", Enum, []string{"Classic", "Expanded"}},
	{36, "Sleep Timer", Enum, []string{"Off", "1 minute", "3 minutes", "5 minutes", "10 minutes", "20 minutes", "30 minutes", "60 minutes"}},
	{37, "Sleep Animation", Enum, []string{"Turn LEDs Off", "Rainbow Wave"}},
	{38, "Bank Change Animations", Bool, nil},
	{32, "Indicator Brightness", Number, nil},
	{31, "RGB Brightness", Number, nil},
	{1, "Bank Side Buttons", Bool, nil},
	{2, "Left Top", Enum, sideFunctions},
	{3, "Left Middle", Enum, sideFunctions},
	{4, "Left Bottom", Enum, sideFunctions},
	{5, "Right Top", Enum, sideFunctions},
	{6, "Right Middle", Enum, sideFunctions},
	{7, "Right Bottom", Enum, sideFunctions},
}
