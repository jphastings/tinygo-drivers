package st25dv

import (
	"bytes"
	"errors"
	"testing"
)

// fakeTag emulates an ST25DV16K on the I2C bus: 2KB user memory with 16-bit
// addressing, a system area, and NACKs while an EEPROM write cycle runs
type fakeTag struct {
	user   [2048]byte
	sys    [0x30]byte
	busy   int // transactions to NACK after a write, like a real write cycle
	writes int // number of user memory write transactions
}

func newFakeTag() *fakeTag {
	f := &fakeTag{}
	// ST25DV16K memory size: 512 blocks (0x01FF last block) of 4 bytes
	f.sys[REG_MEM_SIZE_LSB] = 0xFF
	f.sys[REG_MEM_SIZE_MSB] = 0x01
	f.sys[REG_BLK_SIZE] = 0x03
	// UID, stored least significant byte first
	copy(f.sys[REG_UID:], []byte{0x12, 0x34, 0x56, 0x78, 0x9A, 0x25, 0x02, 0xE0})
	return f
}

func (f *fakeTag) Tx(addr uint16, w, r []byte) error {
	switch uint8(addr) {
	case AddressUser:
		if f.busy > 0 {
			f.busy--
			return errors.New("NACK: write cycle in progress")
		}
		if len(w) >= 2 {
			p := int(w[0])<<8 | int(w[1])
			if len(w) > 2 {
				copy(f.user[p:], w[2:])
				f.busy = 3
				f.writes++
			} else {
				copy(r, f.user[p:])
			}
		}
		return nil
	case AddressSystem:
		if len(w) >= 2 {
			copy(r, f.sys[int(w[0])<<8|int(w[1]):])
		}
		return nil
	}
	return errors.New("unknown I2C address")
}

func configured(t *testing.T) (*fakeTag, *Device) {
	t.Helper()
	tag := newFakeTag()
	d := New(tag)
	if err := d.Configure(); err != nil {
		t.Fatal(err)
	}
	return tag, &d
}

func TestConfigure(t *testing.T) {
	_, d := configured(t)

	if !d.Connected() {
		t.Error("expected device to be connected")
	}
	if d.Size() != 2048 {
		t.Errorf("Size() = %d, want 2048", d.Size())
	}

	uid, err := d.UID()
	if err != nil {
		t.Fatal(err)
	}
	want := [8]byte{0xE0, 0x02, 0x25, 0x9A, 0x78, 0x56, 0x34, 0x12}
	if uid != want {
		t.Errorf("UID() = %X, want %X", uid, want)
	}
}

func TestReadWriteRoundTrip(t *testing.T) {
	tag, d := configured(t)

	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i)
	}

	n, err := d.WriteAt(data, 37)
	if err != nil || n != len(data) {
		t.Fatalf("WriteAt = %d, %v", n, err)
	}
	if tag.writes != 4 {
		t.Errorf("100 bytes written in %d transactions, want 4 chunks", tag.writes)
	}

	got := make([]byte, len(data))
	n, err = d.ReadAt(got, 37)
	if err != nil || n != len(data) {
		t.Fatalf("ReadAt = %d, %v", n, err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("read back %X, want %X", got, data)
	}
}

func TestBounds(t *testing.T) {
	_, d := configured(t)

	buf := make([]byte, 16)
	if _, err := d.WriteAt(buf, 2048-8); err != errOutOfRange {
		t.Errorf("write past the end: %v, want errOutOfRange", err)
	}
	if _, err := d.ReadAt(buf, -1); err != errOutOfRange {
		t.Errorf("read at negative offset: %v, want errOutOfRange", err)
	}

	unconfigured := New(newFakeTag())
	if _, err := unconfigured.WriteAt(buf, 0); err != errNotConfigured {
		t.Errorf("write before Configure: %v, want errNotConfigured", err)
	}
}

// TestWriteNDEFURI checks the exact tag memory layout against the NFC Forum
// Type 5 Tag and URI RTD specifications
func TestWriteNDEFURI(t *testing.T) {
	tag, d := configured(t)

	if err := d.WriteNDEFURI("https://tinygo.org"); err != nil {
		t.Fatal(err)
	}

	want := []byte{
		// 8-byte capability container: 2048 bytes = 0x0100 8-byte blocks
		0xE2, 0x40, 0x00, 0x05, 0x00, 0x00, 0x01, 0x00,
		// NDEF message TLV, 15 byte long message
		0x03, 0x0F,
		// URI record: header, type length, payload length, type 'U'
		0xD1, 0x01, 0x0B, 0x55,
		// URI with abbreviated https:// prefix
		0x04, 't', 'i', 'n', 'y', 'g', 'o', '.', 'o', 'r', 'g',
		// terminator TLV
		0xFE,
	}
	if got := tag.user[:len(want)]; !bytes.Equal(got, want) {
		t.Errorf("tag memory:\ngot  %X\nwant %X", got, want)
	}
}

// TestWriteNDEFMessageRespectsExistingCCWidth checks the TLV is placed
// according to the CC already on the tag, not this driver's own size-based
// guess: a tag can be validly formatted with either CC width for its
// capacity, and guessing wrong would misalign the TLV against the real CC.
func TestWriteNDEFMessageRespectsExistingCCWidth(t *testing.T) {
	tag, d := configured(t)

	// A compact 4-byte CC, even though this 2048-byte tag's own size would
	// make the driver choose an 8-byte CC when formatting from scratch.
	cc := []byte{0xE1, 0x40, 0x40, 0x05}
	copy(tag.user[:], cc)

	if err := d.WriteNDEFMessage([]byte{0xD1, 0x01, 0x00, 'T'}); err != nil {
		t.Fatal(err)
	}

	if got := tag.user[:4]; !bytes.Equal(got, cc) {
		t.Errorf("existing CC was overwritten: got %X, want %X", got, cc)
	}
	if tag.user[4] != tlvNDEFMessage {
		t.Errorf("TLV written at wrong offset: tag.user[4] = %#02x, want the NDEF TLV type %#02x", tag.user[4], tlvNDEFMessage)
	}
}

func TestAbbreviateURI(t *testing.T) {
	cases := []struct {
		uri  string
		code uint8
		rest string
	}{
		{"https://tinygo.org", 0x04, "tinygo.org"},
		{"https://www.example.com", 0x02, "example.com"}, // longest prefix wins
		{"tel:+123456", 0x05, "+123456"},
		{"urn:epc:id:x", 0x1E, "x"},
		{"geo:52.2,0.1", 0x00, "geo:52.2,0.1"}, // no known prefix
	}

	for _, c := range cases {
		code, rest := abbreviateURI(c.uri)
		if code != c.code || rest != c.rest {
			t.Errorf("abbreviateURI(%q) = %#02x, %q; want %#02x, %q",
				c.uri, code, rest, c.code, c.rest)
		}
	}
}
