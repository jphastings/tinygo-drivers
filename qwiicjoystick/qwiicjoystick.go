// Package qwiicjoystick provides a driver for the SparkFun Qwiic Joystick: a
// 2-axis analog thumb joystick with a momentary push button, exposed over
// I2C by an ATtiny85 running SparkFun firmware.
//
// Product page: https://www.sparkfun.com/sparkfun-qwiic-joystick.html
//
// Register map from the SparkFun firmware:
//
//	https://github.com/sparkfun/Qwiic_Joystick/blob/master/Firmware/ATtiny85%20Firmware/Qwiic_Joystick_v26/Qwiic_Joystick_v26.ino
//
// This driver is inspired by the SparkFun Arduino library:
//
//	https://github.com/sparkfun/SparkFun_Qwiic_Joystick_Arduino_Library
package qwiicjoystick

import (
	"errors"

	"tinygo.org/x/drivers"
)

var (
	errNotConnected   = errors.New("qwiicjoystick: no Qwiic Joystick found on the bus")
	errInvalidAddress = errors.New("qwiicjoystick: address must be between 0x08 and 0x77")
)

// Device wraps an I2C connection to a Qwiic Joystick.
type Device struct {
	bus drivers.I2C

	// Address is the 7-bit I2C address of the joystick: DefaultAddress
	// unless it has been reconfigured.
	Address uint8

	// Scratch buffer: register address plus up to four data bytes (the
	// X/Y position registers, read in one burst).
	buf [5]byte
}

// New creates a new Qwiic Joystick connection. The I2C bus must already be
// configured.
//
// This function only creates the Device object, it does not touch the
// device.
func New(bus drivers.I2C, address uint8) Device {
	return Device{
		bus:     bus,
		Address: address,
	}
}

// Connected checks whether a Qwiic Joystick answers on the bus, by
// verifying its ID register.
func (d *Device) Connected() bool {
	id, err := d.readRegister(REG_ID)
	return err == nil && id == DeviceID
}

// Configure checks the device is responding and clears any latched button
// press, so a program starts from a known state regardless of what
// happened before it ran.
func (d *Device) Configure() error {
	if !d.Connected() {
		return errNotConnected
	}
	return d.ClearEventBits()
}

// FirmwareVersion returns the major and minor version of the firmware
// running on the joystick.
func (d *Device) FirmwareVersion() (major, minor uint8, err error) {
	major, err = d.readRegister(REG_FIRMWARE_MAJOR)
	if err != nil {
		return
	}
	minor, err = d.readRegister(REG_FIRMWARE_MINOR)
	return
}

// RawPosition reads the stick's raw 10-bit ADC position: 0-1023 on each
// axis, with Center (512) at rest. Which physical direction increases X or
// Y, and which axis is which, depends on how the board is mounted; use
// Position for a version already centered on zero.
func (d *Device) RawPosition() (x, y uint16, err error) {
	d.buf[0] = REG_X_MSB
	if err = d.bus.Tx(uint16(d.Address), d.buf[:1], d.buf[1:5]); err != nil {
		return 0, 0, err
	}

	// The firmware ADCs each axis to 10 bits and left-shifts it into a
	// 16-bit word before splitting that word into MSB/LSB registers, so the
	// low 6 bits of the reassembled word are always zero and must be
	// shifted back out - the value is not simply the two bytes concatenated.
	x = (uint16(d.buf[1])<<8 | uint16(d.buf[2])) >> 6
	y = (uint16(d.buf[3])<<8 | uint16(d.buf[4])) >> 6
	return x, y, nil
}

// Position is RawPosition expressed as a signed offset from rest, roughly
// -512 to 511 with 0 at rest, so callers can act on direction and
// magnitude without first subtracting Center themselves.
func (d *Device) Position() (x, y int16, err error) {
	rawX, rawY, err := d.RawPosition()
	if err != nil {
		return 0, 0, err
	}
	return int16(rawX) - int16(Center), int16(rawY) - int16(Center), nil
}

// IsPressed returns whether the button is currently held down. The
// firmware reads the button pin with its internal pull-up enabled and the
// switch grounding the pin when pressed, so the register itself is active
// low (0 while held); IsPressed inverts that so true means "held down".
func (d *Device) IsPressed() (bool, error) {
	v, err := d.readRegister(REG_BUTTON)
	return v == 0, err
}

// HasBeenPressed reports whether the button has been pressed since the
// latch was last cleared with ClearEventBits. Unlike qwiicbutton's
// HasBeenClicked, this does not require a full press-and-release cycle:
// the firmware latches it as soon as the button goes down.
func (d *Device) HasBeenPressed() (bool, error) {
	status, err := d.readRegister(REG_STATUS)
	return status&statusHasBeenPressed != 0, err
}

// ClearEventBits clears the latched HasBeenPressed bit. The firmware does
// not clear it on any read by itself - despite a register-map comment in
// the firmware source suggesting otherwise - so this must be called
// explicitly to detect the next press.
func (d *Device) ClearEventBits() error {
	return d.writeRegister(REG_STATUS, 0)
}

// SetAddress changes the I2C address the joystick answers on, from the
// given value (which the firmware requires to be between 0x08 and 0x77),
// and updates Address to match. The firmware requires REG_I2C_LOCK to be
// set immediately before REG_I2C_ADDRESS is written, or it leaves the
// address unchanged; SetAddress does both. The new address is persisted to
// EEPROM immediately, surviving a power cycle.
func (d *Device) SetAddress(address uint8) error {
	if address < 0x08 || address > 0x77 {
		return errInvalidAddress
	}
	if err := d.writeRegister(REG_I2C_LOCK, i2cUnlockValue); err != nil {
		return err
	}
	if err := d.writeRegister(REG_I2C_ADDRESS, address); err != nil {
		return err
	}
	d.Address = address
	return nil
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
