// Package lc709203 provides a driver for the ON Semiconductor LC709203F, a
// fuel gauge IC for a single-cell lithium-ion/polymer battery. It estimates
// state of charge from voltage, temperature and the battery's internal
// impedance (ON Semiconductor's "HG-CVR" algorithm) rather than from a
// coulomb-counting sense resistor.
//
// # The gauge is powered by the battery, not by the I2C bus
//
// The LC709203F draws its own supply from the cell it measures: the
// datasheet's pin description (Table 1) says to wire VDD straight to the
// battery's positive terminal and VSS to its negative terminal - there is
// no separate VIN/logic-supply pin. With no battery connected, the chip
// has no power and will not acknowledge its I2C address at all, which
// looks identical to nothing being wired to that address in the first
// place. This is the single most common reason a fresh board "doesn't
// respond", and it is expected, not a fault: Adafruit's own guide for
// their LC709203F breakout puts it plainly, "Don't disconnect the LiPo
// battery, it is used to power the LC chip!". Connect a battery before
// suspecting the wiring or this driver.
//
// # CRC-8 packet error checking
//
// Every I2C transaction is followed by a CRC-8 byte covering the whole
// transaction, including the address byte(s); see crc.go. Getting this
// wrong does not fail the I2C transaction itself, so a broken
// implementation silently reads back wrong numbers or drops writes - this
// driver validates every read and generates a correct CRC on every write.
//
// # Setup for accurate readings
//
// RSOC (state of charge) and ITE are not meaningful out of the box: the
// gauge needs the connected battery's design capacity (PackSize, the
// Adjustment Pack Application register) and a matching BatteryProfile
// (Change of the Parameter register) - see Config. Skipping either leaves
// HG-CVR using whatever pack size/impedance curve it last had (typically
// none, on a factory-fresh part), and RSOC will not track the real battery.
//
// Datasheet:
//
//	https://cdn-learn.adafruit.com/assets/assets/000/094/597/original/LC709203F-D.PDF
//
// This driver is inspired by Adafruit's Arduino and CircuitPython
// libraries:
//
//	https://github.com/adafruit/Adafruit_LC709203F
//	https://github.com/adafruit/Adafruit_CircuitPython_LC709203F
package lc709203

import (
	"errors"

	"tinygo.org/x/drivers"
)

var (
	errNotConnected          = errors.New("lc709203: no LC709203F found on the bus")
	errCRCMismatch           = errors.New("lc709203: CRC-8 mismatch in I2C response")
	errInvalidPackSize       = errors.New("lc709203: invalid pack size, APA must fit in a byte (0x0000-0x00FF)")
	errInvalidPercent        = errors.New("lc709203: invalid percentage, must be 0-100")
	errTemperatureOutOfRange = errors.New("lc709203: temperature outside the I2C-mode write range, -20.0 to 60.0 degC")
)

// Device wraps an I2C connection to an LC709203F device.
type Device struct {
	bus     drivers.I2C
	Address uint16

	// Last successful reading, for the drivers.Sensor accessors.
	lastMilliVolts   uint16
	lastMilliCelsius int32

	// Scratch buffers: command byte (+ up to 2 data bytes + CRC) for
	// writes, up to 2 data bytes + CRC for reads.
	wbuf [4]byte
	rbuf [3]byte
}

// Config holds the configuration applied by Configure.
type Config struct {
	// PowerMode selects Operate or Sleep. The zero value defaults to
	// PowerModeOperate: there is no register encoding for "off" to fall
	// back to safely, so the usual zero-value-is-a-safe-default rule
	// doesn't apply to the raw register value here.
	PowerMode PowerMode

	// PackSize is the design capacity of the connected battery. The zero
	// value leaves the Adjustment Pack Application register at 0x0000,
	// which the datasheet documents no typical capacity for - RSOC/ITE
	// should not be trusted until this is set correctly. See PackSize's
	// doc comment for where its named constants come from.
	PackSize PackSize

	// BatteryProfile selects which of the IC's two factory-loaded battery
	// profiles to use; see BatteryProfile's doc comment. The zero value,
	// BatteryProfile0, is also the register's own power-on default.
	BatteryProfile BatteryProfile

	// TemperatureSource selects how the Cell Temperature register is kept
	// current. The zero value, TemperatureSourceI2C, requires calling
	// SetTemperature periodically; TemperatureSourceThermistor instead has
	// the chip measure a thermistor itself.
	TemperatureSource TemperatureSource

	// ThermistorB is the B-constant of the thermistor wired to TSENSE,
	// only used when TemperatureSource is TemperatureSourceThermistor. The
	// zero value uses the chip's own power-on default, 3380.
	ThermistorB uint16
}

