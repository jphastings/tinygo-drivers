// Package qwiicbutton provides a driver for the SparkFun Qwiic Button: a
// momentary push button with a built-in LED, exposed over I2C by an ATTiny84
// running SparkFun firmware.
//
// Product page: https://www.sparkfun.com/sparkfun-qwiic-button-green-led.html
//
// Register map from the SparkFun firmware:
//
//	https://github.com/sparkfun/Qwiic_Button/blob/master/Firmware/Qwiic_Button/registers.h
//
// This driver is inspired by the SparkFun Arduino library:
//
//	https://github.com/sparkfun/SparkFun_Qwiic_Button_Arduino_Library
package qwiicbutton

import (
	"errors"

	"tinygo.org/x/drivers"
)

var errNotConnected = errors.New("qwiicbutton: no Qwiic Button found on the bus")

// Device wraps an I2C connection to a Qwiic Button.
type Device struct {
	bus drivers.I2C

	// Address is the 7-bit I2C address of the button: DefaultAddress unless
	// the button has been reconfigured.
	Address uint8

	// Scratch buffer: register address plus up to two data bytes
	buf [3]byte
}

// New creates a new Qwiic Button connection. The I2C bus must already be
// configured.
//
// This function only creates the Device object, it does not touch the device.
func New(bus drivers.I2C, address uint8) Device {
	return Device{
		bus:     bus,
		Address: address,
	}
}

// Connected checks whether a Qwiic Button answers on the bus, by verifying
// its device ID register.
func (d *Device) Connected() bool {
	id, err := d.readRegister(REG_ID)
	return err == nil && id == DeviceID
}

// Configure checks the device is responding and puts it in a known state:
// latched button events cleared and the LED off.
func (d *Device) Configure() error {
	if !d.Connected() {
		return errNotConnected
	}
	if err := d.ClearEventBits(); err != nil {
		return err
	}
	return d.LEDOff()
}

// FirmwareVersion returns the major and minor version of the firmware running
// on the button.
func (d *Device) FirmwareVersion() (major, minor uint8, err error) {
	major, err = d.readRegister(REG_FIRMWARE_MAJOR)
	if err != nil {
		return
	}
	minor, err = d.readRegister(REG_FIRMWARE_MINOR)
	return
}

// IsPressed returns whether the button is currently held down.
func (d *Device) IsPressed() (bool, error) {
	status, err := d.readRegister(REG_BUTTON_STATUS)
	return status&statusIsPressed != 0, err
}

// HasBeenClicked returns whether the button has been clicked (pressed and
// released) since the latch was last cleared with ClearEventBits.
func (d *Device) HasBeenClicked() (bool, error) {
	status, err := d.readRegister(REG_BUTTON_STATUS)
	return status&statusHasBeenClicked != 0, err
}

// EventAvailable returns whether a button event (press or release) has
// occurred since the latch was last cleared with ClearEventBits.
func (d *Device) EventAvailable() (bool, error) {
	status, err := d.readRegister(REG_BUTTON_STATUS)
	return status&statusEventAvailable != 0, err
}

// ClearEventBits clears the latched event and clicked flags, so that
// HasBeenClicked and EventAvailable report only new activity.
func (d *Device) ClearEventBits() error {
	status, err := d.readRegister(REG_BUTTON_STATUS)
	if err != nil {
		return err
	}
	status &^= statusEventAvailable | statusHasBeenClicked | statusIsPressed
	return d.writeRegister(REG_BUTTON_STATUS, status)
}

// DebounceTime returns the time the firmware waits for the mechanical
// contacts to settle, in milliseconds.
func (d *Device) DebounceTime() (uint16, error) {
	return d.readRegister16(REG_BUTTON_DEBOUNCE_TIME)
}

// SetDebounceTime sets the time the firmware waits for the mechanical
// contacts to settle, in milliseconds. The firmware default is 10ms.
func (d *Device) SetDebounceTime(ms uint16) error {
	return d.writeRegister16(REG_BUTTON_DEBOUNCE_TIME, ms)
}

// LEDConfig configures the built-in LED. Brightness ranges from 0 (off) to
// 255 (maximum). A non-zero cycleTime pulses the LED: one breathing cycle
// takes cycleTime milliseconds, followed by offTime milliseconds of darkness.
// Granularity is the brightness step per pulse update; 1 is fine for most
// applications.
func (d *Device) LEDConfig(brightness uint8, cycleTime, offTime uint16, granularity uint8) error {
	if granularity == 0 {
		// The firmware divides the brightness range by granularity to compute
		// its pulse steps, so 0 would make it divide by zero.
		granularity = 1
	}
	if err := d.writeRegister(REG_LED_BRIGHTNESS, brightness); err != nil {
		return err
	}
	if err := d.writeRegister(REG_LED_PULSE_GRANULARITY, granularity); err != nil {
		return err
	}
	if err := d.writeRegister16(REG_LED_PULSE_CYCLE_TIME, cycleTime); err != nil {
		return err
	}
	return d.writeRegister16(REG_LED_PULSE_OFF_TIME, offTime)
}

// LEDOn turns the built-in LED on at the given brightness, from 0 (off) to
// 255 (maximum).
func (d *Device) LEDOn(brightness uint8) error {
	return d.LEDConfig(brightness, 0, 0, 1)
}

// LEDOff turns the built-in LED off.
func (d *Device) LEDOff() error {
	return d.LEDConfig(0, 0, 0, 1)
}

func (d *Device) readRegister(reg uint8) (uint8, error) {
	d.buf[0] = reg
	err := d.bus.Tx(uint16(d.Address), d.buf[:1], d.buf[1:2])
	return d.buf[1], err
}

func (d *Device) writeRegister(reg, value uint8) error {
	d.buf[0] = reg
	d.buf[1] = value
	return d.bus.Tx(uint16(d.Address), d.buf[:2], nil)
}

// Multi-byte registers are the raw memory of the (little-endian) AVR
// firmware, so the low byte comes first on the wire.
func (d *Device) readRegister16(reg uint8) (uint16, error) {
	d.buf[0] = reg
	err := d.bus.Tx(uint16(d.Address), d.buf[:1], d.buf[1:3])
	return uint16(d.buf[1]) | uint16(d.buf[2])<<8, err
}

func (d *Device) writeRegister16(reg uint8, value uint16) error {
	d.buf[0] = reg
	d.buf[1] = uint8(value)
	d.buf[2] = uint8(value >> 8)
	return d.bus.Tx(uint16(d.Address), d.buf[:3], nil)
}
