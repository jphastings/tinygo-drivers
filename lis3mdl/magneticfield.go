package lis3mdl

// ReadMagneticField reads the current magnetic field from the device and
// returns it in mG (milligauss) for all three axes, matching the unit
// convention of this repository's lis2mdl driver. 1 mG = 100 nT
// (nanotesla).
//
// The raw registers are 16-bit two's complement counts; this scales them
// by the sensitivity of the currently configured Scale (Configure or
// SetScale).
func (d *Device) ReadMagneticField() (x, y, z int32, err error) {
	if err := d.readRegisters(regOutXL, d.rbuf[:6]); err != nil {
		return 0, 0, 0, err
	}

	rawX := int16(uint16(d.rbuf[0]) | uint16(d.rbuf[1])<<8)
	rawY := int16(uint16(d.rbuf[2]) | uint16(d.rbuf[3])<<8)
	rawZ := int16(uint16(d.rbuf[4]) | uint16(d.rbuf[5])<<8)

	sensitivity := d.scale.sensitivity()
	x = int32(rawX) * 1000 / sensitivity
	y = int32(rawY) * 1000 / sensitivity
	z = int32(rawZ) * 1000 / sensitivity

	d.lastX, d.lastY, d.lastZ = x, y, z
	return x, y, z, nil
}

// DataReady reports whether a new set of X, Y and Z readings is available
// (STATUS_REG's ZYXDA bit), for polling in ModeSingle or slow ModeContinuous
// data rates.
func (d *Device) DataReady() (bool, error) {
	v, err := d.readRegister(regStatusReg)
	if err != nil {
		return false, err
	}
	return v&(1<<shiftZYXDA) != 0, nil
}
