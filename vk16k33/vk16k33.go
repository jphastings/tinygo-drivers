// Package vk16k33 provides a driver for the SparkFun Qwiic Alphanumeric
// Display: four 14-segment LED characters plus decimal point and colon,
// driven over I2C by the VK16K33, an HT16K33-compatible LED controller.
//
// Product page: https://www.sparkfun.com/sparkfun-qwiic-alphanumeric-display-kit.html
//
// Datasheet:
//
//	https://www.szvinka.com/uploadfile/Datasheet/LED/VK16K33/VK16K33_V1.2-EN.pdf
//
// This driver is inspired by the SparkFun Arduino library, from which the
// character font and the board-specific segment wiring are taken:
//
//	https://github.com/sparkfun/SparkFun_Alphanumeric_Display_Arduino_Library
package vk16k33

import (
	"errors"
	"time"

	"tinygo.org/x/drivers"
)

// Digits is the number of characters on the display
const Digits = 4

// The controller has 16 bytes of display RAM
const ramSize = 16

var (
	errBrightnessOutOfRange = errors.New("vk16k33: brightness out of range")
	errInvalidBlinkRate     = errors.New("vk16k33: invalid blink rate")
)

// Device wraps an I2C connection to a VK16K33 alphanumeric display.
//
// Drawing methods only update a buffered copy of the display RAM; call
// Display to push the buffer to the LEDs.
type Device struct {
	bus     drivers.I2C
	Address uint16

	// The display RAM start address (always 0) followed by a copy of the
	// 16 display RAM bytes, so Display flushes in a single transaction
	buf [1 + ramSize]byte

	// Scratch buffer for command writes
	cmd [1]byte
}

// New creates a new VK16K33 connection at the default address. The I2C bus
// must already be configured.
//
// This function only creates the Device object, it does not touch the
// device.
func New(bus drivers.I2C) Device {
	return Device{
		bus:     bus,
		Address: DefaultAddress,
	}
}

// Configure turns the oscillator on, sets full brightness, no blinking,
// display on, and clears the display.
//
// The VK16K33 has no identity register, so there is no Connected check: a
// missing display shows up as an I2C error here.
func (d *Device) Configure() error {
	err := d.writeCmd(CMD_SYSTEM_SETUP | 0x01)
	if err != nil {
		return err
	}
	// Allow the oscillator to start
	time.Sleep(time.Millisecond)

	err = d.SetBrightness(15)
	if err != nil {
		return err
	}
	err = d.SetBlink(BLINK_OFF)
	if err != nil {
		return err
	}

	d.ClearDisplay()
	return d.Display()
}

// WriteString displays up to four characters of s, left aligned, and pushes
// the result to the LEDs. The buffer is cleared first.
//
// A '.' lights the board's decimal point LED and a ':' its colon LED,
// without taking up a character position. Characters outside printable
// ASCII show as the all-segments-on error pattern.
func (d *Device) WriteString(s string) error {
	d.ClearDisplay()

	digit := uint8(0)
	for i := 0; i < len(s) && digit < Digits; i++ {
		switch c := s[i]; c {
		case '.':
			d.SetDecimalPoint(true)
		case ':':
			d.SetColon(true)
		default:
			d.drawChar(digit, fontSegments(c))
			digit++
		}
	}

	return d.Display()
}

// ClearDisplay turns off every segment in the buffer. Call Display to blank
// the LEDs.
func (d *Device) ClearDisplay() {
	for i := range d.buf {
		d.buf[i] = 0
	}
}

// SetDecimalPoint turns the decimal point LED on or off in the buffer
func (d *Device) SetDecimalPoint(on bool) {
	d.setExtraLED(3, on)
}

// SetColon turns the colon LED on or off in the buffer
func (d *Device) SetColon(on bool) {
	d.setExtraLED(1, on)
}

// Display sends the buffered display RAM to the device in one transaction
func (d *Device) Display() error {
	return d.bus.Tx(d.Address, d.buf[:], nil)
}

// SetBrightness sets the display brightness, from 0 (1/16 duty cycle) to 15
// (always on)
func (d *Device) SetBrightness(brightness uint8) error {
	if brightness > 15 {
		return errBrightnessOutOfRange
	}
	return d.writeCmd(CMD_DIMMING_SETUP | brightness)
}

// SetBlink makes the whole display blink at one of the BlinkRate constants,
// or steady with BLINK_OFF. The display is kept switched on.
//
// The display on/off bit is written here, and its polarity is taken from the
// Holtek HT16K33 datasheet - 1 for on - not from the VK16K33 datasheet, whose
// display setup table prints it the other way round. That table is one of
// several errors in the same document, which also duplicates dimming rows and
// carries a default value copied from an adjacent table. A real display lights
// and blinks correctly with the polarity used here, so leave it alone.
func (d *Device) SetBlink(rate BlinkRate) error {
	if rate > BLINK_0_5HZ {
		return errInvalidBlinkRate
	}
	return d.writeCmd(CMD_DISPLAY_SETUP | uint8(rate)<<1 | 0x01)
}

// segmentCom maps segments H through N to the COM line each shares with
// segments A through G. This wiring is specific to the SparkFun board and
// is transcribed from illuminateSegment in the SparkFun Arduino library.
var segmentCom = [7]uint8{1, 0, 2, 3, 4, 5, 6}

// drawChar lights a font segment pattern for one digit (0 to 3) in the
// buffer. Each COM line uses one display RAM byte pair: segments A-G set
// bit "digit" of the even byte, segments H-N set bit "digit"+4.
func (d *Device) drawChar(digit uint8, segments uint16) {
	for seg := uint8(0); seg < 14; seg++ {
		if segments&(1<<seg) == 0 {
			continue
		}
		com, bit := seg, digit
		if seg >= 7 {
			com, bit = segmentCom[seg-7], digit+4
		}
		d.buf[1+2*com] |= 1 << bit
	}
}

// setExtraLED drives the decimal point and colon LEDs, wired on the
// SparkFun board to bit 0 of the odd display RAM bytes 3 and 1
func (d *Device) setExtraLED(ramAddr int, on bool) {
	if on {
		d.buf[1+ramAddr] |= 0x01
	} else {
		d.buf[1+ramAddr] &^= 0x01
	}
}

func (d *Device) writeCmd(cmd uint8) error {
	d.cmd[0] = cmd
	return d.bus.Tx(d.Address, d.cmd[:], nil)
}
