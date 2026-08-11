package lis3mdl

import "testing"

func TestSetSelfTest(t *testing.T) {
	fake := newFakeLIS3MDL()
	d := New(fake)
	if err := d.Configure(Config{DataRate: DataRate80Hz}); err != nil {
		t.Fatal(err)
	}

	if err := d.SetSelfTest(true); err != nil {
		t.Fatal(err)
	}
	// CTRL_REG1: TEMP_EN(1) OM(00) DO(111, preserved from Configure) ST(1).
	if fake.regs[regCtrlReg1] != 0x9D {
		t.Errorf("CTRL_REG1 with self-test on = %#02x, want %#02x", fake.regs[regCtrlReg1], 0x9D)
	}
	if !d.SelfTest() {
		t.Error("SelfTest() = false after SetSelfTest(true)")
	}

	if err := d.SetSelfTest(false); err != nil {
		t.Fatal(err)
	}
	if fake.regs[regCtrlReg1] != 0x9C {
		t.Errorf("CTRL_REG1 with self-test off = %#02x, want %#02x", fake.regs[regCtrlReg1], 0x9C)
	}
	if d.SelfTest() {
		t.Error("SelfTest() = true after SetSelfTest(false)")
	}
}
