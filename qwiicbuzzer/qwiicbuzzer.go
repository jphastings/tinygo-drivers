// Package qwiicbuzzer provides a driver for the SparkFun Qwiic Buzzer: a
// piezo buzzer with four discrete volume levels, exposed over I2C by an
// ATtiny84 running SparkFun firmware.
//
// Product page: https://www.sparkfun.com/sparkfun-qwiic-buzzer.html
//
// Register map from the SparkFun firmware:
//
//	https://github.com/sparkfun/SparkFun_Qwiic_Buzzer/blob/main/Firmware/QwiicBuzzerFirmware/sfeQwiicBuzzerFirmware.h
//
// This driver is inspired by the SparkFun Arduino library:
//
//	https://github.com/sparkfun/SparkFun_Qwiic_Buzzer_Arduino_Library
package qwiicbuzzer

import (
	"errors"
	"time"

	"tinygo.org/x/drivers"
)

var (
	errNotConnected   = errors.New("qwiicbuzzer: no Qwiic Buzzer found on the bus")
	errInvalidVolume  = errors.New("qwiicbuzzer: invalid volume")
	errInvalidAddress = errors.New("qwiicbuzzer: address must be between 0x08 and 0x77")
)

// Device wraps an I2C connection to a Qwiic Buzzer.
type Device struct {
	bus drivers.I2C

	// Address is the 7-bit I2C address of the buzzer: DefaultAddress unless
	// the buzzer has been reconfigured.
	Address uint8

	// Scratch buffer: register address plus up to five data bytes (the
	// tone-frequency, volume and duration registers, written in one burst).
	buf [6]byte
}

// New creates a new Qwiic Buzzer connection. The I2C bus must already be
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

// Connected checks whether a Qwiic Buzzer answers on the bus, by verifying
// its device ID register.
func (d *Device) Connected() bool {
	id, err := d.readRegister(REG_ID)
	return err == nil && id == DeviceID
}

// Configure checks the device is responding and silences it, so a program
// starts from a known, quiet state regardless of what was left running
// beforehand.
func (d *Device) Configure() error {
	if !d.Connected() {
		return errNotConnected
	}
	return d.Stop()
}

// FirmwareVersion returns the major and minor version of the firmware
// running on the buzzer.
func (d *Device) FirmwareVersion() (major, minor uint8, err error) {
	major, err = d.readRegister(REG_FIRMWARE_MAJOR)
	if err != nil {
		return
	}
	minor, err = d.readRegister(REG_FIRMWARE_MINOR)
	return
}

// SetTone loads the tone frequency, volume and duration registers without
// starting the buzzer. It is useful together with SaveSettings, or to stage
// a tone that Start (or the board's physical TRIGGER pin) will later play.
//
// frequencyHz is the tone's frequency in Hz; 0 is a deliberate "rest" the
// firmware recognises and plays as silence rather than a 0Hz tone.
//
// duration is rounded down to whole milliseconds, the resolution of the
// firmware's timer, and capped at the 16-bit register's ~65.5s maximum. A
// duration of 0 - including the zero value of time.Duration - has a special
// hardware meaning: play indefinitely until Stop is called. A request for a
// short but nonzero duration that would itself round down to 0 is rounded up
// to 1ms instead, so it can't be silently reinterpreted as "forever".
func (d *Device) SetTone(frequencyHz uint16, volume Volume, duration time.Duration) error {
	if !volume.valid() {
		return errInvalidVolume
	}

	ms := millisFromDuration(duration)

	d.buf[0] = REG_TONE_FREQUENCY_MSB
	d.buf[1] = uint8(frequencyHz >> 8)
	d.buf[2] = uint8(frequencyHz)
	d.buf[3] = uint8(volume)
	d.buf[4] = uint8(ms >> 8)
	d.buf[5] = uint8(ms)
	return d.bus.Tx(uint16(d.Address), d.buf[:6], nil)
}

// Start plays whatever tone is currently loaded (via SetTone, a previous
// Play, or settings restored from EEPROM at power-on).
func (d *Device) Start() error {
	return d.writeRegister(REG_ACTIVE, 1)
}

// Stop silences the buzzer immediately, regardless of any duration still
// outstanding.
func (d *Device) Stop() error {
	return d.writeRegister(REG_ACTIVE, 0)
}

// Play loads the given tone and starts it: the one-call form of SetTone
// followed by Start that most programs want.
func (d *Device) Play(frequencyHz uint16, volume Volume, duration time.Duration) error {
	if err := d.SetTone(frequencyHz, volume, duration); err != nil {
		return err
	}
	return d.Start()
}

