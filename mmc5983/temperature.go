package mmc5983

import "time"

// ReadTemperature performs a temperature conversion and returns the result
// in celsius milli degrees (°C/1000). The sensor covers -75°C to +125°C in
// 255 steps of about 0.8°C.
func (d *Device) ReadTemperature() (int32, error) {
	// Set the TM_T bit to start the temperature conversion. This must go
	// through the shadow register: the control registers are write-only, so
	// read-modify-write would corrupt the other bits.
	if err := d.operateWithShadow(INT_CTRL_0_REG, opSet|opWrite, BITS_TM_T); err != nil {
		return 0, err
	}

	// Wait until the conversion is completed. It is rare, but there are some
	// devices and some circumstances where the code can become stuck waiting
	// for MEAS_T_DONE to go high; time out after 5ms.
	for retries := 5; retries > 0; retries-- {
		// Wait a little so we won't flood the MMC with requests.
		time.Sleep(time.Millisecond)

		if ok, _ := d.isBitSet(STATUS_REG, BITS_MEAS_T_DONE); ok {
			break
		}
	}

	// The device clears TM_T itself when the conversion finishes, so clear
	// it in shadow memory only.
	d.operateWithShadow(INT_CTRL_0_REG, opClear, BITS_TM_T)

	// Read the result even if a timeout occurred: old data beats no data.
	result, err := d.readSingleByte(T_OUT_REG)
	if err != nil {
		return 0, err
	}

	// 0x00 stands for -75°C, full scale 0xFF for +125°C.
	return -75000 + int32(result)*200000/255, nil
}
