// Package tsl2591 provides a driver for the AMS TSL2591, a high-dynamic-range
// ambient light sensor with two photodiodes on one die: a full-spectrum
// (visible + infrared) channel and an infrared-only channel. Comparing the
// two lets it derive illuminance in lux while substantially rejecting the
// IR content that throws off simpler single-diode light sensors, over a
// 600M:1 dynamic range via programmable gain and integration time.
//
// The device's I2C register access requires a COMMAND byte to precede
// every read or write: bit 7 set, and bits 6:5 selecting "normal
// operation" (0b01, for addressing a register) or "special function"
// (0b11, used here only to clear a pending interrupt). Conflating the two
// - e.g. always OR'ing in 0xA0, or always 0xE0 - talks to the wrong
// register or issues the wrong command without any I2C-level indication
// that anything went wrong; see registers.go.
//
// Datasheet:
//
//	https://cdn-shop.adafruit.com/datasheets/TSL25911_Datasheet_EN_v1.pdf
//
// This driver is inspired by Adafruit's Arduino and CircuitPython
// libraries:
//
//	https://github.com/adafruit/Adafruit_TSL2591_Library
//	https://github.com/adafruit/Adafruit_CircuitPython_TSL2591
package tsl2591

import (
	"encoding/binary"
	"errors"
	"time"

	"tinygo.org/x/drivers"
)

var (
	errNotConnected      = errors.New("tsl2591: no TSL2591 found on the bus")
	errConversionTimeout = errors.New("tsl2591: timeout waiting for a valid ALS conversion")
	errSaturated         = errors.New("tsl2591: ADC channel saturated, illuminance reading is not valid")
)

// Device wraps an I2C connection to a TSL2591 device.
type Device struct {
	bus     drivers.I2C
	Address uint16

	gain            Gain
	integrationTime IntegrationTime

	// Last successful reading, for the drivers.Sensor accessors.
	lastCh0, lastCh1 uint16
	lastMilliLux     int32
	lastSaturated    bool

	// Scratch buffers: COMMAND byte (+ value) for writes, up to all four
	// data registers for reads.
	wbuf [2]byte
	rbuf [4]byte
}

// Config holds the configuration applied by Configure.
type Config struct {
	// Gain selects the analog gain of both photodiode channels. The zero
	// value is GainLow (1x).
	Gain Gain
	// IntegrationTime selects how long each ADC conversion integrates for.
	// The zero value is IntegrationTime100ms.
	IntegrationTime IntegrationTime
}

// New creates a new TSL2591 connection. The I2C bus must already be
// configured.
//
// This function only creates the Device object, it does not touch the
// device.
func New(bus drivers.I2C) Device {
	return Device{
		bus:     bus,
		Address: I2CAddress,
	}
}

// Connected returns whether a TSL2591 answers with the expected device ID
// on the bus.
func (d *Device) Connected() bool {
	id, err := d.readRegister(regID)
	return err == nil && id == deviceID
}

// Configure applies the given gain and integration time and powers on the
// ALS. It returns an error without touching the device if no TSL2591
// answers on the bus.
func (d *Device) Configure(cfg Config) error {
	if !d.Connected() {
		return errNotConnected
	}
	return d.applyConfig(cfg.Gain, cfg.IntegrationTime)
}

// SetGain changes the analog gain applied to both channels.
func (d *Device) SetGain(gain Gain) error {
	return d.applyConfig(gain, d.integrationTime)
}

// SetIntegrationTime changes the ADC integration time.
func (d *Device) SetIntegrationTime(t IntegrationTime) error {
	return d.applyConfig(d.gain, t)
}

// Gain returns the gain most recently applied by Configure or SetGain.
func (d *Device) Gain() Gain { return d.gain }

// IntegrationTime returns the integration time most recently applied by
// Configure or SetIntegrationTime.
func (d *Device) IntegrationTime() IntegrationTime { return d.integrationTime }

func (d *Device) applyConfig(gain Gain, t IntegrationTime) error {
	if !gain.valid() {
		return errInvalidGain
	}
	if !t.valid() {
		return errInvalidIntegrationTime
	}

	if err := d.writeRegister(regConfig, uint8(gain)|uint8(t)); err != nil {
		return err
	}

	// STATUS's AVALID bit reflects conversions completed since AEN was
	// last asserted, not since CONFIG last changed (per the datasheet's
	// own wording for AVALID). Without re-asserting AEN here, a read
	// immediately after this call could observe AVALID already set from
	// before the change, and return a count taken under the old
	// gain/integration time.
	if err := d.writeRegister(regEnable, enablePON); err != nil {
		return err
	}
	if err := d.writeRegister(regEnable, enablePON|enableAEN); err != nil {
		return err
	}

	d.gain, d.integrationTime = gain, t
	return nil
}

