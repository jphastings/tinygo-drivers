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

// TestRotationMirrorZeroValueIsIdentity verifies that a device which never
// had SetRotation or SetMirror called behaves exactly as it did before
// either existed.
func TestRotationMirrorZeroValueIsIdentity(t *testing.T) {
	d := NewAdafruitRGBMatrixQT13x9(nil, I2C_ADDRESS_GND)

	if d.Rotation() != Rotation0 {
		t.Errorf("zero-value Rotation() = %v, want Rotation0", d.Rotation())
	}
	if d.Mirror() != MirrorNone {
		t.Errorf("zero-value Mirror() = %v, want MirrorNone", d.Mirror())
	}
	if w, h := d.Size(); w != 13 || h != 9 {
		t.Errorf("zero-value Size() = (%d,%d), want (13,9)", w, h)
	}

	d.SetPixel(0, 0, color.RGBA{R: 10, G: 20, B: 30})
	if got := d.ledValue(242); got != 10 {
		t.Errorf("red channel 242 = %d, want 10", got)
	}
}

// TestSizeReflectsRotation verifies Size swaps width and height at 90 and
// 270 degrees, and that mirroring never affects it.
func TestSizeReflectsRotation(t *testing.T) {
	cases := []struct {
		rotation Rotation
		mirror   Mirror
		w, h     int16
	}{
		{Rotation0, MirrorNone, 13, 9},
		{Rotation90, MirrorNone, 9, 13},
		{Rotation180, MirrorNone, 13, 9},
		{Rotation270, MirrorNone, 9, 13},
		{Rotation0, MirrorHorizontal | MirrorVertical, 13, 9},
		{Rotation90, MirrorHorizontal | MirrorVertical, 9, 13},
	}

	for _, c := range cases {
		d := NewAdafruitRGBMatrixQT13x9(nil, I2C_ADDRESS_GND)
		if err := d.SetRotation(c.rotation); err != nil {
			t.Fatalf("SetRotation(%v): %v", c.rotation, err)
		}
		if err := d.SetMirror(c.mirror); err != nil {
			t.Fatalf("SetMirror(%v): %v", c.mirror, err)
		}
		if w, h := d.Size(); w != c.w || h != c.h {
			t.Errorf("rotation=%v mirror=%v: Size() = (%d,%d), want (%d,%d)", c.rotation, c.mirror, w, h, c.w, c.h)
		}
	}
}

// TestSetPixelRotationCorners pins where each corner of the caller-visible
// frame lands on the silkscreen-upright panel, for each rotation. Corners
// catch transposed-axis and flipped-sign bugs that a centre pixel hides.
//
// Expected channel numbers are the reference-frame corners' channels,
// independently cross-checked against TestAdafruitRGBMatrixQTMappingReference
// (which supplies top-left and bottom-right) and the wiring formula in
// SetPixel (for top-right and bottom-left, which that test doesn't cover):
//
//	top-left     (0,0):  r=242 g=241 b=240
//	top-right   (12,0):  r=349 g=348 b=350
//	bottom-left  (0,8):  r=182 g=181 b=180
//	bottom-right(12,8):  r=331 g=330 b=332
func TestSetPixelRotationCorners(t *testing.T) {
	cases := []struct {
		name     string
		rotation Rotation
		x, y     int16 // caller-frame coordinate under test
		r, g, b  uint16
	}{
		// Rotation0: caller frame is the reference frame, unchanged.
		{"R0 top-left", Rotation0, 0, 0, 242, 241, 240},
		{"R0 top-right", Rotation0, 12, 0, 349, 348, 350},
		{"R0 bottom-left", Rotation0, 0, 8, 182, 181, 180},
		{"R0 bottom-right", Rotation0, 12, 8, 331, 330, 332},

		// Rotation90 (mounted 90 degrees clockwise): caller frame is 9
		// wide, 13 tall. Caller top-left lands on reference bottom-left.
		{"R90 top-left", Rotation90, 0, 0, 182, 181, 180},
		{"R90 top-right", Rotation90, 8, 0, 242, 241, 240},
		{"R90 bottom-left", Rotation90, 0, 12, 331, 330, 332},
		{"R90 bottom-right", Rotation90, 8, 12, 349, 348, 350},

		// Rotation180: caller frame is the reference frame, corners opposite.
		{"R180 top-left", Rotation180, 0, 0, 331, 330, 332},
		{"R180 top-right", Rotation180, 12, 0, 182, 181, 180},
		{"R180 bottom-left", Rotation180, 0, 8, 349, 348, 350},
		{"R180 bottom-right", Rotation180, 12, 8, 242, 241, 240},

		// Rotation270 (mounted 270 degrees clockwise, i.e. 90 counter-
		// clockwise): caller frame is 9 wide, 13 tall, mirror image of R90.
		{"R270 top-left", Rotation270, 0, 0, 349, 348, 350},
		{"R270 top-right", Rotation270, 8, 0, 331, 330, 332},
		{"R270 bottom-left", Rotation270, 0, 12, 242, 241, 240},
		{"R270 bottom-right", Rotation270, 8, 12, 182, 181, 180},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := NewAdafruitRGBMatrixQT13x9(nil, I2C_ADDRESS_GND)
			if err := d.SetRotation(c.rotation); err != nil {
				t.Fatalf("SetRotation: %v", err)
			}
			d.SetPixel(c.x, c.y, color.RGBA{R: 10, G: 20, B: 30})

			if got := d.ledValue(c.r); got != 10 {
				t.Errorf("red channel %d = %d, want 10", c.r, got)
			}
			if got := d.ledValue(c.g); got != 20 {
				t.Errorf("green channel %d = %d, want 20", c.g, got)
			}
			if got := d.ledValue(c.b); got != 30 {
				t.Errorf("blue channel %d = %d, want 30", c.b, got)
			}
		})
	}
}

