package mmc5983

import "errors"

// Bandwidth selects the decimation filter bandwidth, which controls the
// duration of each measurement. Its value maps directly onto the BW[1:0]
// bits of Internal Control 1.
type Bandwidth uint8

const (
	Bandwidth100Hz Bandwidth = iota // 8ms measurement time (default)
	Bandwidth200Hz                  // 4ms measurement time
	Bandwidth400Hz                  // 2ms measurement time
	Bandwidth800Hz                  // 0.5ms measurement time
)

var errInvalidBandwidth = errors.New("mmc5983: invalid bandwidth")

// SetBandwidth sets the decimation filter bandwidth.
func (d *Device) SetBandwidth(bw Bandwidth) error {
	if bw > Bandwidth800Hz {
		return errInvalidBandwidth
	}

	// Clear both bandwidth bits in the shadow only, then set the new value
	// and write the whole register in one bus transaction.
	if err := d.operateWithShadow(INT_CTRL_1_REG, opClear, BITS_BW0|BITS_BW1); err != nil {
		return err
	}
	return d.operateWithShadow(INT_CTRL_1_REG, opSet|opWrite, uint8(bw))
}

// Bandwidth returns the decimation filter bandwidth currently configured,
// read from the shadow register.
func (d *Device) Bandwidth() Bandwidth {
	var bw Bandwidth
	if d.isShadowBitSet(INT_CTRL_1_REG, BITS_BW1) {
		bw |= 0b10
	}
	if d.isShadowBitSet(INT_CTRL_1_REG, BITS_BW0) {
		bw |= 0b01
	}
	return bw
}

// measurementTimeout returns how many milliseconds to wait for a
// measurement before giving up.
//
// It is rare, but there are some devices and some circumstances where the
// code can become stuck waiting for the MEAS_M_DONE bit to go high. A
// solution is to time out after 4x the measurement time (defined by
// BW[1:0]), plus 1ms just in case (for 800Hz, whose 0.5ms measurement time
// rounds up to 1ms here).
func (d *Device) measurementTimeout() int {
	measurementTimeMs := 8 >> d.Bandwidth()
	return measurementTimeMs*4 + 1
}
