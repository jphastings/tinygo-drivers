// Package ltr329 provides a driver for the Lite-On LTR-329ALS-01, an I2C
// ambient light sensor with two photodiodes on one die: a full-spectrum
// (visible + infrared) channel (CH0) and an infrared-only channel (CH1).
// Comparing the two lets it derive illuminance in lux while substantially
// rejecting the IR content that throws off simpler single-diode light
// sensors, over a 0.01 to 64,000 lux range via six gain settings.
//
// The four ALS_DATA registers (0x88-0x8B) latch together for the duration
// of a single I2C read transaction, and the datasheet requires they be
// read starting from the lowest address: CH1 low, CH1 high, CH0 low, CH0
// high. A single 4-byte auto-incrementing burst starting at CH1's low byte
// achieves this; reading CH0 first, or reading the two channels as
// separate transactions, risks a torn pair from two different conversions.
// See readRegisters and ReadChannels.
//
// Datasheet:
//
//	https://cdn-shop.adafruit.com/product-files/5591/LTR-329ALS-01-Lite-On-datasheet-140998467.pdf
//
// The lux formula (see lux.go) comes from Lite-On's separate "Appendix A"
// document, not the datasheet itself:
//
//	https://cdck-file-uploads-europe1.s3.dualstack.eu-west-1.amazonaws.com/arduino/original/3X/8/d/8ddfc2d335f178e70e20974439a94ce31dd9df35.pdf
//
// Cross-checked against Adafruit's CircuitPython driver (register map and
// timing constants only - it does not implement a lux conversion) and
// ESPHome's ltr_als_ps component (which independently implements the same
// lux formula):
//
//	https://github.com/adafruit/Adafruit_CircuitPython_LTR329_LTR303
//	https://esphome.io/components/sensor/ltr_als_ps/
package ltr329

import (
	"encoding/binary"
	"errors"
	"time"

	"tinygo.org/x/drivers"
)

var (
	errNotConnected                      = errors.New("ltr329: no LTR-329ALS-01 found on the bus")
	errConversionTimeout                 = errors.New("ltr329: timeout waiting for a new ALS conversion")
	errDataInvalid                       = errors.New("ltr329: ALS data marked invalid by the sensor")
	errIntegrationExceedsMeasurementRate = errors.New("ltr329: integration time must not exceed measurement rate")
)

// wakeupTime is slightly above the datasheet's documented 10ms maximum
// wakeup time from standby to active mode.
const wakeupTime = 12 * time.Millisecond

// Device wraps an I2C connection to an LTR-329ALS-01 device.
type Device struct {
	bus     drivers.I2C
	Address uint16

	gain            Gain
	integrationTime IntegrationTime
	measurementRate MeasurementRate

	// Last successful reading, for the drivers.Sensor accessors.
	lastCh0, lastCh1 uint16
	lastMilliLux     int32
	lastInvalid      bool

	// Scratch buffers: register address (+ value) for writes, up to all
	// four data registers for reads.
	wbuf [2]byte
	rbuf [4]byte
}

// Config holds the configuration applied by Configure.
type Config struct {
	// Gain selects the analog gain of both photodiode channels. The zero
	// value is GainX1 (1x).
	Gain Gain
	// IntegrationTime selects how long each ADC conversion integrates for.
	// The zero value is IntegrationTime100ms.
	IntegrationTime IntegrationTime
	// MeasurementRate selects how often the ALS_DATA registers refresh.
	// The zero value is MeasurementRate500ms. It must be at least
	// IntegrationTime, since the sensor cannot start a new conversion
	// before the previous one has finished.
	MeasurementRate MeasurementRate
}

// New creates a new LTR-329ALS-01 connection. The I2C bus must already be
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

// Connected returns whether an LTR-329ALS-01 answers with the expected
// part number and manufacturer ID on the bus.
func (d *Device) Connected() bool {
	part, err := d.readRegister(regPartID)
	if err != nil || part>>4 != partNumberID {
		return false
	}
	manufacturer, err := d.readRegister(regManufacID)
	return err == nil && manufacturer == manufacturerID
}

// Configure applies the given gain, integration time and measurement rate,
// and powers on the ALS. It returns an error without touching the device
// if no LTR-329ALS-01 answers on the bus, or if the requested integration
// time exceeds the requested measurement rate.
func (d *Device) Configure(cfg Config) error {
	if !d.Connected() {
		return errNotConnected
	}
	return d.applyConfig(cfg.Gain, cfg.IntegrationTime, cfg.MeasurementRate)
}

// SetGain changes the analog gain applied to both channels.
func (d *Device) SetGain(gain Gain) error {
	return d.applyConfig(gain, d.integrationTime, d.measurementRate)
}

// SetIntegrationTime changes the ADC integration time.
func (d *Device) SetIntegrationTime(t IntegrationTime) error {
	return d.applyConfig(d.gain, t, d.measurementRate)
}

// SetMeasurementRate changes how often the ALS_DATA registers refresh.
func (d *Device) SetMeasurementRate(r MeasurementRate) error {
	return d.applyConfig(d.gain, d.integrationTime, r)
}

// Gain returns the gain most recently applied by Configure or SetGain.
func (d *Device) Gain() Gain { return d.gain }

