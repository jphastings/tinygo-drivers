package mpr121

// filteredDataReg, baselineReg, touchThresholdReg and releaseThresholdReg
// compute the register address for a given electrode (0-11), or via
// ElectrodeProximity the combined proximity channel: all four register
// blocks use one address per electrode (two for filtered data and
// thresholds), in ELE0..ELE11, ELEPROX order.
func filteredDataReg(electrode uint8) uint8     { return regFilteredData0 + electrode*2 }
func baselineReg(electrode uint8) uint8         { return regBaseline0 + electrode }
func touchThresholdReg(electrode uint8) uint8   { return regTouchThreshold0 + electrode*2 }
func releaseThresholdReg(electrode uint8) uint8 { return regTouchThreshold0 + electrode*2 + 1 }

func (d *Device) readRegister(reg uint8) (uint8, error) {
	d.wbuf[0] = reg
	if err := d.bus.Tx(uint16(d.Address), d.wbuf[:1], d.rbuf[:1]); err != nil {
		return 0, err
	}
	return d.rbuf[0], nil
}

func (d *Device) readRegisters(reg uint8, data []byte) error {
	d.wbuf[0] = reg
	return d.bus.Tx(uint16(d.Address), d.wbuf[:1], data)
}

func (d *Device) writeRegister(reg, value uint8) error {
	d.wbuf[0] = reg
	d.wbuf[1] = value
	return d.bus.Tx(uint16(d.Address), d.wbuf[:2], nil)
}

// withStopMode runs fn with the device in Stop Mode (ECR = 0x00), which
// the datasheet requires for writes to registers 0x2B-0x7F other than
// the GPIO/LED block (0x73-0x7A) and ECR itself: while Run Mode is
// active, a write to one of them is silently ignored, with no I2C-level
// error to catch the mistake. If Run Mode was active beforehand, it
// resumes afterwards with the same electrode/proximity configuration.
func (d *Device) withStopMode(fn func() error) error {
	ecr, err := d.readRegister(regECR)
	if err != nil {
		return err
	}
	if ecr != 0 {
		if err := d.writeRegister(regECR, 0x00); err != nil {
			return err
		}
	}
	if err := fn(); err != nil {
		return err
	}
	if ecr != 0 {
		return d.writeRegister(regECR, ecr)
	}
	return nil
}
