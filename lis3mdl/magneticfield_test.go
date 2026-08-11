package lis3mdl

import "testing"

func setRawAxes(f *fakeLIS3MDL, x, y, z int16) {
	f.regs[regOutXL] = byte(x)
	f.regs[regOutXL+1] = byte(x >> 8)
	f.regs[regOutXL+2] = byte(y)
	f.regs[regOutXL+3] = byte(y >> 8)
	f.regs[regOutXL+4] = byte(z)
	f.regs[regOutXL+5] = byte(z >> 8)
}

// TestReadMagneticFieldUsesAutoIncrement pins down the driver's use of the
// sub-address auto-increment bit for the OUT_X_L..OUT_Z_H burst read. The
// fake only returns six distinct, correct values when that bit is present
// (see fakeLIS3MDL.Tx); dropping it from readRegisters would make this
// test fail because every axis would read back OUT_X_L's value instead.
func TestReadMagneticFieldUsesAutoIncrement(t *testing.T) {
	fake := newFakeLIS3MDL()
	d := New(fake)
	if err := d.Configure(Config{}); err != nil { // Scale4Gauss, 6842 LSB/gauss
		t.Fatal(err)
	}

	setRawAxes(fake, 6842, -6842, 3421) // +1G, -1G, +0.5G

	x, y, z, err := d.ReadMagneticField()
	if err != nil {
		t.Fatal(err)
	}
	if x != 1000 || y != -1000 || z != 500 {
		t.Errorf("ReadMagneticField() = (%d, %d, %d) mG, want (1000, -1000, 500)", x, y, z)
	}

	last := fake.reads[len(fake.reads)-1]
	if last != regOutXL|autoIncrement {
		t.Errorf("sub-address used for burst read = %#02x, want %#02x (OUT_X_L | auto-increment)", last, regOutXL|autoIncrement)
	}
}

func TestReadMagneticFieldAcrossScales(t *testing.T) {
	cases := []struct {
		scale       FullScale
		sensitivity int16
	}{
		{Scale4Gauss, 6842},
		{Scale8Gauss, 3421},
		{Scale12Gauss, 2281},
		{Scale16Gauss, 1711},
	}

	for _, c := range cases {
		fake := newFakeLIS3MDL()
		d := New(fake)
		if err := d.Configure(Config{Scale: c.scale}); err != nil {
			t.Fatal(err)
		}

		// A raw count equal to the scale's own sensitivity is exactly 1
		// gauss on every scale, so every case should agree on 1000 mG.
		setRawAxes(fake, c.sensitivity, 0, 0)

		x, _, _, err := d.ReadMagneticField()
		if err != nil {
			t.Fatal(err)
		}
		if x != 1000 {
			t.Errorf("scale %v: x = %d mG, want 1000", c.scale, x)
		}
	}
}

func TestDataReady(t *testing.T) {
	fake := newFakeLIS3MDL()
	d := New(fake)

	fake.regs[regStatusReg] = 0
	if ready, err := d.DataReady(); err != nil || ready {
		t.Errorf("DataReady() = (%v, %v), want (false, nil)", ready, err)
	}

	fake.regs[regStatusReg] = 1 << shiftZYXDA
	if ready, err := d.DataReady(); err != nil || !ready {
		t.Errorf("DataReady() = (%v, %v), want (true, nil)", ready, err)
	}
}
