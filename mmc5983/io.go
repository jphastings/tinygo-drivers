package mmc5983

func (d *Device) isBitSet(registerAddr uint8, bitMask uint8) (bool, error) {
	val, err := d.readSingleByte(registerAddr)
	if err != nil {
		return false, err
	}
	return (val & bitMask) != 0, nil
}

func (d *Device) readSingleByte(registerAddr uint8) (byte, error) {
	d.wbuf[0] = registerAddr
	if err := d.bus.Tx(d.Address, d.wbuf[:1], d.rbuf[:1]); err != nil {
		return 0, err
	}
	return d.rbuf[0], nil
}

func (d *Device) readMultipleBytes(registerAddr uint8, data []byte) error {
	d.wbuf[0] = registerAddr
	return d.bus.Tx(d.Address, d.wbuf[:1], data)
}