// PlayPeriod is Play expressed in terms of a wave period in nanoseconds
// rather than a frequency in Hz, so a tone.Note from tinygo-drivers' tone
// package can be played directly:
//
//	buzzer.PlayPeriod(tone.C4.Period(), qwiicbuzzer.VolumeMax, 500*time.Millisecond)
//
// This driver deliberately does not import the tone package: tone.go pulls
// in "machine", which only exists under the TinyGo toolchain, and that
// would stop this package (and its tests) building with plain `go build`.
// See FrequencyFromPeriod for the conversion this performs.
func (d *Device) PlayPeriod(periodNanoseconds uint64, volume Volume, duration time.Duration) error {
	return d.Play(FrequencyFromPeriod(periodNanoseconds), volume, duration)
}

// FrequencyFromPeriod converts a wave period in nanoseconds - as returned by
// tone.Note.Period() - to the Hz value the tone frequency register expects.
// A period of 0, tone's own "no sound" sentinel (Note(0).Period() == 0),
// maps to 0, which the firmware plays as a silent rest rather than
// attempting a 0Hz tone.
func FrequencyFromPeriod(periodNanoseconds uint64) uint16 {
	if periodNanoseconds == 0 {
		return 0
	}
	hz := uint64(1_000_000_000) / periodNanoseconds
	if hz > 0xFFFF {
		hz = 0xFFFF
	}
	return uint16(hz)
}

// Frequency reads back the currently configured tone frequency in Hz.
func (d *Device) Frequency() (uint16, error) {
	return d.readRegister16(REG_TONE_FREQUENCY_MSB)
}

// VolumeSetting reads back the currently configured volume.
func (d *Device) VolumeSetting() (Volume, error) {
	v, err := d.readRegister(REG_VOLUME)
	return Volume(v), err
}

// DurationSetting reads back the currently configured duration. A duration
// of 0 means the buzzer plays indefinitely once started.
func (d *Device) DurationSetting() (time.Duration, error) {
	ms, err := d.readRegister16(REG_DURATION_MSB)
	return time.Duration(ms) * time.Millisecond, err
}

// Active reports whether the buzzer is currently sounding.
func (d *Device) Active() (bool, error) {
	v, err := d.readRegister(REG_ACTIVE)
	return v != 0, err
}

// SaveSettings stores the current tone frequency, volume and duration to
// EEPROM, so they survive a power cycle and become what the board plays when
// triggered by its physical TRIGGER pin or (via Start) immediately after
// boot. It does not affect the I2C address, which SetAddress already
// persists immediately.
func (d *Device) SaveSettings() error {
	return d.writeRegister(REG_SAVE_SETTINGS, 1)
}

// SetAddress changes the I2C address the buzzer answers on, from the given
// value (which the firmware requires to be between 0x08 and 0x77), and
// updates Address to match. The firmware persists the new address to EEPROM
// immediately, without needing SaveSettings.
func (d *Device) SetAddress(address uint8) error {
	if address < 0x08 || address > 0x77 {
		return errInvalidAddress
	}
	if err := d.writeRegister(REG_I2C_ADDRESS, address); err != nil {
		return err
	}
	d.Address = address
	return nil
}

// millisFromDuration converts d to the buzzer's 16-bit millisecond duration
// register, preserving 0's special "play forever" meaning (which is also
// time.Duration's own zero value) while stopping a short but nonzero request
// from rounding down into it.
func millisFromDuration(d time.Duration) uint16 {
	if d <= 0 {
		return 0
	}
	ms := d / time.Millisecond
	if ms == 0 {
		ms = 1
	}
	if ms > 0xFFFF {
		ms = 0xFFFF
	}
	return uint16(ms)
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

// The tone-frequency and duration registers are named MSB/LSB pairs that the
// firmware reassembles big-endian - unlike qwiicbutton's 16-bit registers,
// which are the raw (little-endian) memory layout of a struct field. Getting
// this backwards produces a frequency or duration wrong by roughly a factor
// of 256, not a build or transaction error, so it's easy to miss. Confirmed
// against a live board: registers 0x03/0x04 power up as 0x0A/0xAA, which is
// 2730 (SparkFun's documented default frequency) read big-endian and an
// implausible 43530 read little-endian.
func (d *Device) readRegister16(reg uint8) (uint16, error) {
	d.buf[0] = reg
	err := d.bus.Tx(uint16(d.Address), d.buf[:1], d.buf[1:3])
	return uint16(d.buf[1])<<8 | uint16(d.buf[2]), err
}
