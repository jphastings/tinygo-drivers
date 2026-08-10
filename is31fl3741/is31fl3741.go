// Package is31fl3741 provides a driver for the Lumissil IS31FL3741 matrix LED
// driver.
//
// Driver supports following layouts:
//   - any custom LED matrix layout (up to 39x9 single-color LEDs)
//   - Adafruit 13x9 PWM RGB LED Matrix Driver (IS31FL3741 QT)
//     https://www.adafruit.com/product/5201
//
// Datasheet:
//
//	https://www.lumissil.com/assets/pdf/core/IS31FL3741_DS.pdf
//
// This driver inspired by Adafruit Python driver:
//
//	https://github.com/adafruit/Adafruit_CircuitPython_IS31FL3741
package is31fl3741

import (
	"errors"
	"time"

	"tinygo.org/x/drivers"
)

var errNotConnected = errors.New("is31fl3741: no device found on the configured address")

// Device implements TinyGo driver for Lumissil IS31FL3741 matrix LED driver
type Device struct {
	Address uint8
	bus     drivers.I2C

	// PWM values for all LED channels, buffered so a whole frame can be sent
	// with one I2C transaction per page. The first byte of each buffer stays
	// zero: it addresses the page's first register when the buffer is sent.
	buf1 [1 + firstPageLEDCount]byte
	buf2 [1 + (LEDCount - firstPageLEDCount)]byte

	// Scratch buffer for two-byte register writes
	cmd [2]byte

	// Currently selected page
	page uint8
}

// New creates a raw driver w/o any preset board layout. Addresses:
// - 0x30 (ADDR pin connected to GND)
// - 0x31 (ADDR pin connected to SCL)
// - 0x32 (ADDR pin connected to SDA)
// - 0x33 (ADDR pin connected to VCC)
func New(bus drivers.I2C, address uint8) Device {
	return Device{
		Address: address,
		bus:     bus,
		page:    pageUnknown,
	}
}

// Connected checks whether an IS31FL3741 answers on the configured address
func (d *Device) Connected() bool {
	d.cmd[0] = ID_REGISTER
	id := d.cmd[1:2]
	err := d.bus.Tx(uint16(d.Address), d.cmd[:1], id)
	return err == nil && id[0] == d.Address<<1
}

// Configure resets the chip and brings it into normal operation: current
// scaling of every LED channel at maximum, all PWM values at zero and
// maximum global current (same defaults as the Adafruit drivers). Overall
// brightness can be reduced with SetGlobalCurrent.
func (d *Device) Configure() (err error) {
	if !d.Connected() {
		return errNotConnected
	}

	// Reset all registers to their default values
	err = d.writeFunctionRegister(FUNC_RESET, RESET_TRIGGER)
	if err != nil {
		return err
	}
	d.page = pageUnknown
	time.Sleep(10 * time.Millisecond)

	// Run every LED channel at full current scale
	d.Fill(0xFF)
	err = d.writeBuffers(PAGE_SCALING_1)
	if err != nil {
		return err
	}

	// Start with all LEDs off
	d.Fill(0x00)
	err = d.writeBuffers(PAGE_PWM_1)
	if err != nil {
		return err
	}

	err = d.SetGlobalCurrent(0xFF)
	if err != nil {
		return err
	}

	// Leave software shutdown, activate all 9 switch lines
	return d.writeFunctionRegister(FUNC_CONFIGURATION, CONFIGURATION_NORMAL)
}

// SetLED sets the PWM value [0-255] of a single LED channel [0-350] in the
// internal buffer, call Display to send it to the chip. How channels map to
// physical LED positions is board-specific; values out of range are ignored.
func (d *Device) SetLED(channel uint16, value uint8) {
	if channel >= LEDCount {
		return
	}

	if channel < firstPageLEDCount {
		d.buf1[1+channel] = value
	} else {
		d.buf2[1+channel-firstPageLEDCount] = value
	}
}

// Fill sets every LED channel in the internal buffer to the same PWM value
// [0-255], call Display to send it to the chip
func (d *Device) Fill(value uint8) {
	for i := 1; i < len(d.buf1); i++ {
		d.buf1[i] = value
	}
	for i := 1; i < len(d.buf2); i++ {
		d.buf2[i] = value
	}
}

// Display sends the buffered PWM values to the chip
func (d *Device) Display() error {
	return d.writeBuffers(PAGE_PWM_1)
}

// SetGlobalCurrent sets the global current control register [0-255] that
// scales the brightness of all LEDs at once
func (d *Device) SetGlobalCurrent(value uint8) error {
	return d.writeFunctionRegister(FUNC_GLOBAL_CURRENT, value)
}

// Sleep puts the chip in (or takes it out of) software shutdown. All
// register contents are kept while asleep.
func (d *Device) Sleep(sleepEnabled bool) error {
	if sleepEnabled {
		return d.writeFunctionRegister(FUNC_CONFIGURATION, CONFIGURATION_SHUTDOWN)
	}

	return d.writeFunctionRegister(FUNC_CONFIGURATION, CONFIGURATION_NORMAL)
}

// selectPage selects the register page for subsequent transfers. The command
// register locks itself after every write, so it has to be unlocked first.
func (d *Device) selectPage(page uint8) (err error) {
	if page == d.page {
		return nil
	}

	d.cmd[0], d.cmd[1] = COMMAND_WRITE_LOCK, COMMAND_UNLOCK
	err = d.bus.Tx(uint16(d.Address), d.cmd[:], nil)
	if err != nil {
		return err
	}

	d.cmd[0], d.cmd[1] = COMMAND, page
	err = d.bus.Tx(uint16(d.Address), d.cmd[:], nil)
	if err != nil {
		return err
	}

	d.page = page
	return nil
}

// writeFunctionRegister writes a single register on the function page
func (d *Device) writeFunctionRegister(register, value uint8) (err error) {
	err = d.selectPage(PAGE_FUNCTION)
	if err != nil {
		return err
	}

	d.cmd[0], d.cmd[1] = register, value
	return d.bus.Tx(uint16(d.Address), d.cmd[:], nil)
}

// writeBuffers sends the channel buffers to a pair of pages: either the PWM
// pages or the current scaling pages
func (d *Device) writeBuffers(firstPage uint8) (err error) {
	err = d.selectPage(firstPage)
	if err != nil {
		return err
	}
	err = d.bus.Tx(uint16(d.Address), d.buf1[:], nil)
	if err != nil {
		return err
	}

	err = d.selectPage(firstPage + 1)
	if err != nil {
		return err
	}
	return d.bus.Tx(uint16(d.Address), d.buf2[:], nil)
}