// New creates a new LC709203F connection. The I2C bus must already be
// configured.
//
// This function only creates the Device object, it does not touch the
// device - and touching it before a battery is connected would find
// nothing there regardless; see the package doc.
func New(bus drivers.I2C) Device {
	return Device{
		bus:     bus,
		Address: I2CAddress,
	}
}

// Connected returns whether an LC709203F answers on the bus. The datasheet
// does not publish a fixed expected value for ICVersion the way most
// chips do for a product/device ID register, so this instead relies on a
// read succeeding with a valid CRC-8 - which, given the CRC covers the
// address bytes too, is still strong evidence a real LC709203F answered
// rather than some other device on 0x0B.
func (d *Device) Connected() bool {
	_, err := d.ICVersion()
	return err == nil
}

// Configure applies the given configuration. It returns an error without
// touching the device if no LC709203F answers on the bus - most likely
// because no battery is connected; see the package doc.
func (d *Device) Configure(cfg Config) error {
	if !d.Connected() {
		return errNotConnected
	}

	mode := cfg.PowerMode
	if mode == 0 {
		mode = PowerModeOperate
	}
	if err := d.SetPowerMode(mode); err != nil {
		return err
	}
	if err := d.SetPackSize(cfg.PackSize); err != nil {
		return err
	}
	if err := d.SetBatteryProfile(cfg.BatteryProfile); err != nil {
		return err
	}
	if err := d.SetTemperatureSource(cfg.TemperatureSource); err != nil {
		return err
	}
	if cfg.TemperatureSource == TemperatureSourceThermistor {
		thermistorB := cfg.ThermistorB
		if thermistorB == 0 {
			thermistorB = defaultThermistorB
		}
		if err := d.SetThermistorB(thermistorB); err != nil {
			return err
		}
	}

	return nil
}

// CellVoltage returns the measured battery voltage in millivolts.
func (d *Device) CellVoltage() (uint16, error) {
	v, err := d.readWord(cmdCellVoltage)
	if err != nil {
		return 0, err
	}
	d.lastMilliVolts = v
	return v, nil
}

// RSOC returns the Relative State Of Charge as a percentage, 0-100. It is
// not meaningful until PackSize and BatteryProfile are set for the battery
// in use; see Config.
func (d *Device) RSOC() (uint8, error) {
	v, err := d.readWord(cmdRSOC)
	return uint8(v), err
}

// SetRSOC manually corrects the reported RSOC. This is not needed in
// normal operation - HG-CVR tracks it on its own - but writing it also
// updates ITE to match. Sleep mode is required to make a written value
// stick; in Operate mode it converges back based on the measured battery
// state.
func (d *Device) SetRSOC(percent uint8) error {
	if percent > 100 {
		return errInvalidPercent
	}
	return d.writeWord(cmdRSOC, uint16(percent))
}

// ITE returns the Indicator To Empty: RSOC at finer, 0.1% resolution, as
// tenths of a percent over the range 0-1000 (so 456 means 45.6%).
func (d *Device) ITE() (uint16, error) {
	return d.readWord(cmdITE)
}

