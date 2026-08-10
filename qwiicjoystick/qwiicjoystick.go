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
//
// # Orientation
//
// Measured on a real board with the "SparkFun" silkscreen text upright, a
// physical push up increases the firmware's raw X reading and a push right
// increases its raw Y reading - the firmware's own axes are transposed from
// what a caller expects. RawPosition reports those raw values exactly as
// read. Position corrects the transposition, so that with the board
// mounted silkscreen-up (the reference orientation), pushing right reads
// positive X and pushing up reads positive Y.
//
// If the board is mounted some other way round, set Config.Rotation (or
// call SetRotation later) to the clockwise angle, in 90 degree steps, that
// the board was turned away from silkscreen-up before installation.
// Position then applies that rotation on top of the axis correction above,
// so a push in a given physical direction reads the same way regardless of
// which of the four ways the board was mounted, once Rotation matches it.
// Center is 512; the example below uses a partial deflection of 200 to
// keep the numbers symmetric (the raw range's actual extremes, 0 and 1023,
// are offset -512 and +511 from Center, not symmetric):
//
//	Rotation     Push      RawPosition()   Position()
//	Rotation0    Up        (712, 512)      (0, +200)
//	Rotation0    Right     (512, 712)      (+200, 0)
//	Rotation90   Up        (512, 312)      (0, +200)
//	Rotation90   Right     (712, 512)      (+200, 0)
//	Rotation180  Up        (312, 512)      (0, +200)
//	Rotation180  Right     (512, 312)      (+200, 0)
//	Rotation270  Up        (512, 712)      (0, +200)
//	Rotation270  Right     (312, 512)      (+200, 0)
//
// Config.MirrorHorizontal and Config.MirrorVertical (or SetMirror) flip
// Position's X and Y axes respectively, applied after Rotation, in the
// direction the caller actually sees - so MirrorHorizontal always means
// "flip the left/right axis I see", regardless of Rotation. For example,
// RawPosition (512, 712) above ("push right" at Rotation0) reads Position
// (+200, 0) unmirrored but (-200, 0) with MirrorHorizontal set. Mirroring
// both axes together is equivalent to an additional Rotation180 - e.g. that
// same reading with Rotation0 plus both mirrors set, or with Rotation180
// alone and no mirroring, both read (-200, 0) - so of the sixteen nominal
// Rotation/mirror combinations, only eight are actually distinct.
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

	rotation                         Rotation
	mirrorHorizontal, mirrorVertical bool

	// Scratch buffer: register address plus up to four data bytes (the
	// X/Y position registers, read in one burst).
	buf [5]byte
}

// Config holds settings applied by Configure. All three fields can also be
// changed later, without a full reconfigure, via SetRotation and SetMirror.
type Config struct {
	// Rotation compensates Position for how the board is physically
	// mounted, applied after axis normalization and before any mirroring.
	// The zero value, Rotation0, applies no extra rotation. See Rotation
	// and the package doc for the convention and a worked example.
	Rotation Rotation

	// MirrorHorizontal and MirrorVertical flip Position's X and Y axes,
	// applied after Rotation in the direction the caller actually sees -
	// so MirrorHorizontal always means "flip the left/right axis I see",
	// regardless of Rotation. Both false, the zero value, applies no
	// mirroring. Setting both is equivalent to an additional Rotation180;
	// see the package doc.
	MirrorHorizontal bool
	MirrorVertical   bool
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

// Configure checks the device is responding, applies cfg, and clears any
// latched button press, so a program starts from a known state regardless
// of what happened before it ran.
func (d *Device) Configure(cfg Config) error {
	if !d.Connected() {
		return errNotConnected
	}
	if err := d.SetRotation(cfg.Rotation); err != nil {
		return err
	}
	d.SetMirror(cfg.MirrorHorizontal, cfg.MirrorVertical)
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
// axis, with Center (512) at rest. It is completely untransformed: unlike
// Position, it is never centered, axis-corrected, rotated or mirrored, so
// calibrating or debugging code can see exactly what the firmware sent.
//
// In particular its X and Y are not the same axes Position reports: the
// firmware's raw X increases when the stick is pushed physically up and its
// raw Y increases when pushed physically right, the opposite of what
// RawPosition's own axis names suggest. See the package doc for the
// measurement behind this and how Position corrects it.
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

// Position is RawPosition centered on zero (roughly -512 to 511, 0 at
// rest), axis-corrected, rotated and mirrored, in that order, so callers
// can treat X as left/right and Y as up/down consistently regardless of how
// the board is wired, mounted, or configured. See the package doc for the
// axis correction this always applies and a worked example of Rotation and
// mirroring on top of it.
func (d *Device) Position() (x, y int16, err error) {
	rawX, rawY, err := d.RawPosition()
	if err != nil {
		return 0, 0, err
	}

	// The firmware's raw X tracks up/down and raw Y tracks left/right - see
	// RawPosition - so correcting to the usual X=left/right, Y=up/down
	// convention swaps them, not just centers them.
	x, y = int16(rawY)-int16(Center), int16(rawX)-int16(Center)

	x, y = d.rotation.apply(x, y)

	if d.mirrorHorizontal {
		x = -x
	}
	if d.mirrorVertical {
		y = -y
	}
	return x, y, nil
}

// SetRotation changes the rotation Position applies on top of its axis
// correction, without touching the hardware - useful for correcting the
// mounting at runtime, e.g. from a settings menu, without a full Configure.
// It returns errInvalidRotation, leaving the current rotation unchanged,
// if r is not one of the four declared Rotation constants.
func (d *Device) SetRotation(r Rotation) error {
	if !r.valid() {
		return errInvalidRotation
	}
	d.rotation = r
	return nil
}

// Rotation returns the rotation most recently applied by Configure or
// SetRotation.
func (d *Device) Rotation() Rotation {
	return d.rotation
}

// SetMirror changes the axis mirroring Position applies after Rotation,
// without touching the hardware. See Config.MirrorHorizontal and
// Config.MirrorVertical for what horizontal and vertical mean here. There
// is nothing to validate - any combination of the two bools is valid - so,
// unlike SetRotation, this cannot fail.
func (d *Device) SetMirror(horizontal, vertical bool) {
	d.mirrorHorizontal = horizontal
	d.mirrorVertical = vertical
}

// Mirror returns the axis mirroring most recently applied by Configure or
// SetMirror.
func (d *Device) Mirror() (horizontal, vertical bool) {
	return d.mirrorHorizontal, d.mirrorVertical
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
