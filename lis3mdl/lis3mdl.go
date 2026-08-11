// Package lis3mdl provides a driver for the ST LIS3MDL, an ultra-low-power,
// high-performance 3-axis magnetometer with a selectable full scale of
// ±4/±8/±12/±16 gauss. It is, for example, the magnetometer half of
// Adafruit's LSM6DS3TR-C + LIS3MDL 9-DoF IMU breakout; the accelerometer
// and gyro half of that board is covered by the tinygo.org/x/drivers
// lsm6ds3tr package, not this one.
//
// Two details of the register interface are easy to get wrong and are
// called out where they matter:
//
//   - Multi-byte I2C reads require the auto-increment bit (bit 7 of the
//     sub-address) to be set; without it, every byte of a burst read comes
//     back from the same register. See io.go.
//   - The X/Y operating mode (CTRL_REG1's OM[1:0]) and the Z operating mode
//     (CTRL_REG4's OMZ[1:0]) are two separate fields. SetPerformanceMode
//     writes both together so Z is never left in low-power mode - and
//     therefore noisier - by accident. See performance.go.
//
// Datasheet:
//
//	https://www.st.com/resource/en/datasheet/lis3mdl.pdf
//
// This driver was cross-checked against Adafruit's Arduino and
// CircuitPython libraries and STMicroelectronics' own reference C driver
// and self-test application note:
//
//	https://github.com/adafruit/Adafruit_LIS3MDL
//	https://github.com/adafruit/Adafruit_CircuitPython_LIS3MDL
//	https://github.com/STMicroelectronics/lis3mdl-pid
//	https://github.com/STMicroelectronics/STMems_Standard_C_drivers/blob/master/lis3mdl_STdC/examples/lis3mdl_self_test.c
package lis3mdl // import "tinygo.org/x/drivers/lis3mdl"

import (
	"errors"
	"time"

	"tinygo.org/x/drivers"
)

var (
	errNotConnected = errors.New("lis3mdl: no LIS3MDL found on the bus")
	errResetTimeout = errors.New("lis3mdl: timeout waiting for software reset to complete")
)

// Device wraps an I2C connection to a LIS3MDL device.
type Device struct {
	bus     drivers.I2C
	Address uint16

	scale           FullScale
	performanceMode PerformanceMode
	dataRate        DataRate
	fastODR         bool
	operatingMode   OperatingMode
	selfTest        bool

	// Last successful reading, for the drivers.Sensor accessors in update.go.
	lastX, lastY, lastZ int32
	lastMilliCelsius    int32

	// Scratch buffers: register address (+ value) for writes, up to six
	// output registers for reads.
	wbuf [2]byte
	rbuf [6]byte
}

// Config holds the configuration applied by Configure.
type Config struct {
	// Scale selects the full-scale magnetic range. The zero value is
	// Scale4Gauss, the most sensitive range.
	Scale FullScale
	// PerformanceMode selects the X, Y and Z axis operating mode, trading
	// power and noise for measurement rate. The zero value is LowPower.
	PerformanceMode PerformanceMode
	// DataRate selects the output data rate used while FastODR is false.
	// The zero value is DataRate0Hz625, the slowest and least
	// power-hungry rate.
	DataRate DataRate
	// FastODR enables output data rates above 80Hz, at a fixed rate
	// determined by PerformanceMode instead of DataRate. The zero value,
	// false, means DataRate is used as configured.
	FastODR bool
	// OperatingMode selects continuous, single-shot or power-down
	// operation. The zero value is ModeContinuous.
	OperatingMode OperatingMode
}

// New creates a new LIS3MDL connection. The I2C bus must already be
// configured. Address defaults to AddressLow, the address used when the
// SDO/SA1 pin is tied low - the case on Adafruit's LSM6DS3TR-C + LIS3MDL
// breakout.
//
// This function only creates the Device object, it does not touch the
// device.
func New(bus drivers.I2C) Device {
	return Device{
		bus:     bus,
		Address: AddressLow,
	}
}

// Connected returns whether a LIS3MDL answers with the expected device ID
// on the bus.
func (d *Device) Connected() bool {
	id, err := d.readRegister(regWhoAmI)
	return err == nil && id == deviceID
}

// Configure resets the device to a known state and applies cfg. It returns
// an error without touching the device further if no LIS3MDL answers on
// the bus.
func (d *Device) Configure(cfg Config) error {
	if !d.Connected() {
		return errNotConnected
	}
	if err := d.reset(); err != nil {
		return err
	}

	d.scale = cfg.Scale
	d.performanceMode = cfg.PerformanceMode
	d.dataRate = cfg.DataRate
	d.fastODR = cfg.FastODR
	d.operatingMode = cfg.OperatingMode
	d.selfTest = false

	if err := d.writeCtrlReg1(); err != nil {
		return err
	}
	if err := d.writeCtrlReg2(); err != nil {
		return err
	}
	if err := d.writeCtrlReg3(); err != nil {
		return err
	}
	if err := d.writeCtrlReg4(); err != nil {
		return err
	}
	return d.writeCtrlReg5()
}

// reset triggers CTRL_REG2's SOFT_RST bit, restoring every user register to
// its power-on default, and waits for the device to clear the bit itself -
// the same sequence ST's own self-test example runs before configuring the
// device, so Configure never inherits state (e.g. self-test still enabled)
// left over from earlier use.
func (d *Device) reset() error {
	if err := d.writeRegister(regCtrlReg2, 1<<shiftSoftRst); err != nil {
		return err
	}

	const attempts = 20
	for i := 0; i < attempts; i++ {
		v, err := d.readRegister(regCtrlReg2)
		if err != nil {
			return err
		}
		if v&(1<<shiftSoftRst) == 0 {
			return nil
		}
		time.Sleep(time.Millisecond)
	}
	return errResetTimeout
}
