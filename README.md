# mft-tui

A terminal editor for the DJ TechTools Midi Fighter Twister.

mft-tui changes knob settings on many knobs at once. Select knobs, set the values, and apply them in one step. A setting can get the same value on all knobs or a sequence, for example CC 20, 21, 22 and so on. A selection can cross banks.

This software was developed with the help of a LLM.

## Features

- Edit all 14 knob settings: encoder and button MIDI number, channel and type, sensitivity, shift channel, colours, indicator, detent, super knob.
- Select a range of knobs (from, to), or pick single knobs anywhere on the 8 banks.
- For each setting, choose keep, set to (one value), or sequence (start and step).
- See the off, on and detent colours of each knob in the grid.
- Edit the global settings.
- Read the settings from the device and write them to the device.
- Open and save `.mfs` files. The format is the same as in Midi Fighter Utility.

## Requirements

- A Midi Fighter Twister connected by USB.
- Go 1.24.2 or later.
- A C++ compiler. The MIDI driver (RtMidi) compiles from source. On macOS, install the Xcode Command Line Tools.

The tests were done on macOS with Twister firmware 2026-07-02 (4 banks) and 2026-09-17 (8 banks).

## Install

```sh
go install github.com/biomassa/mft-tui@latest
```

The command puts `mft-tui` in `$(go env GOPATH)/bin`. Make sure that this folder is on your `PATH`.

## Use

Quit Midi Fighter Utility before you start mft-tui. Both programs read the same device replies.

```sh
mft-tui                  # read the connected Twister
mft-tui file.mfs         # open a settings file
mft-tui --dir FOLDER     # set the folder that the file picker opens
```

**Warning:** Ctrl+W writes the settings to the device memory (EEPROM). Save a backup with Ctrl+S before you write.

### Keys

| Key | Action |
|--|--|
| Tab, Shift+Tab | Go to the next or previous area: grid, from, to, program, apply |
| Arrows | Move the cursor. The cursor crosses rows and banks. |
| Shift+Arrows | Select a range |
| `[` `]`, `1`–`8`, PgUp, PgDn | Go to a bank |
| `a` | Select the full bank |
| `s` | Pick or unpick the knob at the cursor (pick mode) |
| Esc | End pick mode |
| Enter | Apply all rows that are not on keep |
| Ctrl+R | Read the device |
| Ctrl+W | Write the device |
| Ctrl+O, Ctrl+S | Open or save a `.mfs` file |
| Ctrl+G | Global settings |
| `q`, Ctrl+Q | Quit |

In the program table:

| Key | Action |
|--|--|
| Up, Down | Go to a row |
| Left, Right | Number rows: go to mode, value or step. List rows: change the value. |
| Space | Number rows, mode cell: change the mode. List rows: change the value. |
| Digits, `-`, `+` | Type or change a number |

### Pick mode

Push `s` to pick the knob at the cursor. The "From knob" and "To knob" fields change to "Knobs". Type a list in this field, for example `1 3 5-8 17` or `1,3,5-8,17`. A sequence goes through the knobs in number order. Push Esc or Shift+Arrow to end pick mode.

### Configuration

The file `config.json` sets the folder that the file picker opens:

```json
{"folder": "~/path/to/presets"}
```

The file is in the `mft-tui` folder of your user configuration folder. On macOS, this is `~/Library/Application Support/mft-tui/`. On Linux, it is `~/.config/mft-tui/`. The `--dir` flag overrides this file. If you do not set a folder, the picker opens the current folder.

## Credits

The colour palettes in `internal/mft/palette.go` come from the Midi Fighter Twister firmware by DJ TechTools ([source](https://github.com/DJ-TechTools/Midi_Fighter_Twister_Open_Source)). The firmware licence limits distribution. The DJ TechTools maintainer gave permission to publish them in this project ([forum post](https://forum.djtechtools.com/t/building-a-mft-control-tui-in-go/155183/4)).

mft-tui uses [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Bubbles](https://github.com/charmbracelet/bubbles), [Lip Gloss](https://github.com/charmbracelet/lipgloss) and [gomidi](https://gitlab.com/gomidi/midi).

## Licence

MIT for the code in this repository. See `LICENSE`. The palette tables are not included in the MIT licence.
