package mmc5983

import (
	"math"
	"time"
)

// The magnetic field outputs are unsigned 18-bit values (verified in the
// datasheet, "Magnetic Sensor Specifications"): a null field reads as
// 131072 counts and the sensitivity is 16384 counts per gauss.
const (
	nullFieldOutput = 131072
	countsPerGauss  = 16384
)

// ReadMagneticField performs a single measurement and returns the magnetic
// field of all three axes in nT (nanotesla). 1 G (gauss) = 100_000 nT.
//
// Please note: to properly correct and calibrate the X, Y and Z channels,
// you need to determine true offsets (zero points) and scale factors
// (gains) for all three channels. Further details can be found at:
// https://thecavepearlproject.org/2015/05/22/calibrating-any-compass-or-accelerometer-for-arduino/
func (d *Device) ReadMagneticField() (x, y, z int32, err error) {
	x, y, z, err = d.measureXYZ()
	if err != nil {
		return 0, 0, 0, err
	}

	return countsToNanoteslas(x), countsToNanoteslas(y), countsToNanoteslas(z), nil
}

func countsToNanoteslas(counts int32) int32 {
	// 100_000 nT/G divided by 16384 counts/G, as the reduced fraction
	// 3125/512 to stay within int32.
	return (counts - nullFieldOutput) * 3125 / 512
}

// ReadCompass reads the current compass heading from the device and returns
// it in millidegrees. When the z axis is pointing straight to Earth and
// the y axis is pointing to North, the heading would be zero.
//
// However, the heading may be off due to electronic compasses would be
// effected by strong magnetic fields and require constant calibration.
func (d *Device) ReadCompass() (int32, error) {
	x, y, _, err := d.ReadMagneticField()
	if err != nil {
		return 0, err
	}

	rh := (math.Atan2(float64(y), float64(x)) * 180) / math.Pi
	if rh < 0 {
		rh = 360 + rh
	}

	return int32(rh * 1000), nil
}

// measureXYZ triggers a measurement and returns the raw, 18-bit unsigned
// magnetic readings from the device.
func (d *Device) measureXYZ() (x, y, z int32, err error) {
	// Set the TM_M bit to start the measurement. This must go through the
	// shadow register: the control registers are write-only, so
	// read-modify-write would corrupt the other bits.
	if err := d.operateWithShadow(INT_CTRL_0_REG, opSet|opWrite, BITS_TM_M); err != nil {
		return 0, 0, 0, err
	}

	// Wait until the measurement is completed or times out.
	done := false
	for timeout := d.measurementTimeout(); timeout > 0; timeout-- {
		// Wait a little so we won't flood the MMC with requests.
		time.Sleep(time.Millisecond)

		if ok, _ := d.isBitSet(STATUS_REG, BITS_MEAS_M_DONE); ok {
			done = true
			break
		}
	}

	// The device clears TM_M itself when the measurement finishes, so clear
	// it in shadow memory only.
	d.operateWithShadow(INT_CTRL_0_REG, opClear, BITS_TM_M)

	if !done {
		return 0, 0, 0, errTimeoutExceeded
	}

	return d.readFieldsXYZ()
}

// readFieldsXYZ assembles the three 18-bit outputs from the seven output
// registers: the top 16 bits of each axis come from its two dedicated
// registers, the lowest 2 bits from the shared XYZ_OUT_2 register.
func (d *Device) readFieldsXYZ() (x, y, z int32, err error) {
	registerValues := d.rbuf[:7]

	if err := d.readMultipleBytes(X_OUT_0_REG, registerValues); err != nil {
		return 0, 0, 0, err
	}

	x = int32(registerValues[0])                    // Xout[17:10]
	x = (x << 8) | int32(registerValues[1])         // Xout[9:2]
	x = (x << 2) | int32(registerValues[6]>>6&0b11) // Xout[1:0]

	y = int32(registerValues[2])                    // Yout[17:10]
	y = (y << 8) | int32(registerValues[3])         // Yout[9:2]
	y = (y << 2) | int32(registerValues[6]>>4&0b11) // Yout[1:0]

	z = int32(registerValues[4])                    // Zout[17:10]
	z = (z << 8) | int32(registerValues[5])         // Zout[9:2]
	z = (z << 2) | int32(registerValues[6]>>2&0b11) // Zout[1:0]

	return x, y, z, nil
}
