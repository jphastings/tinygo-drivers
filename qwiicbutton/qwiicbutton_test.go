package qwiicbutton

import (
	"errors"
	"testing"
)

// fakeButton emulates the Qwiic Button firmware register file: the first
// written byte selects a register, further bytes are written sequentially,
// reads return sequential bytes from the selected register.
type fakeButton struct {
	regs [0x20]byte
}

func newFakeButton() *fakeButton {
	f := &fakeButton{}
	f.regs[REG_ID] = DeviceID
	return f
}

func (f *fakeButton) Tx(addr uint16, w, r []byte) error {
	if uint8(addr) != DefaultAddress {
		return errors.New("NACK: wrong I2C address")
	}
	if len(w) == 0 {
		return nil
	}
	p := int(w[0])
	copy(f.regs[p:], w[1:])
	copy(r, f.regs[p:])
	return nil
}

func configured(t *testing.T) (*fakeButton, *Device) {
	t.Helper()
	fake := newFakeButton()
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

	fake.regs[REG_ID] = 0x42
	if d.Connected() {
		t.Error("expected wrong device ID to report not connected")
	}
}

func TestIsPressed(t *testing.T) {
	fake, d := configured(t)

	fake.regs[REG_BUTTON_STATUS] = statusIsPressed
	if pressed, err := d.IsPressed(); err != nil || !pressed {
		t.Errorf("IsPressed() = %v, %v; want true", pressed, err)
	}

	// Latched event bits set, but button not held down
	fake.regs[REG_BUTTON_STATUS] = statusEventAvailable | statusHasBeenClicked
	if pressed, err := d.IsPressed(); err != nil || pressed {
		t.Errorf("IsPressed() = %v, %v; want false", pressed, err)
	}
}

func TestClickedLatch(t *testing.T) {
	fake, d := configured(t)

	// All three status bits latched, plus reserved upper bits
	fake.regs[REG_BUTTON_STATUS] = 0xA8 | statusEventAvailable | statusHasBeenClicked | statusIsPressed

	if clicked, err := d.HasBeenClicked(); err != nil || !clicked {
		t.Errorf("HasBeenClicked() = %v, %v; want true", clicked, err)
	}
	if available, err := d.EventAvailable(); err != nil || !available {
		t.Errorf("EventAvailable() = %v, %v; want true", available, err)
	}

	if err := d.ClearEventBits(); err != nil {
		t.Fatal(err)
	}
	// Read-modify-write: event bits cleared, reserved bits untouched
	if fake.regs[REG_BUTTON_STATUS] != 0xA8 {
		t.Errorf("status after ClearEventBits = %#02x, want 0xA8", fake.regs[REG_BUTTON_STATUS])
	}
	if clicked, _ := d.HasBeenClicked(); clicked {
		t.Error("expected clicked latch to be cleared")
	}
}

func TestDebounceTime(t *testing.T) {
	fake, d := configured(t)

	if err := d.SetDebounceTime(0x0203); err != nil {
		t.Fatal(err)
	}
	// 16-bit little endian
	if fake.regs[REG_BUTTON_DEBOUNCE_TIME] != 0x03 || fake.regs[REG_BUTTON_DEBOUNCE_TIME+1] != 0x02 {
		t.Errorf("debounce registers = %#02x %#02x, want 0x03 0x02",
			fake.regs[REG_BUTTON_DEBOUNCE_TIME], fake.regs[REG_BUTTON_DEBOUNCE_TIME+1])
	}

	if ms, err := d.DebounceTime(); err != nil || ms != 0x0203 {
		t.Errorf("DebounceTime() = %d, %v; want %d", ms, err, 0x0203)
	}
}

func TestLEDConfig(t *testing.T) {
	fake, d := configured(t)

	if err := d.LEDConfig(128, 0x1234, 0x0567, 2); err != nil {
		t.Fatal(err)
	}
	if fake.regs[REG_LED_BRIGHTNESS] != 128 {
		t.Errorf("brightness = %d, want 128", fake.regs[REG_LED_BRIGHTNESS])
	}
	if fake.regs[REG_LED_PULSE_GRANULARITY] != 2 {
		t.Errorf("granularity = %d, want 2", fake.regs[REG_LED_PULSE_GRANULARITY])
	}
	// 16-bit little endian
	if fake.regs[REG_LED_PULSE_CYCLE_TIME] != 0x34 || fake.regs[REG_LED_PULSE_CYCLE_TIME+1] != 0x12 {
		t.Errorf("cycle time registers = %#02x %#02x, want 0x34 0x12",
			fake.regs[REG_LED_PULSE_CYCLE_TIME], fake.regs[REG_LED_PULSE_CYCLE_TIME+1])
	}
	if fake.regs[REG_LED_PULSE_OFF_TIME] != 0x67 || fake.regs[REG_LED_PULSE_OFF_TIME+1] != 0x05 {
		t.Errorf("off time registers = %#02x %#02x, want 0x67 0x05",
			fake.regs[REG_LED_PULSE_OFF_TIME], fake.regs[REG_LED_PULSE_OFF_TIME+1])
	}

	if err := d.LEDOn(255); err != nil {
		t.Fatal(err)
	}
	if fake.regs[REG_LED_BRIGHTNESS] != 255 || fake.regs[REG_LED_PULSE_CYCLE_TIME] != 0 {
		t.Error("expected LEDOn to set full brightness with pulsing disabled")
	}

	if err := d.LEDOff(); err != nil {
		t.Fatal(err)
	}
	if fake.regs[REG_LED_BRIGHTNESS] != 0 {
		t.Errorf("brightness after LEDOff = %d, want 0", fake.regs[REG_LED_BRIGHTNESS])
	}
}

func TestLEDConfigZeroGranularity(t *testing.T) {
	fake, d := configured(t)

	// The firmware divides by granularity to size its pulse steps, so a
	// zero granularity must not reach the wire.
	if err := d.LEDConfig(128, 1000, 500, 0); err != nil {
		t.Fatal(err)
	}
	if fake.regs[REG_LED_PULSE_GRANULARITY] != 1 {
		t.Errorf("granularity = %d, want 1", fake.regs[REG_LED_PULSE_GRANULARITY])
	}
}

func TestConfigure(t *testing.T) {
	fake := newFakeButton()
	fake.regs[REG_BUTTON_STATUS] = statusEventAvailable | statusHasBeenClicked
	fake.regs[REG_LED_BRIGHTNESS] = 200

	d := New(fake, DefaultAddress)
	if err := d.Configure(); err != nil {
		t.Fatal(err)
	}
	if fake.regs[REG_BUTTON_STATUS] != 0 {
		t.Errorf("status after Configure = %#02x, want 0", fake.regs[REG_BUTTON_STATUS])
	}
	if fake.regs[REG_LED_BRIGHTNESS] != 0 {
		t.Errorf("brightness after Configure = %d, want 0", fake.regs[REG_LED_BRIGHTNESS])
	}

	fake.regs[REG_ID] = 0x42
	if err := d.Configure(); err != errNotConnected {
		t.Errorf("Configure with wrong ID = %v, want errNotConnected", err)
	}
}

func TestFirmwareVersion(t *testing.T) {
	fake, d := configured(t)
	fake.regs[REG_FIRMWARE_MAJOR] = 1
	fake.regs[REG_FIRMWARE_MINOR] = 2

	major, minor, err := d.FirmwareVersion()
	if err != nil || major != 1 || minor != 2 {
		t.Errorf("FirmwareVersion() = %d, %d, %v; want 1, 2", major, minor, err)
	}
}