// ReadChannels performs a fresh reading and returns the raw ADC counts:
// ch0 is the full-spectrum (visible+IR) channel, ch1 is infrared-only. It
// waits for the current integration cycle to complete first.
func (d *Device) ReadChannels() (ch0, ch1 uint16, err error) {
	if err := d.awaitConversion(); err != nil {
		return 0, 0, err
	}

	if err := d.readRegisters(regC0DATAL, d.rbuf[:4]); err != nil {
		return 0, 0, err
	}
	ch0 = binary.LittleEndian.Uint16(d.rbuf[0:2])
	ch1 = binary.LittleEndian.Uint16(d.rbuf[2:4])

	d.lastCh0, d.lastCh1 = ch0, ch1
	return ch0, ch1, nil
}

// Illuminance reads both channels and converts them to illuminance in
// milliLux (1/1000 lux); see lux.go for the formula used and its accuracy
// limitations.
//
// If either channel is at or above the full-scale count for the current
// integration time, the reading is saturated and cannot be trusted: this
// returns errSaturated rather than a plausible-looking but wrong lux
// value. Saturated and MilliLux report the same outcome for later
// inspection. Reduce the gain or integration time and read again.
func (d *Device) Illuminance() (int32, error) {
	ch0, ch1, err := d.ReadChannels()
	if err != nil {
		return 0, err
	}

	fullScale := d.integrationTime.fullScaleCount()
	d.lastSaturated = ch0 >= fullScale || ch1 >= fullScale
	if d.lastSaturated {
		d.lastMilliLux = 0
		return 0, errSaturated
	}

	d.lastMilliLux = calculateMilliLux(ch0, ch1, d.integrationTime.milliseconds(), d.gain.nominalMultiplier())
	return d.lastMilliLux, nil
}

// awaitConversion polls STATUS until AVALID is set (a conversion is ready)
// or the current integration time's worst case has elapsed.
func (d *Device) awaitConversion() error {
	const pollInterval = 5 * time.Millisecond
	deadline := d.integrationTime.settleDuration()

	for elapsed := time.Duration(0); ; elapsed += pollInterval {
		status, err := d.readRegister(regStatus)
		if err != nil {
			return err
		}
		if status&statusAVALID != 0 {
			return nil
		}
		if elapsed >= deadline {
			return errConversionTimeout
		}
		time.Sleep(pollInterval)
	}
}

// Status reads the raw STATUS register (0x13). Bit 0 is AVALID (a
// conversion has completed since ALS was enabled), bit 4 is AINT (an ALS
// threshold interrupt is asserted) and bit 5 is NPINTR (a no-persist
// interrupt is asserted).
func (d *Device) Status() (uint8, error) {
	return d.readRegister(regStatus)
}

// ClearInterrupt clears any pending ALS and no-persist ALS interrupt, via
// the COMMAND register's special-function transaction type rather than a
// normal register write.
func (d *Device) ClearInterrupt() error {
	return d.writeCommand(commandSpecial | sfClearALSAndNoPersistInterrupt)
}

// Update performs an ALS reading and stores it for the accessors below.
// Only drivers.Luminosity is supported; Update does nothing and returns
// nil if it is not requested.
func (d *Device) Update(which drivers.Measurement) error {
	if which&drivers.Luminosity == 0 {
		return nil
	}

	_, err := d.Illuminance()
	return err
}

// MilliLux returns the illuminance from the most recent successful call to
// Illuminance or Update, in milliLux (1/1000 lux). It is meaningless if
// Saturated reports true, or before any reading has succeeded.
func (d *Device) MilliLux() int32 { return d.lastMilliLux }

// Channels returns the raw ADC counts from the most recent call to
// ReadChannels, Illuminance or Update: ch0 is the full-spectrum
// (visible+IR) channel, ch1 is infrared-only.
func (d *Device) Channels() (ch0, ch1 uint16) { return d.lastCh0, d.lastCh1 }

// Saturated reports whether the most recent call to Illuminance or Update
// found either channel saturated, meaning MilliLux does not reflect a real
// reading. Reduce the gain or integration time and read again.
func (d *Device) Saturated() bool { return d.lastSaturated }
