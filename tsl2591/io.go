package tsl2591

func (d *Device) readRegister(reg uint8) (uint8, error) {
	d.wbuf[0] = commandNormal | reg
	if err := d.bus.Tx(d.Address, d.wbuf[:1], d.rbuf[:1]); err != nil {
		return 0, err
	}
	return d.rbuf[0], nil
}

// readRegisters performs a single auto-incrementing burst read starting at
// reg. The data registers latch CH0 and CH1's high bytes together when
// CH0's low byte is read, so a burst read is the only way to get a
// self-consistent pair from both channels.
func (d *Device) readRegisters(reg uint8, data []byte) error {
	d.wbuf[0] = commandNormal | reg
	return d.bus.Tx(d.Address, d.wbuf[:1], data)
}

func (d *Device) writeRegister(reg, value uint8) error {
	d.wbuf[0] = commandNormal | reg
	d.wbuf[1] = value
	return d.bus.Tx(d.Address, d.wbuf[:2], nil)
}

// writeCommand issues a special-function command: a single byte with no
// register address and no data.
func (d *Device) writeCommand(cmd uint8) error {
	d.wbuf[0] = cmd
	return d.bus.Tx(d.Address, d.wbuf[:1], nil)
}
