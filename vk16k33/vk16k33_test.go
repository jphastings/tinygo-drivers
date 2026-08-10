package vk16k33

import (
	"errors"
	"testing"
)

// fakeDisplay emulates a VK16K33 on the I2C bus: the first written byte is
// either a command (0x20 and up) or a display RAM start address
type fakeDisplay struct {
	addr    uint16
	ram     [ramSize]byte
	cmds    []byte
	flushes int
}

func (f *fakeDisplay) Tx(addr uint16, w, r []byte) error {
	f.addr = addr
	if len(w) == 0 {
		return errors.New("empty write")
	}
	if w[0] >= 0x20 {
		f.cmds = append(f.cmds, w[0])
		return nil
	}
	copy(f.ram[w[0]:], w[1:])
	f.flushes++
	return nil
}

func configured(t *testing.T) (*fakeDisplay, *Device) {
	t.Helper()
	f := &fakeDisplay{}
	d := New(f)
	if err := d.Configure(); err != nil {
		t.Fatal(err)
	}
	return f, &d
}

func TestConfigure(t *testing.T) {
	f, _ := configured(t)

	if f.addr != 0x70 {
		t.Errorf("device address = %#02x, want 0x70", f.addr)
	}

	// Oscillator on, full brightness, display on without blinking
	want := []byte{0x21, 0xEF, 0x81}
	if len(f.cmds) != len(want) {
		t.Fatalf("commands = %X, want %X", f.cmds, want)
	}
	for i := range want {
		if f.cmds[i] != want[i] {
			t.Errorf("command %d = %#02x, want %#02x", i, f.cmds[i], want[i])
		}
	}

	if f.flushes != 1 || f.ram != [ramSize]byte{} {
		t.Errorf("expected one flush of cleared RAM, got %d flushes, RAM %X", f.flushes, f.ram)
	}
}

// Golden display RAM contents, hand-derived from the SparkFun font table
// and their illuminateSegment RAM mapping
func TestWriteStringGolden(t *testing.T) {
	cases := []struct {
		s   string
		ram [ramSize]byte
	}{
		// 'A': segments a,b,c,e,f,g and i (which shares COM 0, bit digit+4)
		{"A", [ramSize]byte{0: 0x11, 2: 0x01, 4: 0x01, 8: 0x01, 10: 0x01, 12: 0x01}},
		// '8': segments a-g and i
		{"8", [ramSize]byte{0: 0x11, 2: 0x01, 4: 0x01, 6: 0x01, 8: 0x01, 10: 0x01, 12: 0x01}},
		// '.' and ':' light dedicated LEDs (RAM 3 and 1, bit 0) and do not
		// take a digit: '1' is on digit 0 and '2' on digit 1
		{"1.2:", [ramSize]byte{0: 0x22, 1: 0x01, 2: 0x03, 3: 0x01, 4: 0x01, 6: 0x12, 8: 0x02, 12: 0x02}},
	}

	for _, c := range cases {
		f, d := configured(t)
		if err := d.WriteString(c.s); err != nil {
			t.Fatal(err)
		}
		if f.ram != c.ram {
			t.Errorf("WriteString(%q) RAM:\ngot  %X\nwant %X", c.s, f.ram, c.ram)
		}
	}
}

func TestWriteStringTruncatesToFourDigits(t *testing.T) {
	f, d := configured(t)

	if err := d.WriteString("8888ABC"); err != nil {
		t.Fatal(err)
	}
	want := [ramSize]byte{0: 0xFF, 2: 0x0F, 4: 0x0F, 6: 0x0F, 8: 0x0F, 10: 0x0F, 12: 0x0F}
	if f.ram != want {
		t.Errorf("RAM:\ngot  %X\nwant %X", f.ram, want)
	}
}

func TestBrightness(t *testing.T) {
	f, d := configured(t)

	if err := d.SetBrightness(4); err != nil {
		t.Fatal(err)
	}
	if got := f.cmds[len(f.cmds)-1]; got != 0xE4 {
		t.Errorf("SetBrightness(4) sent %#02x, want 0xE4", got)
	}

	if err := d.SetBrightness(16); err != errBrightnessOutOfRange {
		t.Errorf("SetBrightness(16) = %v, want errBrightnessOutOfRange", err)
	}
}

func TestBlink(t *testing.T) {
	f, d := configured(t)

	cases := []struct {
		rate BlinkRate
		cmd  byte
	}{
		{BLINK_2HZ, 0x83},
		{BLINK_1HZ, 0x85},
		{BLINK_0_5HZ, 0x87},
		{BLINK_OFF, 0x81},
	}
	for _, c := range cases {
		if err := d.SetBlink(c.rate); err != nil {
			t.Fatal(err)
		}
		if got := f.cmds[len(f.cmds)-1]; got != c.cmd {
			t.Errorf("SetBlink(%d) sent %#02x, want %#02x", c.rate, got, c.cmd)
		}
	}

	if err := d.SetBlink(4); err != errInvalidBlinkRate {
		t.Errorf("SetBlink(4) = %v, want errInvalidBlinkRate", err)
	}
}

func TestBufferedDrawing(t *testing.T) {
	f, d := configured(t)
	flushesAfterConfigure := f.flushes

	d.SetColon(true)
	d.SetDecimalPoint(true)
	if f.flushes != flushesAfterConfigure {
		t.Fatal("drawing should not touch the bus until Display is called")
	}

	if err := d.Display(); err != nil {
		t.Fatal(err)
	}
	if f.ram[1] != 0x01 || f.ram[3] != 0x01 {
		t.Errorf("colon/decimal RAM bytes = %#02x, %#02x, want 0x01, 0x01", f.ram[1], f.ram[3])
	}
}
