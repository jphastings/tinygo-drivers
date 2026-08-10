package lc709203

// readWord issues a word-register read: a write of the command byte,
// followed by a repeated-start read of two data bytes and a CRC-8 byte.
// The CRC covers the write-phase address byte, the command, the
// read-phase address byte and both data bytes - see crc.go - so it is
// recomputed here from those exact bytes and checked against what the
// device sent back.
func (d *Device) readWord(cmd uint8) (uint16, error) {
	d.wbuf[0] = cmd
	if err := d.bus.Tx(d.Address, d.wbuf[:1], d.rbuf[:3]); err != nil {
		return 0, err
	}

	writeAddr := uint8(d.Address << 1)
	pec := [5]byte{writeAddr, cmd, writeAddr | 1, d.rbuf[0], d.rbuf[1]}
	if crc8(pec[:]) != d.rbuf[2] {
		return 0, errCRCMismatch
	}

	return uint16(d.rbuf[1])<<8 | uint16(d.rbuf[0]), nil
}

// writeWord issues a word-register write: command byte, data low byte,
// data high byte, then a CRC-8 byte computed over the write-phase address
// byte, the command and both data bytes.
func (d *Device) writeWord(cmd uint8, value uint16) error {
	writeAddr := uint8(d.Address << 1)
	d.wbuf[0] = cmd
	d.wbuf[1] = byte(value)
	d.wbuf[2] = byte(value >> 8)
	pec := [4]byte{writeAddr, d.wbuf[0], d.wbuf[1], d.wbuf[2]}
	d.wbuf[3] = crc8(pec[:])

	return d.bus.Tx(d.Address, d.wbuf[:4], nil)
}
