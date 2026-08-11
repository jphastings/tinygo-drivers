package ltr329

func (d *Device) readRegister(reg uint8) (uint8, error) {
	d.wbuf[0] = reg
	if err := d.bus.Tx(d.Address, d.wbuf[:1], d.rbuf[:1]); err != nil {
		return 0, err
	}
	return d.rbuf[0], nil
}

// readRegisters performs a single auto-incrementing burst read starting at
// reg. The datasheet requires this for the ALS_DATA registers specifically:
// all four are locked for the duration of one I2C read transaction, so a
// burst is the only way to get a self-consistent CH0/CH1 pair - see
// ReadChannels.
func (d *Device) readRegisters(reg uint8, data []byte) error {
	d.wbuf[0] = reg
	return d.bus.Tx(d.Address, d.wbuf[:1], data)
}

func (d *Device) writeRegister(reg, value uint8) error {
	d.wbuf[0] = reg
	d.wbuf[1] = value
	return d.bus.Tx(d.Address, d.wbuf[:2], nil)
}
