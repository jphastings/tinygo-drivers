package is31fl3741

import (
	"image/color"
	"testing"
)

// ledValue reads a channel's PWM value back out of the internal buffers
func (d *Device) ledValue(channel uint16) uint8 {
	if channel < firstPageLEDCount {
		return d.buf1[1+channel]
	}
	return d.buf2[1+channel-firstPageLEDCount]
}

// TestAdafruitRGBMatrixQTMappingCoverage verifies that the 13x9x3 pixel
// channels map onto all 351 chip channels with no collisions
func TestAdafruitRGBMatrixQTMappingCoverage(t *testing.T) {
	d := NewAdafruitRGBMatrixQT13x9(nil, I2C_ADDRESS_GND)

	for x := int16(0); x < 13; x++ {
		for y := int16(0); y < 9; y++ {
			d.SetPixel(x, y, color.RGBA{R: 1, G: 1, B: 1})
		}
	}

	for channel := uint16(0); channel < LEDCount; channel++ {
		if d.ledValue(channel) != 1 {
			t.Errorf("channel %d written %d times, want exactly once", channel, d.ledValue(channel))
		}
	}
}

// TestAdafruitRGBMatrixQTMappingReference spot-checks pixel to channel
// mapping against values computed with the reference implementation:
// https://github.com/adafruit/Adafruit_CircuitPython_IS31FL3741
func TestAdafruitRGBMatrixQTMappingReference(t *testing.T) {
	cases := []struct {
		x, y    int16
		r, g, b uint16
	}{
		{x: 0, y: 0, r: 242, g: 241, b: 240},  // even column
		{x: 1, y: 6, r: 4, g: 3, b: 5},        // odd column
		{x: 9, y: 3, r: 118, g: 117, b: 119},  // odd, last column of first block
		{x: 10, y: 0, r: 344, g: 343, b: 342}, // even, second block
		{x: 12, y: 8, r: 331, g: 330, b: 332}, // last column special case
	}

	for _, c := range cases {
		d := NewAdafruitRGBMatrixQT13x9(nil, I2C_ADDRESS_GND)
		d.SetPixel(c.x, c.y, color.RGBA{R: 10, G: 20, B: 30})

		if got := d.ledValue(c.r); got != 10 {
			t.Errorf("pixel (%d,%d): red channel %d = %d, want 10", c.x, c.y, c.r, got)
		}
		if got := d.ledValue(c.g); got != 20 {
			t.Errorf("pixel (%d,%d): green channel %d = %d, want 20", c.x, c.y, c.g, got)
		}
		if got := d.ledValue(c.b); got != 30 {
			t.Errorf("pixel (%d,%d): blue channel %d = %d, want 30", c.x, c.y, c.b, got)
		}
	}
}

// TestSetPixelOutOfRange verifies pixels outside the matrix are ignored
func TestSetPixelOutOfRange(t *testing.T) {
	d := NewAdafruitRGBMatrixQT13x9(nil, I2C_ADDRESS_GND)

	for _, p := range [][2]int16{{-1, 0}, {0, -1}, {13, 0}, {0, 9}} {
		d.SetPixel(p[0], p[1], color.RGBA{R: 255, G: 255, B: 255})
	}

	for channel := uint16(0); channel < LEDCount; channel++ {
		if d.ledValue(channel) != 0 {
			t.Errorf("channel %d = %d, want 0", channel, d.ledValue(channel))
		}
	}
}

// busRecorder records I2C transactions
type busRecorder struct {
	writes [][]byte
}

func (b *busRecorder) Tx(addr uint16, w, r []byte) error {
	b.writes = append(b.writes, append([]byte{}, w...))
	return nil
}

// TestDisplay verifies the buffered PWM values are sent to the two PWM
// pages, each page selection preceded by an unlock
func TestDisplay(t *testing.T) {
	bus := &busRecorder{}
	d := New(bus, I2C_ADDRESS_GND)

	d.SetLED(0, 11)
	d.SetLED(179, 22)
	d.SetLED(180, 33)
	d.SetLED(350, 44)

	if err := d.Display(); err != nil {
		t.Fatal(err)
	}

	want := [][]byte{
		{COMMAND_WRITE_LOCK, COMMAND_UNLOCK},
		{COMMAND, PAGE_PWM_1},
		nil, // first PWM page data, checked separately
		{COMMAND_WRITE_LOCK, COMMAND_UNLOCK},
		{COMMAND, PAGE_PWM_2},
		nil, // second PWM page data, checked separately
	}
	if len(bus.writes) != len(want) {
		t.Fatalf("got %d writes, want %d", len(bus.writes), len(want))
	}
	for i, w := range want {
		if w == nil {
			continue
		}
		if len(bus.writes[i]) != 2 || bus.writes[i][0] != w[0] || bus.writes[i][1] != w[1] {
			t.Errorf("write %d = %#v, want %#v", i, bus.writes[i], w)
		}
	}

	page1 := bus.writes[2]
	if len(page1) != 1+180 || page1[0] != 0x00 || page1[1] != 11 || page1[180] != 22 {
		t.Errorf("unexpected first PWM page write: len=%d start=%#x", len(page1), page1[0])
	}
	page2 := bus.writes[5]
	if len(page2) != 1+171 || page2[0] != 0x00 || page2[1] != 33 || page2[171] != 44 {
		t.Errorf("unexpected second PWM page write: len=%d start=%#x", len(page2), page2[0])
	}
}