// TestSetPixelMirror pins each mirror axis alone, at Rotation0, using the
// same reference-frame corner channels as TestSetPixelRotationCorners.
func TestSetPixelMirror(t *testing.T) {
	cases := []struct {
		name    string
		mirror  Mirror
		x, y    int16
		r, g, b uint16
	}{
		{"MirrorHorizontal top-left", MirrorHorizontal, 0, 0, 349, 348, 350},
		{"MirrorHorizontal top-right", MirrorHorizontal, 12, 0, 242, 241, 240},
		{"MirrorHorizontal bottom-left", MirrorHorizontal, 0, 8, 331, 330, 332},
		{"MirrorHorizontal bottom-right", MirrorHorizontal, 12, 8, 182, 181, 180},

		{"MirrorVertical top-left", MirrorVertical, 0, 0, 182, 181, 180},
		{"MirrorVertical top-right", MirrorVertical, 12, 0, 331, 330, 332},
		{"MirrorVertical bottom-left", MirrorVertical, 0, 8, 242, 241, 240},
		{"MirrorVertical bottom-right", MirrorVertical, 12, 8, 349, 348, 350},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := NewAdafruitRGBMatrixQT13x9(nil, I2C_ADDRESS_GND)
			if err := d.SetMirror(c.mirror); err != nil {
				t.Fatalf("SetMirror: %v", err)
			}
			d.SetPixel(c.x, c.y, color.RGBA{R: 10, G: 20, B: 30})

			if got := d.ledValue(c.r); got != 10 {
				t.Errorf("red channel %d = %d, want 10", c.r, got)
			}
			if got := d.ledValue(c.g); got != 20 {
				t.Errorf("green channel %d = %d, want 20", c.g, got)
			}
			if got := d.ledValue(c.b); got != 30 {
				t.Errorf("blue channel %d = %d, want 30", c.b, got)
			}
		})
	}
}

// TestSetPixelRotationAndMirror pins two rotation-and-mirror combinations,
// since the two must compose in a specific order: mirror in the caller's
// frame first, then rotate into panel space.
//
// The Rotation90|MirrorHorizontal case doubles as a guard against that
// pipeline being silently reversed: if mirroring were instead applied after
// rotating (in the panel's reference frame), the caller's top-left corner
// would land on reference bottom-right (channel 331/330/332) rather than
// top-left (242/241/240).
func TestSetPixelRotationAndMirror(t *testing.T) {
	cases := []struct {
		name     string
		rotation Rotation
		mirror   Mirror
		x, y     int16
		r, g, b  uint16
	}{
		{"R90|MirrorHorizontal top-left", Rotation90, MirrorHorizontal, 0, 0, 242, 241, 240},
		{"R90|MirrorHorizontal top-right", Rotation90, MirrorHorizontal, 8, 0, 182, 181, 180},
		{"R90|MirrorHorizontal bottom-left", Rotation90, MirrorHorizontal, 0, 12, 349, 348, 350},
		{"R90|MirrorHorizontal bottom-right", Rotation90, MirrorHorizontal, 8, 12, 331, 330, 332},

		{"R90|MirrorVertical top-left", Rotation90, MirrorVertical, 0, 0, 331, 330, 332},
		{"R90|MirrorVertical top-right", Rotation90, MirrorVertical, 8, 0, 349, 348, 350},
		{"R90|MirrorVertical bottom-left", Rotation90, MirrorVertical, 0, 12, 182, 181, 180},
		{"R90|MirrorVertical bottom-right", Rotation90, MirrorVertical, 8, 12, 242, 241, 240},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := NewAdafruitRGBMatrixQT13x9(nil, I2C_ADDRESS_GND)
			if err := d.SetRotation(c.rotation); err != nil {
				t.Fatalf("SetRotation: %v", err)
			}
			if err := d.SetMirror(c.mirror); err != nil {
				t.Fatalf("SetMirror: %v", err)
			}
			d.SetPixel(c.x, c.y, color.RGBA{R: 10, G: 20, B: 30})

			if got := d.ledValue(c.r); got != 10 {
				t.Errorf("red channel %d = %d, want 10", c.r, got)
			}
			if got := d.ledValue(c.g); got != 20 {
				t.Errorf("green channel %d = %d, want 20", c.g, got)
			}
			if got := d.ledValue(c.b); got != 30 {
				t.Errorf("blue channel %d = %d, want 30", c.b, got)
			}
		})
	}
}

