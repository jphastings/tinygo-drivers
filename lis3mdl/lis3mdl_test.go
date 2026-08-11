package lis3mdl

import (
	"errors"
	"testing"
)

type write struct {
	register uint8
	value    uint8
}

// fakeLIS3MDL emulates a LIS3MDL on the I2C bus, including the two
// behaviors this driver depends on: a multi-byte read only advances
// through consecutive registers when the sub-address's auto-increment bit
// is set (otherwise every byte comes from the same register, per the
// datasheet), and CTRL_REG2's SOFT_RST bit self-clears the instant it is
// written, like the real device's reset pulse.
type fakeLIS3MDL struct {
	regs   [0x30]byte
	writes []write
	reads  []uint8 // sub-address byte (including the auto-increment bit) used by each read
}

func newFakeLIS3MDL() *fakeLIS3MDL {
	f := &fakeLIS3MDL{}
	f.regs[regWhoAmI] = deviceID
	return f
}

func (f *fakeLIS3MDL) Tx(addr uint16, w, r []byte) error {
	if addr != AddressLow {
		return errors.New("wrong I2C address")
	}

	switch {
	case len(w) == 2 && len(r) == 0: // register write
		reg, val := w[0]&^autoIncrement, w[1]
		f.writes = append(f.writes, write{reg, val})
		if reg == regCtrlReg2 && val&(1<<shiftSoftRst) != 0 {
			val &^= 1 << shiftSoftRst
		}
		f.regs[reg] = val
		return nil

	case len(w) == 1 && len(r) > 0: // register read
		reg := w[0]
		f.reads = append(f.reads, reg)
		if reg&autoIncrement != 0 {
			copy(r, f.regs[reg&^autoIncrement:])
		} else {
			for i := range r {
				r[i] = f.regs[reg]
			}
		}
		return nil
	}

	return errors.New("unsupported transaction")
}

func TestConnected(t *testing.T) {
	fake := newFakeLIS3MDL()
	d := New(fake)

	if !d.Connected() {
		t.Error("expected device with correct WHO_AM_I to be connected")
	}

	fake.regs[regWhoAmI] = 0x99
	if d.Connected() {
		t.Error("expected device with wrong WHO_AM_I to not be connected")
	}
}

func TestConfigureAppliesRegisters(t *testing.T) {
	fake := newFakeLIS3MDL()
	d := New(fake)

	err := d.Configure(Config{
		Scale:           Scale8Gauss,
		PerformanceMode: HighPerformance,
		DataRate:        DataRate40Hz,
		OperatingMode:   ModeSingle,
	})
	if err != nil {
		t.Fatal(err)
	}

	// CTRL_REG1: TEMP_EN(1) OM(10) DO(110) FAST_ODR(0) ST(0).
	if fake.regs[regCtrlReg1] != 0xD8 {
		t.Errorf("CTRL_REG1 = %#02x, want %#02x", fake.regs[regCtrlReg1], 0xD8)
	}
	// CTRL_REG2: FS(01).
	if fake.regs[regCtrlReg2] != 0x20 {
		t.Errorf("CTRL_REG2 = %#02x, want %#02x", fake.regs[regCtrlReg2], 0x20)
	}
	// CTRL_REG3: MD(01).
	if fake.regs[regCtrlReg3] != 0x01 {
		t.Errorf("CTRL_REG3 = %#02x, want %#02x", fake.regs[regCtrlReg3], 0x01)
	}
	// CTRL_REG4: OMZ(10) - kept in lockstep with CTRL_REG1's OM, the one
	// easy-to-miss step: writing only CTRL_REG1 leaves Z in low-power mode.
	if fake.regs[regCtrlReg4] != 0x08 {
		t.Errorf("CTRL_REG4 = %#02x, want %#02x (OMZ should match OM)", fake.regs[regCtrlReg4], 0x08)
	}
	// CTRL_REG5: BDU(1).
	if fake.regs[regCtrlReg5] != 0x40 {
		t.Errorf("CTRL_REG5 = %#02x, want %#02x", fake.regs[regCtrlReg5], 0x40)
	}

	if len(fake.writes) == 0 || fake.writes[0] != (write{regCtrlReg2, 1 << shiftSoftRst}) {
		t.Errorf("first write = %v, want a SOFT_RST pulse to CTRL_REG2 before applying Config", fake.writes)
	}
}

func TestConfigureFailsWhenNotConnected(t *testing.T) {
	fake := &fakeLIS3MDL{} // WHO_AM_I left at 0
	d := New(fake)

	if err := d.Configure(Config{}); err != errNotConnected {
		t.Errorf("Configure() = %v, want errNotConnected", err)
	}
	if len(fake.writes) != 0 {
		t.Errorf("Configure without a device touched the bus: %v", fake.writes)
	}
}

func TestConfigureLeavesSelfTestDisabled(t *testing.T) {
	fake := newFakeLIS3MDL()
	d := New(fake)

	if err := d.Configure(Config{}); err != nil {
		t.Fatal(err)
	}
	if err := d.SetSelfTest(true); err != nil {
		t.Fatal(err)
	}

	if err := d.Configure(Config{}); err != nil {
		t.Fatal(err)
	}
	if d.SelfTest() {
		t.Error("SelfTest() = true after Configure, want false")
	}
	if fake.regs[regCtrlReg1]&1 != 0 {
		t.Errorf("CTRL_REG1 ST bit set after Configure: %#02x", fake.regs[regCtrlReg1])
	}
}
