// Package mpr121 provides a driver for the NXP (formerly Freescale)
// MPR121, a 12-channel proximity capacitive touch sensor controller. It
// measures each electrode's capacitance via a constant-current charge
// scheme, tracks a slow-moving baseline per electrode, and reports a
// touch whenever the filtered signal deviates from that baseline by
// more than a configurable threshold - all in hardware, so the host
// only needs to poll (or watch IRQ for) the resulting status bits.
//
// Two aspects of the register interface are easy to get wrong in a way
// I2C itself cannot detect:
//
//   - Registers 0x2B-0x7F (thresholds, baseline filtering, debounce,
//     AFE config and more) other than the GPIO/LED block and ECR itself
//     accept writes only while the device is in Stop Mode (ECR = 0x00).
//     A write attempted while it is running is silently ignored - no
//     I2C NAK, no readback difference until you go looking. This driver
//     handles it: every setter that touches one of those registers
//     drops into Stop Mode first and resumes Run Mode afterwards; see
//     withStopMode in io.go.
//   - The 8-bit Baseline Value registers store only the upper 8 bits of
//     an internal 10-bit baseline; see Baseline in data.go.
//
// Datasheet:
//
//	https://www.nxp.com/docs/en/data-sheet/MPR121.pdf
//
// Initialization sequence and default thresholds/filter values are from
// Freescale application note AN3944, "MPR121 Quick Start Guide" (now
// retired by NXP and folded into the datasheet itself), with one
// documented deviation - see clLoadFromFirstReading in config.go - plus
// a second where following AN3944 literally would leave the device
// unable to sense capacitance at all; see the comment on Configure.
//
// This driver is also inspired by Adafruit's Arduino library:
//
//	https://github.com/adafruit/Adafruit_MPR121
package mpr121

import (
	"errors"
	"time"

	"tinygo.org/x/drivers"
)

// The MPR121's ADDR pin selects one of four I2C addresses.
const (
	DefaultAddress uint8 = 0x5A // ADDR strapped to VSS (ground)
	AddressVDD     uint8 = 0x5B // ADDR strapped to VDD
	AddressSDA     uint8 = 0x5C // ADDR strapped to SDA
	AddressSCL     uint8 = 0x5D // ADDR strapped to SCL
)

var (
	errNotConnected          = errors.New("mpr121: no MPR121 found on the bus")
	errInvalidElectrodeCount = errors.New("mpr121: electrode count must be between 1 and 12")
	errInvalidElectrode      = errors.New("mpr121: electrode must be 0-11, or ElectrodeProximity")
	errReleaseNotBelowTouch  = errors.New("mpr121: release threshold must be below touch threshold")
	errInvalidProximityMode  = errors.New("mpr121: invalid proximity mode")
	errInvalidDebounce       = errors.New("mpr121: debounce sample counts must be 0-7")
)

// Device wraps an I2C connection to an MPR121.
type Device struct {
	bus     drivers.I2C
	Address uint8

	// lastECR is the Electrode Configuration Register value Configure
	// last established (electrode count, proximity mode and CL bits).
	// withStopMode restores it after any write that requires Stop Mode,
	// and ClearOverCurrent uses it to resume Run Mode after a fault.
	lastECR uint8

	// Scratch buffers: register address (+ value) for writes, up to a
	// filtered-data/touch-status register pair for reads.
	wbuf [2]byte
	rbuf [2]byte
}

// New creates a new MPR121 connection. address is one of DefaultAddress
// or, depending on how the ADDR pin is strapped, AddressVDD, AddressSDA
// or AddressSCL. The I2C bus must already be configured.
//
// This function only creates the Device object, it does not touch the
// device.
func New(bus drivers.I2C, address uint8) Device {
	return Device{
		bus:     bus,
		Address: address,
	}
}

