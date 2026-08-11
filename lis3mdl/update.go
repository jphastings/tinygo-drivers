package lis3mdl

import "tinygo.org/x/drivers"

// Update performs the requested readings and stores them for the
// accessors below. Only drivers.MagneticField and drivers.Temperature are
// supported; bits for any other measurement are ignored and never touch
// the bus.
func (d *Device) Update(which drivers.Measurement) error {
	if which&drivers.MagneticField != 0 {
		if _, _, _, err := d.ReadMagneticField(); err != nil {
			return err
		}
	}
	if which&drivers.Temperature != 0 {
		if _, err := d.ReadTemperature(); err != nil {
			return err
		}
	}
	return nil
}

// MagneticField returns the reading from the most recent successful call
// to ReadMagneticField or Update, in mG (milligauss).
func (d *Device) MagneticField() (x, y, z int32) { return d.lastX, d.lastY, d.lastZ }

// Temperature returns the reading from the most recent successful call to
// ReadTemperature or Update, in celsius milli degrees (°C/1000).
func (d *Device) Temperature() int32 { return d.lastMilliCelsius }
