package qwiicjoystick

import (
	"errors"
	"testing"
)

type write struct {
	register uint8
	value    uint8
}

// fakeJoystick emulates the Qwiic Joystick firmware register file: the
// first written byte selects a register, further bytes are written
// sequentially, reads return sequential bytes from the selected register.
type fakeJoystick struct {
	regs   [0x0B]byte
	writes []write
}

func newFakeJoystick() *fakeJoystick {
	f := &fakeJoystick{}
	f.regs[REG_ID] = DeviceID
	f.setRawPosition(Center, Center)
	f.regs[REG_BUTTON] = 1 // idle: active low, so 1 means not pressed
	return f
}

// setRawPosition packs a 10-bit ADC value into a register pair exactly as
// the firmware does: left-shifted into a 16-bit word, then split MSB/LSB.
func (f *fakeJoystick) setRawPosition(x, y uint16) {
	f.regs[REG_X_MSB], f.regs[REG_X_LSB] = byte(x<<6>>8), byte(x<<6)
	f.regs[REG_Y_MSB], f.regs[REG_Y_LSB] = byte(y<<6>>8), byte(y<<6)
}

func (f *fakeJoystick) Tx(addr uint16, w, r []byte) error {
	if uint8(addr) != DefaultAddress {
		return errors.New("NACK: wrong I2C address")
	}
	if len(w) == 0 {
		return nil
	}
	p := int(w[0])
	for i, v := range w[1:] {
		f.writes = append(f.writes, write{uint8(p + i), v})
	}
	copy(f.regs[p:], w[1:])
	copy(r, f.regs[p:])
	return nil
}

func configured(t *testing.T) (*fakeJoystick, *Device) {
	t.Helper()
	fake := newFakeJoystick()
	d := New(fake, DefaultAddress)
	if err := d.Configure(); err != nil {
		t.Fatal(err)
	}
	return fake, &d
}

func TestConnected(t *testing.T) {
	fake, d := configured(t)

	if !d.Connected() {
		t.Error("expected device to be connected")
	}

	fake.regs[REG_ID] = 0x5F // a plausible-looking but wrong guess, not the real ID
	if d.Connected() {
		t.Error("expected wrong ID to report not connected")
	}
}

func TestConfigureChecksIDAndClearsLatch(t *testing.T) {
	fake := newFakeJoystick()
	fake.regs[REG_STATUS] = 1

	d := New(fake, DefaultAddress)
	if err := d.Configure(); err != nil {
		t.Fatal(err)
	}
	if fake.regs[REG_STATUS] != 0 {
		t.Errorf("status after Configure = %d, want 0", fake.regs[REG_STATUS])
	}

	fake.regs[REG_ID] = 0x00
	if err := d.Configure(); err != errNotConnected {
		t.Errorf("Configure with wrong ID = %v, want errNotConnected", err)
	}
}

func TestFirmwareVersion(t *testing.T) {
	fake, d := configured(t)
	fake.regs[REG_FIRMWARE_MAJOR] = 2
	fake.regs[REG_FIRMWARE_MINOR] = 6

	major, minor, err := d.FirmwareVersion()
	if err != nil || major != 2 || minor != 6 {
		t.Errorf("FirmwareVersion() = %d, %d, %v; want 2, 6", major, minor, err)
	}
}

// TestRawPositionPacking pins the axis packing against the exact worked
// example in the firmware's own source comment (X_Pot=630 -> MSB 0x9D, LSB
// 0x80), so a byte-order or shift regression is caught even though it
// can't be spotted just by reading the driver code.
func TestRawPositionPacking(t *testing.T) {
	fake, d := configured(t)

	fake.regs[REG_X_MSB], fake.regs[REG_X_LSB] = 0x9D, 0x80
	fake.regs[REG_Y_MSB], fake.regs[REG_Y_LSB] = 0x00, 0x00

	x, y, err := d.RawPosition()
	if err != nil {
		t.Fatal(err)
	}
	if x != 630 {
		t.Errorf("x = %d, want 630", x)
	}
	if y != 0 {
		t.Errorf("y = %d, want 0", y)
	}
}