// CellTemperature returns the battery temperature in milli-degrees
// Celsius. In TemperatureSourceThermistor mode this reflects the attached
// thermistor; in TemperatureSourceI2C mode it reflects whatever was last
// written with SetTemperature.
func (d *Device) CellTemperature() (int32, error) {
	v, err := d.readWord(cmdCellTemperature)
	if err != nil {
		return 0, err
	}
	milliC := rawToMilliCelsius(v)
	d.lastMilliCelsius = milliC
	return milliC, nil
}

// SetTemperature writes the battery temperature in milli-degrees Celsius,
// for TemperatureSourceI2C mode; it has no effect on the reading in
// TemperatureSourceThermistor mode. The datasheet only permits -20.0 to
// 60.0 degC in this register; during charge/discharge it should be
// refreshed whenever the true temperature changes by more than 1 degC.
func (d *Device) SetTemperature(milliC int32) error {
	if milliC < -20000 || milliC > 60000 {
		return errTemperatureOutOfRange
	}
	return d.writeWord(cmdCellTemperature, milliCelsiusToRaw(milliC))
}

// SetTemperatureSource selects how the Cell Temperature register is
// populated; see TemperatureSource.
func (d *Device) SetTemperatureSource(src TemperatureSource) error {
	if !src.valid() {
		return errInvalidTemperatureSource
	}
	return d.writeWord(cmdStatusBit, uint16(src))
}

// TemperatureSource reads back the current temperature source.
func (d *Device) TemperatureSource() (TemperatureSource, error) {
	v, err := d.readWord(cmdStatusBit)
	return TemperatureSource(v), err
}

// SetThermistorB sets the B-constant of the thermistor wired to TSENSE,
// used only in TemperatureSourceThermistor mode. Refer to the thermistor's
// own datasheet for its B-constant.
func (d *Device) SetThermistorB(b uint16) error {
	return d.writeWord(cmdThermistorB, b)
}

// ThermistorB reads back the configured thermistor B-constant.
func (d *Device) ThermistorB() (uint16, error) {
	return d.readWord(cmdThermistorB)
}

// SetAPT adjusts the Adjustment Pack Thermistor register, which
// compensates for measurement delay introduced by a capacitor placed
// across the thermistor for ESD protection. The chip's own default,
// 0x001E, suits circuits without such a capacitor; most applications never
// need to change this.
func (d *Device) SetAPT(v uint16) error {
	return d.writeWord(cmdAPT, v)
}

// APT reads back the current Adjustment Pack Thermistor value.
func (d *Device) APT() (uint16, error) {
	return d.readWord(cmdAPT)
}

// SetPackSize sets the Adjustment Pack Application (APA) register: the
// design capacity of the connected battery. See PackSize's doc comment for
// where its named constants come from and when they don't apply.
func (d *Device) SetPackSize(size PackSize) error {
	if size > 0x00FF {
		return errInvalidPackSize
	}
	return d.writeWord(cmdAPA, uint16(size))
}

// PackSize reads back the configured Adjustment Pack Application value.
func (d *Device) PackSize() (PackSize, error) {
	v, err := d.readWord(cmdAPA)
	return PackSize(v), err
}

// SetBatteryProfile selects which of the IC's two factory-loaded battery
// profiles to use; see BatteryProfile's doc comment.
func (d *Device) SetBatteryProfile(p BatteryProfile) error {
	if !p.valid() {
		return errInvalidBatteryProfile
	}
	return d.writeWord(cmdChangeOfParameter, uint16(p))
}

// BatteryProfile reads back the currently selected battery profile.
func (d *Device) BatteryProfile() (BatteryProfile, error) {
	v, err := d.readWord(cmdChangeOfParameter)
	return BatteryProfile(v), err
}

// NumberOfParameter reads back which of the IC's two factory data files is
// active (0x0301 or 0x0504 per the datasheet), confirming what
// BatteryProfile actually selected.
func (d *Device) NumberOfParameter() (uint16, error) {
	return d.readWord(cmdNumberOfParameter)
}

// SetCurrentDirection selects how RSOC is permitted to move; see
// CurrentDirection.
func (d *Device) SetCurrentDirection(dir CurrentDirection) error {
	if !dir.valid() {
		return errInvalidCurrentDirection
	}
	return d.writeWord(cmdCurrentDirection, uint16(dir))
}

