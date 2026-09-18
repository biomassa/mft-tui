package mft

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv"
)

const portName = "Midi Fighter Twister"

// Device is an open connection to a Twister. Replies are matched by prefix,
// so other clients on the port (e.g. the Utility) don't confuse it.
type Device struct {
	in   drivers.In
	send func(midi.Message) error
	stop func()
	rx   chan []byte
	Port string
}

type Info struct {
	Firmware string // YYYY-MM-DD
	Banks    int    // 4 or 8
}

func findPort[T interface{ String() string }](ports []T) (T, bool) {
	for _, p := range ports {
		if strings.Contains(p.String(), portName) {
			return p, true
		}
	}
	var zero T
	return zero, false
}

func Open() (*Device, error) {
	in, ok := findPort(midi.GetInPorts())
	if !ok {
		return nil, errors.New("no Midi Fighter Twister MIDI input found")
	}
	out, ok := findPort(midi.GetOutPorts())
	if !ok {
		return nil, errors.New("no Midi Fighter Twister MIDI output found")
	}
	send, err := midi.SendTo(out)
	if err != nil {
		return nil, err
	}
	d := &Device{in: in, send: send, rx: make(chan []byte, 1024), Port: in.String()}
	d.stop, err = midi.ListenTo(in, func(msg midi.Message, _ int32) {
		b := msg.Bytes()
		if len(b) > 2 && b[0] == 0xF0 {
			body := bytes.TrimSuffix(b[1:], []byte{0xF7})
			select {
			case d.rx <- append([]byte(nil), body...):
			default: // drop if nobody is reading
			}
		}
	}, midi.UseSysEx())
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Device) Close() {
	if d.stop != nil {
		d.stop()
	}
}

// CloseDriver releases the MIDI driver; call once at program exit.
func CloseDriver() { midi.CloseDriver() }

func (d *Device) sendSysEx(body []byte) error { return d.send(midi.SysEx(body)) }

func (d *Device) drain() {
	for {
		select {
		case <-d.rx:
		default:
			return
		}
	}
}

// collect gathers replies accepted by match until done() says so or the
// timeout passes without a matching reply.
func (d *Device) collect(match func([]byte) bool, done func([][]byte) bool, timeout time.Duration) [][]byte {
	var got [][]byte
	t := time.NewTimer(timeout)
	defer t.Stop()
	for {
		select {
		case b := <-d.rx:
			if match(b) {
				got = append(got, b)
				if done != nil && done(got) {
					return got
				}
				t.Reset(timeout)
			}
		case <-t.C:
			return got
		}
	}
}

func hasPrefix(b []byte, p ...byte) bool { return bytes.HasPrefix(b, p) }

func (d *Device) Info() (Info, error) {
	d.drain()
	if err := d.sendSysEx(identityRequest); err != nil {
		return Info{}, err
	}
	r := d.collect(func(b []byte) bool { return hasPrefix(b, 0x7E, 0x7F, 0x06, 0x02) && len(b) >= 15 },
		func(g [][]byte) bool { return true }, 800*time.Millisecond)
	if len(r) == 0 {
		return Info{}, errors.New("no identity reply")
	}
	b := r[0]
	info := Info{Firmware: fmt.Sprintf("%02x%02x-%02x-%02x", b[11], b[12], b[13], b[14]), Banks: 4}

	// Bank count: slot 65 answers with data only on 8-bank firmware.
	d.drain()
	if err := d.sendSysEx(msgPullEncoder(65)); err != nil {
		return info, err
	}
	rs := d.collect(func(b []byte) bool { return hasPrefix(b, 0x00, 0x01, 0x79, cmdBulkXfer, 0x00, 65) && len(b) >= 9 },
		func(g [][]byte) bool { return true }, 500*time.Millisecond)
	if len(rs) > 0 && rs[0][8] > 0 {
		info.Banks = 8
	}
	return info, nil
}

func (d *Device) PullGlobal() (Pairs, error) {
	d.drain()
	if err := d.sendSysEx(msgPullGlobal()); err != nil {
		return nil, err
	}
	r := d.collect(func(b []byte) bool { return hasPrefix(b, 0x00, 0x01, 0x79, cmdPullConf, 0x01) },
		func(g [][]byte) bool { return true }, 800*time.Millisecond)
	if len(r) == 0 {
		return nil, errors.New("no reply to global config request")
	}
	return parsePairs(r[0][5:])
}

// PullEncoder reads one slot. The device answers the last slot (64 or 128) as tag 0.
func (d *Device) PullEncoder(slot, slots int) (Pairs, error) {
	want := wireTag(slot)
	alt := want
	if slot == slots {
		alt = 0
	}
	d.drain()
	if err := d.sendSysEx(msgPullEncoder(slot)); err != nil {
		return nil, err
	}
	parts := map[byte][]byte{}
	var total byte
	d.collect(func(b []byte) bool {
		if !hasPrefix(b, 0x00, 0x01, 0x79, cmdBulkXfer, 0x00) || len(b) < 9 || (b[5] != want && b[5] != alt) {
			return false
		}
		part, tot, size := b[6], b[7], int(b[8])
		if len(b) < 9+size {
			return false
		}
		parts[part] = b[9 : 9+size]
		total = tot
		return true
	}, func([][]byte) bool { return total > 0 && len(parts) >= int(total) }, 500*time.Millisecond)
	if total == 0 || len(parts) < int(total) {
		return nil, fmt.Errorf("slot %d: incomplete reply", slot)
	}
	var body []byte
	for k := byte(1); k <= total; k++ {
		body = append(body, parts[k]...)
	}
	return parsePairs(body)
}

// PullPreset reads the global table and every slot. progress may be nil.
func (d *Device) PullPreset(slots int, progress func(done, total int)) (*Preset, error) {
	p := NewPreset()
	var err error
	if p.Globals, err = d.PullGlobal(); err != nil {
		return nil, err
	}
	for n := 1; n <= slots; n++ {
		if p.Encoders[n], err = d.PullEncoder(n, slots); err != nil {
			return nil, err
		}
		if progress != nil {
			progress(n, slots)
		}
	}
	return p, nil
}

// Push writes the given slots, then the global table. The global push makes
// the device reload and redraw (encoder pushes blank the display).
func (d *Device) Push(p *Preset, slots []int, progress func(done, total int)) error {
	for i, n := range slots {
		for _, m := range msgsPushEncoder(n, p.Encoders[n]) {
			if err := d.sendSysEx(m); err != nil {
				return err
			}
			time.Sleep(15 * time.Millisecond)
		}
		if progress != nil {
			progress(i+1, len(slots))
		}
	}
	d.drain()
	if err := d.sendSysEx(msgPushGlobal(p.Globals)); err != nil {
		return err
	}
	// The device answers a global push with its config once EEPROM is written.
	d.collect(func(b []byte) bool { return hasPrefix(b, 0x00, 0x01, 0x79, cmdPullConf, 0x01) },
		func([][]byte) bool { return true }, 3*time.Second)
	return nil
}
