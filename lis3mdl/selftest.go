package lis3mdl

import "time"

// Self-test limits, in mG (milligauss), for the absolute difference between
// the average of several readings taken with self-test off and the average
// of several readings taken with self-test on (datasheet "Magnetic and
// electrical characteristics" table, row ST, "Absolute value"). ST only
// publishes these limits at Scale12Gauss; they are not valid at any other
// Scale.
//
// STMicroelectronics' own reference self-test procedure - which these
// values are cross-checked against - runs it at SelfTestPerformanceMode and
// SelfTestDataRate specifically:
//
//	https://github.com/STMicroelectronics/STMems_Standard_C_drivers/blob/master/lis3mdl_STdC/examples/lis3mdl_self_test.c
//
// That procedure is: configure as below, discard the first reading after
// each change (wait for DataReady), average 5 readings with self-test off,
// enable self-test and wait at least SelfTestSettleTime, average 5 more
// readings, then compare the absolute difference per axis against the
// limits below.
const (
	SelfTestScale           = Scale12Gauss
	SelfTestPerformanceMode = LowPower
	SelfTestDataRate        = DataRate80Hz

	SelfTestMinXMilligauss int32 = 1000
	SelfTestMaxXMilligauss int32 = 3000
	SelfTestMinYMilligauss int32 = 1000
	SelfTestMaxYMilligauss int32 = 3000
	SelfTestMinZMilligauss int32 = 100
	SelfTestMaxZMilligauss int32 = 1000
)

// SelfTestSettleTime is how long to wait after enabling self-test before
// its effect on the output has fully settled (ST's reference procedure
// waits 60ms).
const SelfTestSettleTime = 60 * time.Millisecond

// SetSelfTest enables or disables the self-test excitation (CTRL_REG1's ST
// bit), which applies a known synthetic offset to each axis so the whole
// signal chain can be validated against the SelfTestMin/MaxMilligauss
// limits without depending on the ambient magnetic field. It leaves every
// other setting (Scale, PerformanceMode, DataRate, ...) untouched.
func (d *Device) SetSelfTest(enabled bool) error {
	d.selfTest = enabled
	return d.writeCtrlReg1()
}

// SelfTest returns the self-test setting most recently applied by
// SetSelfTest. Configure always leaves it disabled.
func (d *Device) SelfTest() bool { return d.selfTest }