// CurrentDirection reads back the currently configured current direction.
func (d *Device) CurrentDirection() (CurrentDirection, error) {
	v, err := d.readWord(cmdCurrentDirection)
	return CurrentDirection(v), err
}

// SetPowerMode selects Operate or Sleep mode; see PowerMode.
func (d *Device) SetPowerMode(mode PowerMode) error {
	if !mode.valid() {
		return errInvalidPowerMode
	}
	return d.writeWord(cmdPowerMode, uint16(mode))
}

// PowerMode reads back the current power mode.
func (d *Device) PowerMode() (PowerMode, error) {
	v, err := d.readWord(cmdPowerMode)
	return PowerMode(v), err
}

// ICVersion returns the LSI's ID number. The datasheet does not publish a
// fixed expected value for it, which is why Connected does not compare
// against one; see Connected's doc comment.
func (d *Device) ICVersion() (uint16, error) {
	return d.readWord(cmdICVersion)
}

// SetAlarmLowRSOC sets the RSOC threshold, in percent, below which ALARMB
// asserts; 0 disables the alarm.
func (d *Device) SetAlarmLowRSOC(percent uint8) error {
	if percent > 100 {
		return errInvalidPercent
	}
	return d.writeWord(cmdAlarmLowRSOC, uint16(percent))
}

// AlarmLowRSOC reads back the configured low-RSOC alarm threshold.
func (d *Device) AlarmLowRSOC() (uint8, error) {
	v, err := d.readWord(cmdAlarmLowRSOC)
	return uint8(v), err
}

// SetAlarmLowVoltage sets the cell voltage threshold, in millivolts, below
// which ALARMB asserts; 0 disables the alarm.
func (d *Device) SetAlarmLowVoltage(mv uint16) error {
	return d.writeWord(cmdAlarmLowCellVoltage, mv)
}

// AlarmLowVoltage reads back the configured low-voltage alarm threshold,
// in millivolts.
func (d *Device) AlarmLowVoltage() (uint16, error) {
	return d.readWord(cmdAlarmLowCellVoltage)
}

// InitialRSOC forces RSOC to be recomputed from the battery's open-circuit
// voltage measured at the moment this is called. A power-on/battery
// insertion reset already does this automatically; call this again if the
// reported RSOC after Configure looks wrong. Keep system load under 0.025C
// while calling it, or the sampled voltage - and so the resulting RSOC -
// will be off.
func (d *Device) InitialRSOC() error {
	return d.writeWord(cmdInitialRSOC, rsocInitMagic)
}

// BeforeRSOC is like InitialRSOC, but initialises RSOC from the maximum
// battery voltage observed since the last reset rather than the
// instantaneous voltage - useful if load couldn't be kept low enough for
// InitialRSOC's instantaneous sample to be a true open-circuit voltage. Do
// not use this if the battery has been charged since the last reset.
func (d *Device) BeforeRSOC() error {
	return d.writeWord(cmdBeforeRSOC, rsocInitMagic)
}

// Update performs a fresh reading for each requested measurement.
// Supported measurements are drivers.Voltage (cell voltage) and
// drivers.Temperature (cell temperature); any other bits are ignored.
func (d *Device) Update(which drivers.Measurement) error {
	if which&drivers.Voltage != 0 {
		if _, err := d.CellVoltage(); err != nil {
			return err
		}
	}
	if which&drivers.Temperature != 0 {
		if _, err := d.CellTemperature(); err != nil {
			return err
		}
	}
	return nil
}

// MilliVolts returns the cell voltage from the most recent successful call
// to CellVoltage or Update, in millivolts.
func (d *Device) MilliVolts() uint16 { return d.lastMilliVolts }

// MilliCelsius returns the cell temperature from the most recent
// successful call to CellTemperature or Update, in milli-degrees Celsius.
func (d *Device) MilliCelsius() int32 { return d.lastMilliCelsius }
