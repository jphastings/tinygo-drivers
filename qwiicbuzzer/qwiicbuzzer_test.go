package qwiicbuzzer

import (
	"errors"
	"testing"
	"time"
)

// fakeBuzzer emulates the Qwiic Buzzer firmware register file: the first
// written byte selects a register, further bytes are written sequentially,
// reads return sequential bytes from the selected register.
type fakeBuzzer struct {
	regs [0x0B]byte
}

func newFakeBuzzer() *fakeBuzzer {
	f := &fakeBuzzer{}
	f.regs[REG_ID] = DeviceID
	return f
}

func (f *fakeBuzzer) Tx(addr uint16, w, r []byte) error {
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

func configured(t *testing.T) (*fakeBuzzer, *Device) {
	t.Helper()
	fake := newFakeBuzzer()
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

func TestConfigureSilencesAndChecksID(t *testing.T) {
	fake := newFakeBuzzer()
	fake.regs[REG_ACTIVE] = 1

	d := New(fake, DefaultAddress)
	if err := d.Configure(); err != nil {
		t.Fatal(err)
	}
	if fake.regs[REG_ACTIVE] != 0 {
		t.Errorf("active after Configure = %d, want 0", fake.regs[REG_ACTIVE])
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

func TestSetToneByteOrder(t *testing.T) {
	fake, d := configured(t)

	if err := d.SetTone(0x1234, VolumeMid, 0x0203*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	// Frequency and duration are big-endian (MSB at the lower register
	// address), unlike qwiicbutton's little-endian 16-bit registers.
	if fake.regs[REG_TONE_FREQUENCY_MSB] != 0x12 || fake.regs[REG_TONE_FREQUENCY_LSB] != 0x34 {
		t.Errorf("frequency registers = %#02x %#02x, want 0x12 0x34",
			fake.regs[REG_TONE_FREQUENCY_MSB], fake.regs[REG_TONE_FREQUENCY_LSB])
	}
	if fake.regs[REG_VOLUME] != uint8(VolumeMid) {
		t.Errorf("volume register = %d, want %d", fake.regs[REG_VOLUME], VolumeMid)
	}
	if fake.regs[REG_DURATION_MSB] != 0x02 || fake.regs[REG_DURATION_LSB] != 0x03 {
		t.Errorf("duration registers = %#02x %#02x, want 0x02 0x03",
			fake.regs[REG_DURATION_MSB], fake.regs[REG_DURATION_LSB])
	}

	// REG_ACTIVE must be untouched: SetTone stages a tone without playing it.
	if fake.regs[REG_ACTIVE] != 0 {
		t.Errorf("active register = %d, want 0 (SetTone must not start the buzzer)", fake.regs[REG_ACTIVE])
	}

	freq, err := d.Frequency()
	if err != nil || freq != 0x1234 {
		t.Errorf("Frequency() = %#04x, %v; want 0x1234", freq, err)
	}
	dur, err := d.DurationSetting()
	if err != nil || dur != 0x0203*time.Millisecond {
		t.Errorf("DurationSetting() = %v, %v; want %v", dur, err, 0x0203*time.Millisecond)
	}
}

func TestSetToneInvalidVolume(t *testing.T) {
	fake, d := configured(t)

	if err := d.SetTone(440, Volume(5), time.Second); err != errInvalidVolume {
		t.Errorf("SetTone with volume 5 = %v, want errInvalidVolume", err)
	}
	// Nothing should have reached the wire.
	if fake.regs[REG_TONE_FREQUENCY_MSB] != 0 || fake.regs[REG_TONE_FREQUENCY_LSB] != 0 {
		t.Error("expected rejected SetTone call to leave frequency registers untouched")
	}
}

func TestPlayStartsTheBuzzer(t *testing.T) {
	fake, d := configured(t)

	if err := d.Play(440, VolumeMax, 250*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if fake.regs[REG_ACTIVE] != 1 {
		t.Errorf("active register after Play = %d, want 1", fake.regs[REG_ACTIVE])
	}
	if active, err := d.Active(); err != nil || !active {
		t.Errorf("Active() = %v, %v; want true", active, err)
	}

	if err := d.Stop(); err != nil {
		t.Fatal(err)
	}
	if active, err := d.Active(); err != nil || active {
		t.Errorf("Active() after Stop = %v, %v; want false", active, err)
	}
}

func TestZeroDurationPlaysForever(t *testing.T) {
	fake, d := configured(t)

	// The zero value of time.Duration must reach the wire as literal 0,
	// which the firmware interprets as "play until told to stop" rather
	// than "don't play".
	if err := d.SetTone(ResonantFrequency, VolumeMax, 0); err != nil {
		t.Fatal(err)
	}
	if fake.regs[REG_DURATION_MSB] != 0 || fake.regs[REG_DURATION_LSB] != 0 {
		t.Errorf("duration registers = %#02x %#02x, want 0 0",
			fake.regs[REG_DURATION_MSB], fake.regs[REG_DURATION_LSB])
	}
}

func TestShortDurationDoesNotRoundToForever(t *testing.T) {
	_, d := configured(t)

	if err := d.SetTone(ResonantFrequency, VolumeMax, 500*time.Microsecond); err != nil {
		t.Fatal(err)
	}
	dur, err := d.DurationSetting()
	if err != nil {
		t.Fatal(err)
	}
	if dur != time.Millisecond {
		t.Errorf("DurationSetting() = %v, want 1ms (rounded up, not down to 0/forever)", dur)
	}
}

func TestFrequencyZeroIsARest(t *testing.T) {
	fake, d := configured(t)

	// Frequency 0 is a deliberate "rest" the firmware plays as silence, not
	// an error and not a 0Hz tone.
	if err := d.Play(0, VolumeMax, 0); err != nil {
		t.Fatal(err)
	}
	if fake.regs[REG_TONE_FREQUENCY_MSB] != 0 || fake.regs[REG_TONE_FREQUENCY_LSB] != 0 {
		t.Error("expected frequency registers to be 0")
	}
	if fake.regs[REG_ACTIVE] != 1 {
		t.Error("expected Play(0, ...) to still set the active register")
	}
}

func TestFrequencyFromPeriod(t *testing.T) {
	cases := []struct {
		periodNanoseconds uint64
		wantHz            uint16
	}{
		{0, 0},         // tone's "no sound" sentinel
		{2272727, 440}, // period of a 440Hz wave, concert pitch A
		{1, 0xFFFF},    // absurdly short period: clamp rather than overflow
	}
	for _, c := range cases {
		if got := FrequencyFromPeriod(c.periodNanoseconds); got != c.wantHz {
			t.Errorf("FrequencyFromPeriod(%d) = %d, want %d", c.periodNanoseconds, got, c.wantHz)
		}
	}
}

func TestPlayPeriod(t *testing.T) {
	fake, d := configured(t)

	if err := d.PlayPeriod(2272727, VolumeLow, time.Second); err != nil {
		t.Fatal(err)
	}
	freq, err := d.Frequency()
	if err != nil || freq != 440 {
		t.Errorf("Frequency() after PlayPeriod = %d, %v; want 440", freq, err)
	}
	if fake.regs[REG_ACTIVE] != 1 {
		t.Error("expected PlayPeriod to start the buzzer")
	}
}

func TestSaveSettings(t *testing.T) {
	fake, d := configured(t)

	if err := d.SaveSettings(); err != nil {
		t.Fatal(err)
	}
	if fake.regs[REG_SAVE_SETTINGS] != 1 {
		t.Errorf("save-settings register = %d, want 1", fake.regs[REG_SAVE_SETTINGS])
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

	if err := d.SetAddress(0x60); err != nil {
		t.Fatal(err)
	}
	if fake.regs[REG_I2C_ADDRESS] != 0x60 {
		t.Errorf("i2c address register = %#02x, want 0x60", fake.regs[REG_I2C_ADDRESS])
	}
	if d.Address != 0x60 {
		t.Errorf("d.Address = %#02x, want 0x60", d.Address)
	}
}
