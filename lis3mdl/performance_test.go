package lis3mdl

import "testing"

// TestSetPerformanceModeSetsZTandem is the behavioral counterpart to the
// datasheet fact that OM (CTRL_REG1, X/Y) and OMZ (CTRL_REG4, Z) are
// separate fields: it pins that SetPerformanceMode writes both, and that
// doing so does not disturb CTRL_REG1's other, independently configured
// bits (DataRate here).
func TestSetPerformanceModeSetsZTandem(t *testing.T) {
	fake := newFakeLIS3MDL()
	d := New(fake)
	if err := d.Configure(Config{DataRate: DataRate40Hz}); err != nil {
		t.Fatal(err)
	}

	if err := d.SetPerformanceMode(UltraHighPerformance); err != nil {
		t.Fatal(err)
	}

	// CTRL_REG1: TEMP_EN(1) OM(11) DO(110, unchanged from Configure) ST(0).
	if fake.regs[regCtrlReg1] != 0xF8 {
		t.Errorf("CTRL_REG1 = %#02x, want %#02x", fake.regs[regCtrlReg1], 0xF8)
	}
	// CTRL_REG4: OMZ(11), matching OM above.
	if fake.regs[regCtrlReg4] != 0x0C {
		t.Errorf("CTRL_REG4 = %#02x, want %#02x", fake.regs[regCtrlReg4], 0x0C)
	}
	if d.PerformanceMode() != UltraHighPerformance {
		t.Errorf("PerformanceMode() = %v, want UltraHighPerformance", d.PerformanceMode())
	}
}

func TestSetDataRateAndFastODR(t *testing.T) {
	fake := newFakeLIS3MDL()
	d := New(fake)
	if err := d.Configure(Config{}); err != nil {
		t.Fatal(err)
	}

	if err := d.SetDataRate(DataRate80Hz); err != nil {
		t.Fatal(err)
	}
	if fake.regs[regCtrlReg1] != 0x9C { // TEMP_EN(1) OM(00) DO(111) FAST_ODR(0)
		t.Errorf("CTRL_REG1 after SetDataRate = %#02x, want %#02x", fake.regs[regCtrlReg1], 0x9C)
	}

	if err := d.SetFastODR(true); err != nil {
		t.Fatal(err)
	}
	if fake.regs[regCtrlReg1] != 0x9E { // DO bits preserved, FAST_ODR now set
		t.Errorf("CTRL_REG1 after SetFastODR = %#02x, want %#02x", fake.regs[regCtrlReg1], 0x9E)
	}
	if !d.FastODR() {
		t.Error("FastODR() = false, want true")
	}
}

func TestSetOperatingMode(t *testing.T) {
	fake := newFakeLIS3MDL()
	d := New(fake)
	if err := d.Configure(Config{}); err != nil {
		t.Fatal(err)
	}

	if err := d.SetOperatingMode(ModePowerDown); err != nil {
		t.Fatal(err)
	}
	if fake.regs[regCtrlReg3] != 0x02 {
		t.Errorf("CTRL_REG3 = %#02x, want %#02x", fake.regs[regCtrlReg3], 0x02)
	}
	if d.OperatingMode() != ModePowerDown {
		t.Errorf("OperatingMode() = %v, want ModePowerDown", d.OperatingMode())
	}
}

func TestSetScale(t *testing.T) {
	fake := newFakeLIS3MDL()
	d := New(fake)
	if err := d.Configure(Config{}); err != nil {
		t.Fatal(err)
	}

	if err := d.SetScale(Scale16Gauss); err != nil {
		t.Fatal(err)
	}
	if fake.regs[regCtrlReg2] != 0x60 {
		t.Errorf("CTRL_REG2 = %#02x, want %#02x", fake.regs[regCtrlReg2], 0x60)
	}
	if d.Scale() != Scale16Gauss {
		t.Errorf("Scale() = %v, want Scale16Gauss", d.Scale())
	}
}
