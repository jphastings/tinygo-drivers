package trustm

import (
	"bytes"
	"errors"
	"testing"
)

// TestCRC16 pins the frame check sequence to known vectors. The Infineon
// algorithm is CRC-16/KERMIT, whose published check value for "123456789"
// is 0x2189. The second vector is the OpenApplication example frame from
// the Infineon I2C protocol specification, whose CRC is 0x041A.
func TestCRC16(t *testing.T) {
	if got := crc16([]byte("123456789")); got != 0x2189 {
		t.Errorf("crc16(check vector) = %#04X, want 0x2189", got)
	}

	frame := []byte{
		0x03, 0x00, 0x15, 0x00, 0x70, 0x00, 0x00, 0x10,
		0xD2, 0x76, 0x00, 0x00, 0x04, 0x47, 0x65, 0x6E,
		0x41, 0x75, 0x74, 0x68, 0x41, 0x70, 0x70, 0x6C,
	}
	if got := crc16(frame); got != 0x041A {
		t.Errorf("crc16(OpenApplication frame) = %#04X, want 0x041A", got)
	}
}

// TestGetRandomFrame checks the exact bytes written to the DATA register
// for a GetRandom command on a fresh link: frame control 0x03 (frame 0,
// acknowledging 3), payload length 7, packet control 0, APDU, CRC
func TestGetRandomFrame(t *testing.T) {
	chip := newFakeChip()
	d := New(chip)
	d.frameSize = 0x110
	if err := d.GetRandom(make([]byte, 16)); err != nil {
		t.Fatal(err)
	}

	want := []byte{
		0x80, // DATA register
		0x03, 0x00, 0x07,
		0x00,                               // PCTR: no chaining
		0x8C, 0x00, 0x00, 0x02, 0x00, 0x10, // GetRandom TRNG, 16 bytes
		0x79, 0x08, // CRC
	}
	if len(chip.frameWrites) == 0 || !bytes.Equal(chip.frameWrites[0], want) {
		t.Errorf("frame written = %X, want %X", chip.frameWrites[0], want)
	}
}

func TestConfigureAndCommands(t *testing.T) {
	chip := newFakeChip()
	d := New(chip)

	if !d.Connected() {
		t.Error("expected device to be connected")
	}
	if err := d.Configure(); err != nil {
		t.Fatal(err)
	}
	if !chip.opened {
		t.Error("Configure did not open the application")
	}

	rnd := make([]byte, 16)
	if err := d.GetRandom(rnd); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rnd, chip.random[:16]) {
		t.Errorf("GetRandom() = %X, want %X", rnd, chip.random[:16])
	}

	uid, err := d.UID()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(uid[:], chip.uid[:]) {
		t.Errorf("UID() = %X, want %X", uid, chip.uid)
	}
}

func TestGetRandomLengthLimits(t *testing.T) {
	chip := newFakeChip()
	d := New(chip)
	if err := d.Configure(); err != nil {
		t.Fatal(err)
	}

	// Below the chip's 8-byte minimum, the driver requests more and
	// truncates
	rnd := make([]byte, 3)
	if err := d.GetRandom(rnd); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rnd, chip.random[:3]) {
		t.Errorf("GetRandom() = %X, want %X", rnd, chip.random[:3])
	}

	if err := d.GetRandom(make([]byte, 257)); err == nil {
		t.Error("expected an error for a 257 byte request")
	}
}

func TestCommandsBeforeConfigure(t *testing.T) {
	d := New(newFakeChip())
	if err := d.GetRandom(make([]byte, 16)); err == nil {
		t.Error("expected an error before Configure")
	}
}

// fakeChip emulates the OPTIGA Trust M I2C register set and protocol: it
// ACKs valid data frames, answers OpenApplication, GetRandom and
// GetDataObject, and exposes what was written for inspection
type fakeChip struct {
	lastReg     byte     // register selected by the last 1-byte write
	pending     [][]byte // frames waiting to be read from DATA
	seq         uint8    // the chip's own frame sequence counter
	opened      bool
	frameWrites [][]byte // every write to the DATA register, with register byte

	random [maxRandomLength]byte
	uid    [UIDLength]byte
}

func newFakeChip() *fakeChip {
	f := &fakeChip{seq: seqInit}
	for i := range f.random {
		f.random[i] = byte(0xA5 ^ i)
	}
	for i := range f.uid {
		f.uid[i] = byte(0xC0 + i)
	}
	return f
}

func (f *fakeChip) Tx(addr uint16, w, r []byte) error {
	if addr != Address {
		return errors.New("NACK: unknown I2C address")
	}
	if len(w) > 0 {
		f.lastReg = w[0]
		if f.lastReg == REG_DATA && len(w) > 1 {
			f.frameWrites = append(f.frameWrites, append([]byte(nil), w...))
			return f.receiveFrame(w[1:])
		}
		return nil
	}

	switch f.lastReg {
	case REG_I2C_STATE:
		r[0] = stateSoftReset
		if len(f.pending) > 0 {
			r[0] |= stateResponseReady
			r[2] = byte(len(f.pending[0]) >> 8)
			r[3] = byte(len(f.pending[0]))
		}
	case REG_DATA_REG_LEN:
		r[0], r[1] = 0x01, 0x10
	case REG_DATA:
		if len(f.pending) == 0 {
			return errors.New("NACK: no frame pending")
		}
		copy(r, f.pending[0])
		f.pending = f.pending[1:]
	}
	return nil
}

func (f *fakeChip) receiveFrame(frame []byte) error {
	if len(frame) < dlOverhead {
		return errors.New("frame too short")
	}
	crc := uint16(frame[len(frame)-2])<<8 | uint16(frame[len(frame)-1])
	if crc16(frame[:len(frame)-2]) != crc {
		return errors.New("bad frame CRC")
	}
	fctr := frame[0]
	if fctr&fctrControlFrame != 0 {
		return nil // an ACK for our data frame, nothing to answer
	}

	hostSeq := fctr >> fctrFrameNrPos & fctrAckNrMask
	apdu := frame[4 : len(frame)-2] // skip header and PCTR
	var out []byte
	switch apdu[0] {
	case CMD_OPEN_APPLICATION:
		f.opened = true
	case CMD_GET_RANDOM:
		out = f.random[:int(apdu[4])<<8|int(apdu[5])]
	case CMD_GET_DATA_OBJECT:
		if int(apdu[4])<<8|int(apdu[5]) != int(OID_COPROCESSOR_UID) {
			return errors.New("unexpected OID")
		}
		n := int(apdu[8])<<8 | int(apdu[9])
		if n > len(f.uid) {
			n = len(f.uid)
		}
		out = f.uid[:n]
	default:
		return errors.New("unexpected command")
	}

	// Acknowledge with a control frame, then queue the response APDU
	f.queue(fctrControlFrame|hostSeq, nil)
	f.seq = (f.seq + 1) & fctrAckNrMask
	resp := append([]byte{0x00, 0x00, byte(len(out) >> 8), byte(len(out))}, out...)
	f.queue(f.seq<<fctrFrameNrPos|hostSeq, resp)
	return nil
}

func (f *fakeChip) queue(fctr byte, apdu []byte) {
	payloadLen := 0
	if apdu != nil {
		payloadLen = 1 + len(apdu) // PCTR byte
	}
	frame := []byte{fctr, byte(payloadLen >> 8), byte(payloadLen)}
	if apdu != nil {
		frame = append(frame, pctrChainNone)
		frame = append(frame, apdu...)
	}
	crc := crc16(frame)
	frame = append(frame, byte(crc>>8), byte(crc))
	f.pending = append(f.pending, frame)
}
