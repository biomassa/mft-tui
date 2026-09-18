package mft

// SysEx bodies without F0/F7 (gomidi's midi.SysEx adds them).

var mfrID = []byte{0x00, 0x01, 0x79}

const (
	cmdPushConf = 0x01
	cmdPullConf = 0x02
	cmdSystem   = 0x03
	cmdBulkXfer = 0x04
	cmdDeviceID = 0x05
	cmdNative   = 0x06
)

func sx(cmd byte, body ...byte) []byte {
	return append(append(append([]byte{}, mfrID...), cmd), body...)
}

// wireTag maps slot 1..128 to the tag byte; slot 128 is sent as 0.
func wireTag(slot int) byte { return byte(slot & 0x7F) }

var identityRequest = []byte{0x7E, 0x7F, 0x06, 0x01}

func msgPullGlobal() []byte          { return sx(cmdPullConf, 0x00) }
func msgPushGlobal(g Pairs) []byte   { return sx(cmdPushConf, g.bytes()...) }
func msgPullEncoder(slot int) []byte { return sx(cmdBulkXfer, 0x01, wireTag(slot)) }
func msgSetupQuery() []byte          { return sx(cmdSystem, 0x05, 0x00) }

// msgsPushEncoder splits an encoder's pairs into <=24-byte parts, as the firmware does.
func msgsPushEncoder(slot int, ps Pairs) [][]byte {
	body := ps.bytes()
	var parts [][]byte
	for i := 0; i < len(body); i += 24 {
		end := min(i+24, len(body))
		parts = append(parts, body[i:end])
	}
	if len(parts) == 0 {
		parts = [][]byte{{}}
	}
	msgs := make([][]byte, len(parts))
	for k, c := range parts {
		msgs[k] = sx(cmdBulkXfer, append([]byte{0x00, wireTag(slot), byte(k + 1), byte(len(parts)), byte(len(c))}, c...)...)
	}
	return msgs
}
