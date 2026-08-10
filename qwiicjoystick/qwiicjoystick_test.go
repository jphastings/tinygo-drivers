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
	if err := d.Configure(Config{}); err != nil {
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
	if err := d.Configure(Config{}); err != nil {
		t.Fatal(err)
	}
	if fake.regs[REG_STATUS] != 0 {
		t.Errorf("status after Configure = %d, want 0", fake.regs[REG_STATUS])
	}

	fake.regs[REG_ID] = 0x00
	if err := d.Configure(Config{}); err != errNotConnected {
		t.Errorf("Configure with wrong ID = %v, want errNotConnected", err)
	}
}

func TestConfigureAppliesRotationAndMirror(t *testing.T) {
	fake := newFakeJoystick()
	d := New(fake, DefaultAddress)
	if err := d.Configure(Config{Rotation: Rotation180, MirrorHorizontal: true}); err != nil {
		t.Fatal(err)
	}

	// Raw (712, 512): normalized (0, +200), Rotation180 -> (0, -200),
	// MirrorHorizontal -> (0, -200) (x is already 0).
	fake.setRawPosition(712, 512)
	x, y, err := d.Position()
	if err != nil {
		t.Fatal(err)
	}
	if x != 0 || y != -200 {
		t.Errorf("Position() = %d, %d; want 0, -200", x, y)
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

func TestPositionAtRest(t *testing.T) {
	fake, d := configured(t)
	fake.setRawPosition(Center, Center)

	x, y, err := d.Position()
	if err != nil || x != 0 || y != 0 {
		t.Errorf("Position() at rest = %d, %d, %v; want 0, 0", x, y, err)
	}
}

// TestPositionNormalizesAxes pins Position's axis correction against the
// hardware measurement documented in the package doc: the firmware's raw X
// increases on a physical push up, and raw Y increases on a physical push
// right - the opposite of what their names suggest. Position must swap
// them so X always means left/right and Y always means up/down.
func TestPositionNormalizesAxes(t *testing.T) {
	fake, d := configured(t)

	// Physical push up: raw X at max, raw Y centered.
	fake.setRawPosition(1023, Center)
	x, y, err := d.Position()
	if err != nil {
		t.Fatal(err)
	}
	if x != 0 || y != 511 {
		t.Errorf("Position() for push up = %d, %d; want 0, 511", x, y)
	}

	// Physical push right: raw Y at max, raw X centered.
	fake.setRawPosition(Center, 1023)
	x, y, err = d.Position()
	if err != nil {
		t.Fatal(err)
	}
	if x != 511 || y != 0 {
		t.Errorf("Position() for push right = %d, %d; want 511, 0", x, y)
	}
}

// TestPositionRotation pins each Rotation against the same fixed raw
// reading (raw X=1023, Y=Center, i.e. a physical push up, normalizing to
// (0, 511)). The expected values are written out explicitly rather than
// derived from the rotation formula under test, so a transposed or
// sign-flipped rotation is caught rather than silently matching.
func TestPositionRotation(t *testing.T) {
	cases := []struct {
		rotation Rotation
		wantX    int16
		wantY    int16
	}{
		{Rotation0, 0, 511},
		{Rotation90, 511, 0},
		{Rotation180, 0, -511},
		{Rotation270, -511, 0},
	}

	for _, c := range cases {
		fake, d := configured(t)
		d.SetRotation(c.rotation)
		fake.setRawPosition(1023, Center)

		x, y, err := d.Position()
		if err != nil {
			t.Fatal(err)
		}
		if x != c.wantX || y != c.wantY {
			t.Errorf("Position() with rotation %d = %d, %d; want %d, %d", c.rotation, x, y, c.wantX, c.wantY)
		}
	}
}

// TestRotationZeroValueIsIdentity proves a Device that never sets Rotation
// (Config{} leaves it at its zero value) applies no extra rotation beyond
// the mandatory axis correction, matching the Rotation0 case above exactly.
func TestRotationZeroValueIsIdentity(t *testing.T) {
	fake, d := configured(t)
	fake.setRawPosition(1023, Center)

	x, y, err := d.Position()
	if err != nil {
		t.Fatal(err)
	}
	if x != 0 || y != 511 {
		t.Errorf("Position() with unset Rotation = %d, %d; want 0, 511 (same as Rotation0)", x, y)
	}
}

// TestMirrorAlone pins MirrorHorizontal and MirrorVertical each applied on
// their own, against explicit expected values.
func TestMirrorAlone(t *testing.T) {
	fake, d := configured(t)
	d.SetMirror(true, false)          // horizontal only
	fake.setRawPosition(Center, 1023) // physical push right -> normalized (511, 0)
	x, y, err := d.Position()
	if err != nil {
		t.Fatal(err)
	}
	if x != -511 || y != 0 {
		t.Errorf("Position() with MirrorHorizontal = %d, %d; want -511, 0", x, y)
	}

	d.SetMirror(false, true)          // vertical only
	fake.setRawPosition(1023, Center) // physical push up -> normalized (0, 511)
	x, y, err = d.Position()
	if err != nil {
		t.Fatal(err)
	}
	if x != 0 || y != -511 {
		t.Errorf("Position() with MirrorVertical = %d, %d; want 0, -511", x, y)
	}
}

// TestMirrorBothEqualsRotation180 confirms, with real numbers rather than
// just documentation prose, that mirroring both axes is the same transform
// as an additional Rotation180: the same raw reading produces identical
// Position output whichever of the two configurations is used.
func TestMirrorBothEqualsRotation180(t *testing.T) {
	fakeA, dA := configured(t)
	dA.SetMirror(true, true)
	fakeA.setRawPosition(Center, 1023)

	fakeB, dB := configured(t)
	dB.SetRotation(Rotation180)
	fakeB.setRawPosition(Center, 1023)

	xA, yA, errA := dA.Position()
	xB, yB, errB := dB.Position()
	if errA != nil || errB != nil {
		t.Fatal(errA, errB)
	}
	if xA != xB || yA != yB {
		t.Errorf("MirrorHorizontal+MirrorVertical = %d, %d; Rotation180 = %d, %d; want equal", xA, yA, xB, yB)
	}
	if xA != -511 || yA != 0 {
		t.Errorf("Position() = %d, %d; want -511, 0", xA, yA)
	}
}

// TestRotationThenMirrorOrder pins Rotation90 combined with
// MirrorHorizontal against the value produced by rotating first and then
// mirroring, as Position documents. The value the opposite order (mirror
// first, then rotate) would produce - (100, 300), not (-100, -300) - is
// noted here so a future refactor that silently reverses the pipeline
// fails this test instead of passing unnoticed.
func TestRotationThenMirrorOrder(t *testing.T) {
	fake, d := configured(t)
	d.SetRotation(Rotation90)
	d.SetMirror(true, false)

	// Raw (612, 812): normalized (300, 100). Rotate90 -> (100, -300).
	// Mirror horizontal -> (-100, -300). (Mirroring (300,100) first would
	// give (-300,100), then rotating gives (100,300) - a different answer.)
	fake.setRawPosition(612, 812)
	x, y, err := d.Position()
	if err != nil {
		t.Fatal(err)
	}
	if x != -100 || y != -300 {
		t.Errorf("Position() = %d, %d; want -100, -300", x, y)
	}
}

// TestRotationThenMirrorOrderSecondCase is a second rotation+mirror
// combination (Rotation270 with MirrorVertical), to guard against the first
// combination's correctness being a coincidence of the particular numbers
// chosen.
func TestRotationThenMirrorOrderSecondCase(t *testing.T) {
	fake, d := configured(t)
	d.SetRotation(Rotation270)
	d.SetMirror(false, true)

	// Raw (650, 750): normalized (238, 138). Rotate270 -> (-138, 238).
	// Mirror vertical -> (-138, -238).
	fake.setRawPosition(650, 750)
	x, y, err := d.Position()
	if err != nil {
		t.Fatal(err)
	}
	if x != -138 || y != -238 {
		t.Errorf("Position() = %d, %d; want -138, -238", x, y)
	}
}

// TestRawPositionUnaffectedByRotationAndMirror proves RawPosition reports
// the firmware's bytes exactly as sent, regardless of any Rotation or
// mirroring configured - it is Position, not RawPosition, that transforms.
func TestRawPositionUnaffectedByRotationAndMirror(t *testing.T) {
	fake, d := configured(t)
	d.SetRotation(Rotation270)
	d.SetMirror(true, true)

	fake.setRawPosition(712, 312)
	x, y, err := d.RawPosition()
	if err != nil {
		t.Fatal(err)
	}
	if x != 712 || y != 312 {
		t.Errorf("RawPosition() = %d, %d; want 712, 312 (unaffected by rotation/mirror)", x, y)
	}
}

// TestSetRotationAndSetMirrorDoNotTouchHardware proves both setters are
// pure software state: no I2C transaction is issued by either.
func TestSetRotationAndSetMirrorDoNotTouchHardware(t *testing.T) {
	fake, d := configured(t)
	fake.writes = nil

	d.SetRotation(Rotation90)
	d.SetMirror(true, true)

	if len(fake.writes) != 0 {
		t.Errorf("writes after SetRotation/SetMirror = %v, want none", fake.writes)
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
