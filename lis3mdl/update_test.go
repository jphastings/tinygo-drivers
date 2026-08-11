package lis3mdl

import (
	"testing"

	"tinygo.org/x/drivers"
)

func TestUpdate(t *testing.T) {
	fake := newFakeLIS3MDL()
	d := New(fake)
	if err := d.Configure(Config{}); err != nil {
		t.Fatal(err)
	}
	setRawAxes(fake, 6842, 0, 0) // +1G on X
	fake.regs[regTempOutL] = 0   // raw 0 -> 25.000C
	fake.regs[regTempOutL+1] = 0

	fake.reads = nil
	if err := d.Update(drivers.Temperature); err != nil {
		t.Fatal(err)
	}
	if containsReg(fake.reads, regOutXL|autoIncrement) {
		t.Error("Update(Temperature) read the magnetic field registers")
	}
	if !containsReg(fake.reads, regTempOutL|autoIncrement) {
		t.Error("Update(Temperature) did not read the temperature registers")
	}
	if d.Temperature() != 25000 {
		t.Errorf("Temperature() = %d, want 25000", d.Temperature())
	}

	fake.reads = nil
	if err := d.Update(drivers.MagneticField); err != nil {
		t.Fatal(err)
	}
	if containsReg(fake.reads, regTempOutL|autoIncrement) {
		t.Error("Update(MagneticField) read the temperature registers")
	}
	if !containsReg(fake.reads, regOutXL|autoIncrement) {
		t.Error("Update(MagneticField) did not read the magnetic field registers")
	}
	x, y, z := d.MagneticField()
	if x != 1000 || y != 0 || z != 0 {
		t.Errorf("MagneticField() = (%d, %d, %d), want (1000, 0, 0)", x, y, z)
	}

	// Measurements this driver doesn't support must not touch the bus.
	fake.reads = nil
	fake.writes = nil
	if err := d.Update(drivers.Pressure); err != nil {
		t.Errorf("Update(Pressure) = %v, want nil", err)
	}
	if len(fake.reads) != 0 || len(fake.writes) != 0 {
		t.Errorf("Update(Pressure) touched the bus: reads=%v writes=%v", fake.reads, fake.writes)
	}
}

func containsReg(regs []uint8, want uint8) bool {
	for _, r := range regs {
		if r == want {
			return true
		}
	}
	return false
}
