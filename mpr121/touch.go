package mpr121

import "encoding/binary"

// Touched returns the raw 13-channel touch status as a bitmask: bits
// 0-11 are ELE0-ELE11, bit 12 is ElectrodeProximity, and bit 15 is the
// over-current flag (see OverCurrent). IsTouched decodes a single bit
// from the same read.
func (d *Device) Touched() (uint16, error) {
	if err := d.readRegisters(regTouchStatusL, d.rbuf[:2]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(d.rbuf[:2]), nil
}

// IsTouched reports whether a single electrode (0-11), or via
// ElectrodeProximity the combined proximity channel, is currently
// touched.
func (d *Device) IsTouched(electrode uint8) (bool, error) {
	if electrode > ElectrodeProximity {
		return false, errInvalidElectrode
	}
	status, err := d.Touched()
	return status&(1<<electrode) != 0, err
}

// OverCurrent reports whether the REXT current-reference pin has
// tripped the over-current protection. When set, the MPR121 forces
// itself into Stop Mode and discards further ECR writes until the fault
// is cleared; see ClearOverCurrent.
func (d *Device) OverCurrent() (bool, error) {
	status, err := d.Touched()
	return status&overCurrentStatusBit != 0, err
}

// ClearOverCurrent clears a latched over-current fault. The datasheet
// documents that the fault also zeroes ECR's electrode-enable bits, so
// this additionally resumes Run Mode with the electrode/proximity
// configuration Configure last established.
func (d *Device) ClearOverCurrent() error {
	if err := d.writeRegister(regTouchStatusH, overCurrentWriteBit); err != nil {
		return err
	}
	return d.writeRegister(regECR, d.lastECR)
}