func TestRawPositionAtRest(t *testing.T) {
	fake, d := configured(t)
	fake.setRawPosition(Center, Center)

	x, y, err := d.RawPosition()
	if err != nil || x != Center || y != Center {
		t.Errorf("RawPosition() at rest = %d, %d, %v; want %d, %d", x, y, err, Center, Center)
	}
}

func TestPositionCenters(t *testing.T) {
	fake, d := configured(t)

	fake.setRawPosition(0, 1023)
	x, y, err := d.Position()
	if err != nil {
		t.Fatal(err)
	}
	if x != -512 {
		t.Errorf("x = %d, want -512 (fully one direction)", x)
	}
	if y != 511 {
		t.Errorf("y = %d, want 511 (fully the other direction)", y)
	}

	fake.setRawPosition(Center, Center)
	x, y, err = d.Position()
	if err != nil || x != 0 || y != 0 {
		t.Errorf("Position() at rest = %d, %d, %v; want 0, 0", x, y, err)
	}
}

// TestButtonPolarity pins IsPressed against the raw register value, since
// the firmware reports the button active-low (0 while held, 1 at rest) -
// the opposite of what most callers expect, and easy to get backwards.
func TestButtonPolarity(t *testing.T) {
	fake, d := configured(t)

	fake.regs[REG_BUTTON] = 1
	if pressed, err := d.IsPressed(); err != nil || pressed {
		t.Errorf("IsPressed() with register=1 = %v, %v; want false (idle)", pressed, err)
	}

	fake.regs[REG_BUTTON] = 0
	if pressed, err := d.IsPressed(); err != nil || !pressed {
		t.Errorf("IsPressed() with register=0 = %v, %v; want true (held down)", pressed, err)
	}
}

func TestHasBeenPressedLatch(t *testing.T) {
	fake, d := configured(t)

	fake.regs[REG_STATUS] = statusHasBeenPressed
	if pressed, err := d.HasBeenPressed(); err != nil || !pressed {
		t.Errorf("HasBeenPressed() = %v, %v; want true", pressed, err)
	}

	if err := d.ClearEventBits(); err != nil {
		t.Fatal(err)
	}
	if fake.regs[REG_STATUS] != 0 {
		t.Errorf("status after ClearEventBits = %d, want 0", fake.regs[REG_STATUS])
	}
	if pressed, _ := d.HasBeenPressed(); pressed {
		t.Error("expected latch to be cleared")
	}
}

func TestSetAddress(t *testing.T) {
	fake, d := configured(t)

	if err := d.SetAddress(0x07); err != errInvalidAddress {
		t.Errorf("SetAddress(0x07) = %v, want errInvalidAddress", err)
	}
	if err := d.SetAddress(0x78); err != errInvalidAddress {
		t.Errorf("SetAddress(0x78) = %v, want errInvalidAddress", err)
	}

	fake.writes = nil
	if err := d.SetAddress(0x60); err != nil {
		t.Fatal(err)
	}
	if d.Address != 0x60 {
		t.Errorf("d.Address = %#02x, want 0x60", d.Address)
	}

	// The firmware only accepts the new address if REG_I2C_LOCK was set
	// immediately beforehand; writing them in the other order silently
	// leaves the address unchanged.
	want := []write{
		{REG_I2C_LOCK, i2cUnlockValue},
		{REG_I2C_ADDRESS, 0x60},
	}
	if len(fake.writes) != len(want) {
		t.Fatalf("writes = %v, want %v", fake.writes, want)
	}
	for i, w := range want {
		if fake.writes[i] != w {
			t.Errorf("write %d = %v, want %v", i, fake.writes[i], w)
		}
	}
}
