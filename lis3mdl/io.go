package lis3mdl

func (d *Device) readRegister(reg uint8) (uint8, error) {
	d.wbuf[0] = reg
	if err := d.bus.Tx(d.Address, d.wbuf[:1], d.rbuf[:1]); err != nil {
		return 0, err
	}
	return d.rbuf[0], nil
}

// readRegisters performs a multi-byte read starting at reg. It sets the
// auto-increment bit (see registers.go) so consecutive bytes come from
// consecutive registers instead of all being read from reg.
func (d *Device) readRegisters(reg uint8, data []byte) error {
	d.wbuf[0] = reg | autoIncrement
	return d.bus.Tx(d.Address, d.wbuf[:1], data)
}

func (d *Device) writeRegister(reg, value uint8) error {
	d.wbuf[0], d.wbuf[1] = reg, value
	return d.bus.Tx(d.Address, d.wbuf[:2], nil)
}