// IntegrationTime returns the integration time most recently applied by
// Configure or SetIntegrationTime.
func (d *Device) IntegrationTime() IntegrationTime { return d.integrationTime }

// MeasurementRate returns the measurement rate most recently applied by
// Configure or SetMeasurementRate.
func (d *Device) MeasurementRate() MeasurementRate { return d.measurementRate }

func (d *Device) applyConfig(gain Gain, t IntegrationTime, r MeasurementRate) error {
	if !gain.valid() {
		return errInvalidGain
	}
	if !t.valid() {
		return errInvalidIntegrationTime
	}
	if !r.valid() {
		return errInvalidMeasurementRate
	}
	if t.milliseconds() > r.milliseconds() {
		return errIntegrationExceedsMeasurementRate
	}

	// ALS_MEAS_RATE is written before ALS_CONTR so the sensor already
	// knows the requested integration/measurement rate for the very
	// first conversion it starts once ALS_CONTR enables active mode,
	// rather than briefly running under whatever it had before.
	if err := d.writeRegister(regALSMeasRate, t.registerBits()|r.registerBits()); err != nil {
		return err
	}
	if err := d.writeRegister(regALSContr, uint8(gain)|alsContrModeActive); err != nil {
		return err
	}

	time.Sleep(wakeupTime)

	d.gain, d.integrationTime, d.measurementRate = gain, t, r
	return nil
}

// ReadChannels performs a fresh reading and returns the raw ADC counts:
// ch0 is the full-spectrum (visible+IR) channel, ch1 is infrared-only. It
// waits for a conversion to complete first.
func (d *Device) ReadChannels() (ch0, ch1 uint16, err error) {
	status, err := d.awaitNewData()
	if err != nil {
		return 0, 0, err
	}

	if err := d.readRegisters(regALSDataCH1L, d.rbuf[:4]); err != nil {
		return 0, 0, err
	}
	ch1 = binary.LittleEndian.Uint16(d.rbuf[0:2])
	ch0 = binary.LittleEndian.Uint16(d.rbuf[2:4])

	d.lastCh0, d.lastCh1 = ch0, ch1
	d.lastInvalid = status&statusDataInvalid != 0
	return ch0, ch1, nil
}

// Illuminance reads both channels and converts them to illuminance in
// milliLux (1/1000 lux); see lux.go for the formula used and its accuracy
// limitations.
//
// If the sensor itself marks the reading invalid (ALS_STATUS bit 7 - this
// is the closest thing this part has to the TSL2591's saturation flag,
// since it does not publish a fixed full-scale ADC count to compare
// against), this returns errDataInvalid rather than a plausible-looking
// but wrong lux value. DataInvalid and MilliLux report the same outcome
// for later inspection. Reduce the gain or integration time and read
// again.
func (d *Device) Illuminance() (int32, error) {
	ch0, ch1, err := d.ReadChannels()
	if err != nil {
		return 0, err
	}

	if d.lastInvalid {
		d.lastMilliLux = 0
		return 0, errDataInvalid
	}

	d.lastMilliLux = calculateMilliLux(ch0, ch1, d.gain.nominalMultiplier(), d.integrationTime.milliseconds())
	return d.lastMilliLux, nil
}

// awaitNewData polls ALS_STATUS until the new-data flag is set (a
// conversion has completed since it was last read) or the worst case time
// between conversions has elapsed, and returns the status byte observed
// at that point so callers don't need a second read to get the
// data-invalid bit for the same conversion.
func (d *Device) awaitNewData() (status uint8, err error) {
	const pollInterval = 5 * time.Millisecond
	// Bounded by measurement rate (the interval between conversions once
	// running) plus integration time and wakeup time, to also cover the
	// very first read after Configure.
	deadline := time.Duration(d.measurementRate.milliseconds())*time.Millisecond +
		time.Duration(d.integrationTime.milliseconds())*time.Millisecond +
		wakeupTime

	for elapsed := time.Duration(0); ; elapsed += pollInterval {
		status, err = d.readRegister(regALSStatus)
		if err != nil {
			return 0, err
		}
		if status&statusNewData != 0 {
			return status, nil
		}
		if elapsed >= deadline {
			return 0, errConversionTimeout
		}
		time.Sleep(pollInterval)
	}
}

// Status reads the raw ALS_STATUS register (0x8C): bit 7 is data-invalid,
// bits 6:4 are the gain the current reading was taken at, and bit 2 is set
// when a conversion has completed since ALS_DATA was last read.
func (d *Device) Status() (uint8, error) {
	return d.readRegister(regALSStatus)
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
// DataInvalid reports true, or before any reading has succeeded.
func (d *Device) MilliLux() int32 { return d.lastMilliLux }

// Channels returns the raw ADC counts from the most recent call to
// ReadChannels, Illuminance or Update: ch0 is the full-spectrum
// (visible+IR) channel, ch1 is infrared-only.
func (d *Device) Channels() (ch0, ch1 uint16) { return d.lastCh0, d.lastCh1 }

// DataInvalid reports whether the most recent call to ReadChannels,
// Illuminance or Update found the sensor's own data-invalid flag set,
// meaning MilliLux does not reflect a real reading. Reduce the gain or
// integration time and read again.
func (d *Device) DataInvalid() bool { return d.lastInvalid }
