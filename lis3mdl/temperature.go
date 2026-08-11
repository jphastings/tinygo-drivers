package lis3mdl

// ReadTemperature reads the die temperature and returns it in celsius
// milli degrees (°C/1000). The sensor is not factory-calibrated for
// absolute accuracy (the datasheet gives only a slope, 8 LSB/°C, and an
// arbitrary zero point of 0 LSB = 25°C) so this is only useful for relative
// change, not as an ambient temperature reading.
//
// TEMP_EN is always enabled by Configure, so this can be called any time
// after Configure without a separate opt-in.
func (d *Device) ReadTemperature() (int32, error) {
	if err := d.readRegisters(regTempOutL, d.rbuf[:2]); err != nil {
		return 0, err
	}

	raw := int16(uint16(d.rbuf[0]) | uint16(d.rbuf[1])<<8)
	milliCelsius := int32(raw)*125 + 25000

	d.lastMilliCelsius = milliCelsius
	return milliCelsius, nil
}
