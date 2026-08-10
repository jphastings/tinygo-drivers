// Package mmc5983 provides a driver for the MEMSIC MMC5983MA, a highly
// sensitive 3-axis magnetometer with 18-bit resolution over a ±8 gauss
// range, for example on the SparkFun Micro Magnetometer breakout:
// https://www.sparkfun.com/sparkfun-micro-magnetometer-mmc5983ma-qwiic.html
//
// The chip's control registers are write-only: reading them returns
// undefined data. The driver therefore keeps a RAM shadow of each control
// register and writes whole registers from the shadow, instead of doing
// read-modify-write on the hardware.
//
// Datasheet:
//
//	https://media.digikey.com/pdf/Data%20Sheets/MEMSIC%20PDFs/MMC5983MA_RevA_4-3-19.pdf
//
// This driver is inspired by the SparkFun Arduino library:
//
//	https://github.com/sparkfun/SparkFun_MMC5983MA_Magnetometer_Arduino_Library
package mmc5983

import (
	"errors"
	"time"

	"tinygo.org/x/drivers"
)

var (
	errNotConnected    = errors.New("mmc5983: no MMC5983MA found on the bus")
	errTimeoutExceeded = errors.New("mmc5983: timeout waiting for measurement")
)

// Device wraps an I2C connection to an MMC5983MA device.
type Device struct {
	bus     drivers.I2C
	Address uint16

	// The chip's four control registers are write-only, so their current
	// contents are shadowed here.
	memoryShadow controlRegisterShadow

	// Scratch buffers: register address (+ value) for writes, XYZ output
	// registers for reads.
	wbuf [2]byte
	rbuf [7]byte
}

// Config holds the configuration applied by Configure.
type Config struct {
	// Bandwidth selects the decimation filter bandwidth, which determines
	// the duration of each measurement. The zero value is Bandwidth100Hz.
	Bandwidth Bandwidth
}

// New creates a new MMC5983MA connection. The I2C bus must already be
// configured.
//
// This function only creates the Device object, it does not touch the
// device.
func New(bus drivers.I2C) Device {
	return Device{
		bus:     bus,
		Address: I2C_ADDR,
	}
}

// Connected returns whether an MMC5983MA answers with the expected product
// ID on the bus.
func (d *Device) Connected() bool {
	prodID, err := d.readSingleByte(PROD_ID_REG)
	return err == nil && prodID == PROD_ID
}

// Configure resets the device and applies the given configuration.
func (d *Device) Configure(cfg Config) error {
	if !d.Connected() {
		return errNotConnected
	}
	if err := d.SoftwareReset(); err != nil {
		return err
	}
	// How the sensing elements are magnetized survives a software reset — it
	// is a physical state, not a register — and a RESET inverts every axis.
	// SET them so that readings have a defined sign whatever ran beforehand.
	if err := d.PerformSet(); err != nil {
		return err
	}
	return d.SetBandwidth(cfg.Bandwidth)
}

// SoftwareReset resets the device, similar to a power-up: it clears all
// registers and rereads the chip's OTP memory. It takes about 10ms, so this
// call sleeps for 15ms to be safe.
func (d *Device) SoftwareReset() error {
	if err := d.operateWithShadow(INT_CTRL_1_REG, opSet|opWrite, BITS_SW_RST); err != nil {
		return err
	}

	// The reset clears every register on the chip, so zero the whole shadow
	// to stay in sync.
	d.memoryShadow = controlRegisterShadow{}

	time.Sleep(15 * time.Millisecond)
	return nil
}

// PerformSet runs a SET operation: a large current pulse through the
// sensor's internal coils, restoring (degaussing) the sensing elements
// after exposure to a strong magnetic field.
func (d *Device) PerformSet() error {
	return d.pulseOperation(BITS_SET_OPERATION)
}

// PerformReset runs a RESET operation: like PerformSet, but the current
// flows in the opposite direction, magnetizing the sensing elements with
// the reverse polarity.
func (d *Device) PerformReset() error {
	return d.pulseOperation(BITS_RESET_OPERATION)
}

func (d *Device) pulseOperation(bitMask uint8) error {
	if err := d.operateWithShadow(INT_CTRL_0_REG, opSet|opWrite, bitMask); err != nil {
		return err
	}

	// The bit self-clears in hardware at the end of the operation, so clear
	// it in shadow memory only.
	d.operateWithShadow(INT_CTRL_0_REG, opClear, bitMask)

	// The operation itself only takes 500ns.
	time.Sleep(time.Millisecond)
	return nil
}
