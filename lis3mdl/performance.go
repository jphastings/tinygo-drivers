package lis3mdl

// FullScale selects the magnetometer's full-scale range. Its value maps
// directly onto the FS[1:0] bits of CTRL_REG2 (datasheet Table 24).
type FullScale uint8

const (
	Scale4Gauss  FullScale = iota // ±4 gauss, 6842 LSB/gauss (default)
	Scale8Gauss                   // ±8 gauss, 3421 LSB/gauss
	Scale12Gauss                  // ±12 gauss, 2281 LSB/gauss - also the range the datasheet's self-test limits assume, see selftest.go
	Scale16Gauss                  // ±16 gauss, 1711 LSB/gauss
)

// sensitivity returns the scale's gain in LSB/gauss (datasheet "Magnetic
// and electrical characteristics" table, row GN).
func (s FullScale) sensitivity() int32 {
	switch s {
	case Scale8Gauss:
		return 3421
	case Scale12Gauss:
		return 2281
	case Scale16Gauss:
		return 1711
	default:
		return 6842
	}
}

// PerformanceMode selects the operating mode used for data conversion,
// trading power consumption and noise for measurement time. Its value maps
// directly onto both the OM[1:0] bits of CTRL_REG1 (X/Y axes, Table 20) and
// the OMZ[1:0] bits of CTRL_REG4 (Z axis, Table 30) - SetPerformanceMode
// writes both from the same value.
type PerformanceMode uint8

const (
	LowPower PerformanceMode = iota // lowest power, most noise (default)
	MediumPerformance
	HighPerformance
	UltraHighPerformance // highest power, least noise
)

// DataRate selects the output data rate used while FastODR is disabled.
// Its value maps directly onto the DO[2:0] bits of CTRL_REG1 (datasheet
// Table 21).
type DataRate uint8

const (
	DataRate0Hz625 DataRate = iota // slowest, least power-hungry (default)
	DataRate1Hz25
	DataRate2Hz5
	DataRate5Hz
	DataRate10Hz
	DataRate20Hz
	DataRate40Hz
	DataRate80Hz
)

// OperatingMode selects the system operating mode. Its value maps directly
// onto the MD[1:0] bits of CTRL_REG3 (datasheet Table 27), except that
// ModePowerDown always writes 0b10: the datasheet defines both 0b10 and
// 0b11 as power-down, and this driver only ever produces the former.
type OperatingMode uint8

const (
	ModeContinuous OperatingMode = iota // free-runs at the configured DataRate (default)
	ModeSingle                          // takes one measurement, then powers down
	ModePowerDown
)

// SetScale changes the full-scale range used to convert future
// ReadMagneticField results.
func (d *Device) SetScale(scale FullScale) error {
	d.scale = scale
	return d.writeCtrlReg2()
}

// Scale returns the full-scale range most recently applied by Configure or
// SetScale.
func (d *Device) Scale() FullScale { return d.scale }

// SetPerformanceMode changes the operating mode for all three axes,
// writing CTRL_REG1's OM[1:0] and CTRL_REG4's OMZ[1:0] together. These are
// independent fields in hardware; writing only one would leave the other
// axis pair's mode unchanged; the Z axis would still be readable, just
// noisier than X and Y, silently.
func (d *Device) SetPerformanceMode(mode PerformanceMode) error {
	d.performanceMode = mode
	if err := d.writeCtrlReg1(); err != nil {
		return err
	}
	return d.writeCtrlReg4()
}

// PerformanceMode returns the operating mode most recently applied by
// Configure or SetPerformanceMode.
func (d *Device) PerformanceMode() PerformanceMode { return d.performanceMode }

// SetDataRate changes the output data rate used while FastODR is disabled.
func (d *Device) SetDataRate(rate DataRate) error {
	d.dataRate = rate
	return d.writeCtrlReg1()
}

// DataRate returns the output data rate most recently applied by Configure
// or SetDataRate.
func (d *Device) DataRate() DataRate { return d.dataRate }

// SetFastODR enables (true) or disables (false) data rates above 80Hz. When
// enabled, the actual rate is fixed by PerformanceMode (1000/560/300/155Hz
// for low/medium/high/ultra-high performance respectively, datasheet Table
// 19) and DataRate is ignored by the hardware.
func (d *Device) SetFastODR(enabled bool) error {
	d.fastODR = enabled
	return d.writeCtrlReg1()
}

// FastODR returns the FastODR setting most recently applied by Configure or
// SetFastODR.
func (d *Device) FastODR() bool { return d.fastODR }

// SetOperatingMode changes the system operating mode: continuous,
// single-shot, or power-down.
func (d *Device) SetOperatingMode(mode OperatingMode) error {
	d.operatingMode = mode
	return d.writeCtrlReg3()
}

// OperatingMode returns the operating mode most recently applied by
// Configure or SetOperatingMode.
func (d *Device) OperatingMode() OperatingMode { return d.operatingMode }

func (d *Device) writeCtrlReg1() error {
	// TEMP_EN is always set: it costs nothing to leave on and ReadTemperature
	// depends on it.
	v := uint8(1) << shiftTempEn
	v |= uint8(d.performanceMode) << shiftOM
	v |= uint8(d.dataRate) << shiftDO
	if d.fastODR {
		v |= 1 << shiftFastODR
	}
	if d.selfTest {
		v |= 1 << shiftST
	}
	return d.writeRegister(regCtrlReg1, v)
}

func (d *Device) writeCtrlReg2() error {
	return d.writeRegister(regCtrlReg2, uint8(d.scale)<<shiftFS)
}

func (d *Device) writeCtrlReg3() error {
	return d.writeRegister(regCtrlReg3, uint8(d.operatingMode)<<shiftMD)
}

func (d *Device) writeCtrlReg4() error {
	// OMZ tracks the same PerformanceMode as OM; see SetPerformanceMode.
	return d.writeRegister(regCtrlReg4, uint8(d.performanceMode)<<shiftOMZ)
}

func (d *Device) writeCtrlReg5() error {
	// BDU (block data update) holds OUT_x_L/OUT_x_H steady once the first
	// byte of a pair has been read, until the second is read too - without
	// it a conversion completing mid-read can tear a sample across two
	// different readings.
	return d.writeRegister(regCtrlReg5, 1<<shiftBDU)
}