// Connected reports whether something acknowledges I2C reads at
// Address. The MPR121 has no fixed device-ID register to check against,
// unlike many I2C parts, so a true result only shows that some device
// is present. Configure additionally checks the AFE/filter config
// registers' known power-on values immediately after a reset, which is
// a stronger signal that it is specifically an MPR121.
func (d *Device) Connected() bool {
	_, err := d.readRegister(regECR)
	return err == nil
}

// Reset issues a soft reset. Every register returns to its power-on
// value - all-zero, except regAFEConfig1 and regFilterConfig, see
// Configure - and the device is left in Stop Mode.
func (d *Device) Reset() error {
	if err := d.writeRegister(regSoftReset, softResetMagic); err != nil {
		return err
	}
	time.Sleep(time.Millisecond)
	return nil
}

// baselineFilterDefaults are AN3944 sections A and B: how quickly the
// tracked baseline follows the raw signal above (rising) and below
// (falling) it. The "touched" scenario (regNHDTouched etc.) has no
// AN3944 override, so Configure writes its own power-on-default value
// (zero) there instead, purely so that calling Configure again on an
// already-configured device is idempotent.
var baselineFilterDefaults = [...]struct{ reg, value uint8 }{
	{regMHDRising, 0x01}, {regNHDRising, 0x01}, {regNCLRising, 0x00}, {regFDLRising, 0x00},
	{regMHDFalling, 0x01}, {regNHDFalling, 0x01}, {regNCLFalling, 0xFF}, {regFDLFalling, 0x02},
	{regNHDTouched, 0x00}, {regNCLTouched, 0x00}, {regFDLTouched, 0x00},
}

// Configure resets the device and applies AN3944's recommended baseline
// filtering and touch/release thresholds, then enables
// cfg.ElectrodeCount electrodes (and, if requested, the proximity
// channel) and starts Run Mode. It returns an error, without leaving
// the device mid-configured, if cfg fails validation or if nothing
// answering like an MPR121 is found at Address.
//
// AN3944 also recommends writing regFilterConfig (0x5D) = 0x04 and
// enabling auto-configuration (registers 0x7B, 0x7D-0x7F); Configure
// does neither. regFilterConfig's top 3 bits are the global
// charge/discharge time (CDT), and zero there - which 0x04 sets - means
// "disable electrode charging" per the current datasheet, a bit layout
// for this register that AN3944 (2010) predates. AN3944 pairs that
// value with auto-configuration, which independently derives a working
// per-electrode charge time; without also driving that (out of scope
// here), applying 0x04 on its own would leave every electrode's
// effective charge time at zero and break capacitance sensing entirely.
// Configure instead leaves regAFEConfig1/regFilterConfig at their
// power-on defaults (0x10/0x24), which the datasheet documents as
// working values in their own right.
func (d *Device) Configure(cfg Config) error {
	electrodeCount, touch, release, err := cfg.resolve()
	if err != nil {
		return err
	}

	if err := d.Reset(); err != nil {
		return err
	}

	afe1, err := d.readRegister(regAFEConfig1)
	if err != nil {
		return err
	}
	filterCfg, err := d.readRegister(regFilterConfig)
	if err != nil {
		return err
	}
	if afe1 != afeConfig1PowerOnDefault || filterCfg != filterConfigPowerOnDefault {
		return errNotConnected
	}

	for _, w := range baselineFilterDefaults {
		if err := d.writeRegister(w.reg, w.value); err != nil {
			return err
		}
	}

	for e := uint8(0); e < NumElectrodes; e++ {
		if err := d.setElectrodeThresholdsRaw(e, touch, release); err != nil {
			return err
		}
	}

	// CL = 2b10, see clLoadFromFirstReading. ECR is written last, both
	// because it is what starts Run Mode and because AN3944 and the
	// datasheet's ECR section both say every other register must be
	// configured first.
	ecr := clLoadFromFirstReading<<6 | uint8(cfg.Proximity)<<4 | electrodeCount
	d.lastECR = ecr
	return d.writeRegister(regECR, ecr)
}