// TestMirrorBothAxesEqualsRotation180 confirms that mirroring both axes is
// the same transform as a half turn, across every pixel, not just corners.
func TestMirrorBothAxesEqualsRotation180(t *testing.T) {
	rotated := NewAdafruitRGBMatrixQT13x9(nil, I2C_ADDRESS_GND)
	if err := rotated.SetRotation(Rotation180); err != nil {
		t.Fatal(err)
	}

	mirrored := NewAdafruitRGBMatrixQT13x9(nil, I2C_ADDRESS_GND)
	if err := mirrored.SetMirror(MirrorHorizontal | MirrorVertical); err != nil {
		t.Fatal(err)
	}

	for x := int16(0); x < 13; x++ {
		for y := int16(0); y < 9; y++ {
			c := color.RGBA{R: uint8(x + 1), G: uint8(y + 1), B: 1}
			rotated.SetPixel(x, y, c)
			mirrored.SetPixel(x, y, c)
		}
	}

	for channel := uint16(0); channel < LEDCount; channel++ {
		if got, want := mirrored.ledValue(channel), rotated.ledValue(channel); got != want {
			t.Errorf("channel %d: MirrorHorizontal|MirrorVertical = %d, Rotation180 = %d, want equal", channel, got, want)
		}
	}
}

// TestSetPixelOutOfRangeAllOrientations verifies out-of-range pixels stay a
// no-op under every valid rotation and mirror combination, using each
// combination's own Size() as the boundary.
func TestSetPixelOutOfRangeAllOrientations(t *testing.T) {
	rotations := []Rotation{Rotation0, Rotation90, Rotation180, Rotation270}
	mirrors := []Mirror{MirrorNone, MirrorHorizontal, MirrorVertical, MirrorHorizontal | MirrorVertical}

	for _, r := range rotations {
		for _, m := range mirrors {
			d := NewAdafruitRGBMatrixQT13x9(nil, I2C_ADDRESS_GND)
			if err := d.SetRotation(r); err != nil {
				t.Fatalf("SetRotation(%v): %v", r, err)
			}
			if err := d.SetMirror(m); err != nil {
				t.Fatalf("SetMirror(%v): %v", m, err)
			}

			w, h := d.Size()
			for _, p := range [][2]int16{{-1, 0}, {0, -1}, {w, 0}, {0, h}} {
				d.SetPixel(p[0], p[1], color.RGBA{R: 255, G: 255, B: 255})
			}

			for channel := uint16(0); channel < LEDCount; channel++ {
				if d.ledValue(channel) != 0 {
					t.Errorf("rotation=%v mirror=%v: channel %d = %d, want 0", r, m, channel, d.ledValue(channel))
				}
			}
		}
	}
}

// TestSetRotationInvalid verifies out-of-range rotations are rejected and
// leave the previous rotation in place.
func TestSetRotationInvalid(t *testing.T) {
	d := NewAdafruitRGBMatrixQT13x9(nil, I2C_ADDRESS_GND)

	if err := d.SetRotation(Rotation(4)); err != errInvalidRotation {
		t.Errorf("SetRotation(4) = %v, want errInvalidRotation", err)
	}
	if d.Rotation() != Rotation0 {
		t.Errorf("Rotation() after rejected SetRotation = %v, want unchanged Rotation0", d.Rotation())
	}
}

// TestSetMirrorInvalid verifies mirror bitmasks outside the two defined
// flags are rejected and leave the previous mirror setting in place.
func TestSetMirrorInvalid(t *testing.T) {
	d := NewAdafruitRGBMatrixQT13x9(nil, I2C_ADDRESS_GND)

	if err := d.SetMirror(Mirror(4)); err != errInvalidMirror {
		t.Errorf("SetMirror(4) = %v, want errInvalidMirror", err)
	}
	if d.Mirror() != MirrorNone {
		t.Errorf("Mirror() after rejected SetMirror = %v, want unchanged MirrorNone", d.Mirror())
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
