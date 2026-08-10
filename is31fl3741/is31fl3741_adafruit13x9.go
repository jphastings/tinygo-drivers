package is31fl3741

import (
	"image/color"

	"tinygo.org/x/drivers"
)

// DeviceAdafruitRGBMatrixQT13x9 implements TinyGo driver for Lumissil
// IS31FL3741 matrix LED driver on Adafruit 13x9 PWM RGB LED Matrix Driver
// (IS31FL3741 QT) board: https://www.adafruit.com/product/5201
//
// It implements the drivers.Displayer interface: (0, 0) is the top left
// pixel when the board is oriented with its silkscreen text upright, same
// as the Adafruit CircuitPython and Arduino drivers. This is the reference
// orientation SetRotation and SetMirror compensate from; see Rotation and
// SetMirror.
type DeviceAdafruitRGBMatrixQT13x9 struct {
	Device

	rotation                         Rotation
	mirrorHorizontal, mirrorVertical bool
}

var _ drivers.Displayer = (*DeviceAdafruitRGBMatrixQT13x9)(nil)

// The board wires the chip's switch lines to pixel rows out of order; table
// taken from the Adafruit CircuitPython driver.
var adafruitRGBMatrixQTRows = [9]uint16{8, 5, 4, 3, 2, 1, 0, 7, 6}

// NewAdafruitRGBMatrixQT13x9 creates a new driver with the Adafruit 13x9 PWM
// RGB LED Matrix Driver (IS31FL3741 QT) layout.
// Available addresses (selectable with the two address jumpers):
// - 0x30 (default)
// - 0x31, 0x32, 0x33
func NewAdafruitRGBMatrixQT13x9(bus drivers.I2C, address uint8) DeviceAdafruitRGBMatrixQT13x9 {
	return DeviceAdafruitRGBMatrixQT13x9{
		Device: New(bus, address),
	}
}

// Size returns the dimensions of the LED matrix as currently rotated; see
// Rotation. SetMirror does not affect the reported size.
func (d *DeviceAdafruitRGBMatrixQT13x9) Size() (x, y int16) {
	if d.rotation == Rotation90 || d.rotation == Rotation270 {
		return panelHeight, panelWidth
	}
	return panelWidth, panelHeight
}

// SetPixel sets the color of a single pixel in the internal buffer, call
// Display to send it to the board. The alpha channel is ignored, pixels out
// of range are ignored. Coordinates are in the panel's current, as-mounted
// and mirrored orientation; see Rotation and SetMirror.
func (d *DeviceAdafruitRGBMatrixQT13x9) SetPixel(x, y int16, c color.RGBA) {
	x, y, ok := d.toPanel(x, y)
	if !ok {
		return
	}

	row := adafruitRGBMatrixQTRows[y]
	var offset uint16
	if x < 10 {
		offset = 3 * (uint16(x) + row*10)
	} else {
		offset = 3 * (uint16(x) + 80 + row*3)
	}

	// The board wires each cell's channels B,G,R (Adafruit's IS3741_BGR
	// default), and the physical order alternates between even and odd
	// columns, with the last column differing once more
	if x&1 == 1 || x == 12 {
		d.SetLED(offset+1, c.R)
		d.SetLED(offset, c.G)
		d.SetLED(offset+2, c.B)
	} else {
		d.SetLED(offset+2, c.R)
		d.SetLED(offset+1, c.G)
		d.SetLED(offset, c.B)
	}
}
