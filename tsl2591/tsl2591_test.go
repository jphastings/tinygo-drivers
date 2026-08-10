package tsl2591

import (
	"errors"
	"testing"

	"tinygo.org/x/drivers"
)

// fakeTSL2591 emulates a TSL2591 on the I2C bus, including the
// COMMAND-byte transaction-type distinction (normal register access vs.
// special function) that real hardware enforces but a naive fake would
// happily ignore.
type fakeTSL2591 struct {
	regs            [0x18]byte
	regWrites       []regWrite
	specialFuncsHit []uint8

	// suppressAVALID keeps STATUS.AVALID clear regardless of ENABLE
	// writes, to exercise the conversion-timeout path.
	suppressAVALID bool
}

type regWrite struct {
	reg   uint8
	value uint8
}

func newFakeTSL2591() *fakeTSL2591 {
	f := &fakeTSL2591{}
	f.regs[regID] = deviceID
	return f
}

func (f *fakeTSL2591) setChannels(ch0, ch1 uint16) {
	f.regs[regC0DATAL], f.regs[regC0DATAH] = byte(ch0), byte(ch0>>8)
	f.regs[regC1DATAL], f.regs[regC1DATAH] = byte(ch1), byte(ch1>>8)
}

func (f *fakeTSL2591) Tx(addr uint16, w, r []byte) error {
	if addr != I2CAddress {
		return errors.New("wrong I2C address")
	}
	if len(w) == 0 {
		return errors.New("empty write")
	}

	transactionType := w[0] & 0xE0 // CMD | TRANSACTION
	field := w[0] & 0x1F           // ADDR or SF

	switch {
	case transactionType == commandSpecial && len(w) == 1 && len(r) == 0:
		f.specialFuncsHit = append(f.specialFuncsHit, field)
		return nil

	case transactionType == commandNormal && len(w) == 2 && len(r) == 0:
		f.regWrites = append(f.regWrites, regWrite{field, w[1]})
		f.regs[field] = w[1]
		if field == regEnable {
			if w[1]&enableAEN != 0 && !f.suppressAVALID {
				f.regs[regStatus] |= statusAVALID
			} else {
				f.regs[regStatus] &^= statusAVALID
			}
		}
		return nil

	case transactionType == commandNormal && len(w) == 1 && len(r) > 0:
		copy(r, f.regs[field:])
		return nil
	}

	return errors.New("unsupported transaction")
}

func TestConnected(t *testing.T) {
	fake := newFakeTSL2591()
	d := New(fake)

	if !d.Connected() {
		t.Error("expected device with correct device ID to be connected")
	}

	fake.regs[regID] = 0x99
	if d.Connected() {
		t.Error("expected device with wrong device ID to not be connected")
	}
}

func TestConfigureRejectsMissingDevice(t *testing.T) {
	d := New(&fakeTSL2591{})
	if err := d.Configure(Config{}); err != errNotConnected {
		t.Errorf("Configure() = %v, want errNotConnected", err)
	}
}

func TestConfigureAndSetters(t *testing.T) {
	fake := newFakeTSL2591()
	d := New(fake)

	if err := d.Configure(Config{Gain: GainHigh, IntegrationTime: IntegrationTime300ms}); err != nil {
		t.Fatal(err)
	}
	if d.Gain() != GainHigh || d.IntegrationTime() != IntegrationTime300ms {
		t.Errorf("after Configure: gain=%v integrationTime=%v, want GainHigh/IntegrationTime300ms", d.Gain(), d.IntegrationTime())
	}
	if got := fake.regs[regConfig]; got != uint8(GainHigh)|uint8(IntegrationTime300ms) {
		t.Errorf("CONFIG register = %#02x, want %#02x", got, uint8(GainHigh)|uint8(IntegrationTime300ms))
	}

	if err := d.SetGain(GainMedium); err != nil {
		t.Fatal(err)
	}
	if d.Gain() != GainMedium || d.IntegrationTime() != IntegrationTime300ms {
		t.Errorf("SetGain changed integration time: gain=%v integrationTime=%v", d.Gain(), d.IntegrationTime())
	}

	if err := d.SetIntegrationTime(IntegrationTime600ms); err != nil {
		t.Fatal(err)
	}
	if d.Gain() != GainMedium || d.IntegrationTime() != IntegrationTime600ms {
		t.Errorf("SetIntegrationTime changed gain: gain=%v integrationTime=%v", d.Gain(), d.IntegrationTime())
	}

	if err := d.SetGain(Gain(0xFF)); err != errInvalidGain {
		t.Errorf("SetGain(invalid) = %v, want errInvalidGain", err)
	}
	if err := d.SetIntegrationTime(IntegrationTime(0xFF)); err != errInvalidIntegrationTime {
		t.Errorf("SetIntegrationTime(invalid) = %v, want errInvalidIntegrationTime", err)
	}
}

// TestConfigureReassertsAEN pins that every settings change re-toggles AEN
// (rather than just writing CONFIG), which is what makes STATUS.AVALID
// trustworthy for a read taken under the new settings; see applyConfig.
func TestConfigureReassertsAEN(t *testing.T) {
	fake := newFakeTSL2591()
	d := New(fake)

	if err := d.Configure(Config{Gain: GainLow, IntegrationTime: IntegrationTime100ms}); err != nil {
		t.Fatal(err)
	}

	want := []regWrite{
		{regConfig, uint8(GainLow) | uint8(IntegrationTime100ms)},
		{regEnable, enablePON},
		{regEnable, enablePON | enableAEN},
	}
	if len(fake.regWrites) != len(want) {
		t.Fatalf("register writes = %v, want %v", fake.regWrites, want)
	}
	for i, w := range want {
		if fake.regWrites[i] != w {
			t.Errorf("write %d = %v, want %v", i, fake.regWrites[i], w)
		}
	}
}

func TestReadChannels(t *testing.T) {
	fake := newFakeTSL2591()
	d := New(fake)
	if err := d.Configure(Config{}); err != nil {
		t.Fatal(err)
	}
	fake.setChannels(12345, 678)

	ch0, ch1, err := d.ReadChannels()
	if err != nil {
		t.Fatal(err)
	}
	if ch0 != 12345 || ch1 != 678 {
		t.Errorf("ReadChannels() = (%d, %d), want (12345, 678)", ch0, ch1)
	}

	gotCh0, gotCh1 := d.Channels()
	if gotCh0 != 12345 || gotCh1 != 678 {
		t.Errorf("Channels() = (%d, %d), want (12345, 678)", gotCh0, gotCh1)
	}
}

func TestReadChannelsTimesOutWithoutAVALID(t *testing.T) {
	fake := newFakeTSL2591()
	fake.suppressAVALID = true
	d := New(fake)
	if err := d.Configure(Config{IntegrationTime: IntegrationTime100ms}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := d.ReadChannels(); err != errConversionTimeout {
		t.Errorf("ReadChannels() = %v, want errConversionTimeout", err)
	}
}

// TestIlluminance pins the exact lux conversion (the AMS coefficient-based
// formula, see lux.go) against known inputs, including a case built
// directly from the datasheet's own published typical ADC counts, so a
// wrong coefficient or a wrong gain/integration encoding shows up as a
// wrong number rather than merely "some number".
func TestIlluminance(t *testing.T) {
	cases := []struct {
		name            string
		ch0, ch1        uint16
		gain            Gain
		integrationTime IntegrationTime
		wantMilliLux    int32
	}{
		{
			// Datasheet Figure 9, "White light", ATIME=000b, AGAIN=Max:
			// CH0 typ 30000, CH1 typ 4996.
			name: "datasheet white-light typical counts",
			ch0:  30000, ch1: 4996,
			gain: GainMax, integrationTime: IntegrationTime100ms,
			wantMilliLux: 9009,
		},
		{
			// Datasheet Figure 9, "850nm" (IR-heavy), same conditions:
			// CH0 typ 30000, CH1 typ 19522.
			name: "datasheet 850nm typical counts",
			ch0:  30000, ch1: 19522,
			gain: GainMax, integrationTime: IntegrationTime100ms,
			wantMilliLux: 376,
		},
		{
			name: "medium gain, longer integration",
			ch0:  10000, ch1: 2000,
			gain: GainMedium, integrationTime: IntegrationTime200ms,
			wantMilliLux: 548352,
		},
		{
			name: "ch1 dominates ch0: clamp to zero, not negative",
			ch0:  100, ch1: 200,
			gain: GainLow, integrationTime: IntegrationTime100ms,
			wantMilliLux: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := newFakeTSL2591()
			d := New(fake)
			if err := d.Configure(Config{Gain: c.gain, IntegrationTime: c.integrationTime}); err != nil {
				t.Fatal(err)
			}
			fake.setChannels(c.ch0, c.ch1)

			got, err := d.Illuminance()
			if err != nil {
				t.Fatal(err)
			}
			if got != c.wantMilliLux {
				t.Errorf("Illuminance() = %d mlx, want %d mlx", got, c.wantMilliLux)
			}
			if got != d.MilliLux() {
				t.Errorf("MilliLux() = %d, want %d (from Illuminance)", d.MilliLux(), got)
			}
		})
	}
}

// TestIlluminanceGainScalesInversely checks the behavioural relationship a
// wrong AGAIN encoding would break: for the same raw counts, a higher gain
// setting must scale the derived lux down by (roughly) its nominal
// multiplier, not up, and not by some other factor.
func TestIlluminanceGainScalesInversely(t *testing.T) {
	fake := newFakeTSL2591()
	d := New(fake)
	if err := d.Configure(Config{Gain: GainLow, IntegrationTime: IntegrationTime100ms}); err != nil {
		t.Fatal(err)
	}
	fake.setChannels(20000, 1000)
	lowGainLux, err := d.Illuminance()
	if err != nil {
		t.Fatal(err)
	}

	if err := d.SetGain(GainMedium); err != nil { // 25x nominal
		t.Fatal(err)
	}
	fake.setChannels(20000, 1000)
	medGainLux, err := d.Illuminance()
	if err != nil {
		t.Fatal(err)
	}

	wantMedGainLux := lowGainLux / 25
	if medGainLux != wantMedGainLux {
		t.Errorf("25x gain lux = %d, want %d (1x gain lux / 25)", medGainLux, wantMedGainLux)
	}
}

func TestIlluminanceSaturated(t *testing.T) {
	cases := []struct {
		name            string
		integrationTime IntegrationTime
		satCh0          uint16
	}{
		{"100ms full-scale is 37888, not 65535", IntegrationTime100ms, 37888},
		{"600ms full-scale is the register max, 65535", IntegrationTime600ms, 65535},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := newFakeTSL2591()
			d := New(fake)
			if err := d.Configure(Config{IntegrationTime: c.integrationTime}); err != nil {
				t.Fatal(err)
			}
			fake.setChannels(c.satCh0, 0)

			milliLux, err := d.Illuminance()
			if err != errSaturated {
				t.Fatalf("Illuminance() error = %v, want errSaturated", err)
			}
			if milliLux != 0 {
				t.Errorf("Illuminance() = %d, want 0 on saturation", milliLux)
			}
			if !d.Saturated() {
				t.Error("Saturated() = false, want true")
			}
			if d.MilliLux() != 0 {
				t.Errorf("MilliLux() = %d, want 0 on saturation", d.MilliLux())
			}

			// One count below full scale must not be reported as saturated.
			fake.setChannels(c.satCh0-1, 0)
			if _, err := d.Illuminance(); err != nil {
				t.Errorf("Illuminance() one count below full scale = %v, want no error", err)
			}
			if d.Saturated() {
				t.Error("Saturated() = true one count below full scale")
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	fake := newFakeTSL2591()
	d := New(fake)
	// Same datasheet white-light scenario as TestIlluminance, so the
	// expected milliLux (9009) is the one already cross-checked there.
	if err := d.Configure(Config{Gain: GainMax, IntegrationTime: IntegrationTime100ms}); err != nil {
		t.Fatal(err)
	}

	// Measurements this driver doesn't support must not touch the bus.
	fake.regWrites = nil
	if err := d.Update(drivers.Temperature); err != nil {
		t.Errorf("Update(Temperature) = %v, want nil", err)
	}
	if len(fake.regWrites) != 0 {
		t.Errorf("Update(Temperature) touched the bus: %v", fake.regWrites)
	}

	fake.setChannels(30000, 4996)
	if err := d.Update(drivers.Luminosity); err != nil {
		t.Fatal(err)
	}
	if d.MilliLux() != 9009 {
		t.Errorf("MilliLux() after Update = %d, want 9009", d.MilliLux())
	}
	ch0, ch1 := d.Channels()
	if ch0 != 30000 || ch1 != 4996 {
		t.Errorf("Channels() after Update = (%d, %d), want (30000, 4996)", ch0, ch1)
	}

	// Saturation must surface through Update's error too, not just Illuminance's.
	fake.setChannels(65535, 0)
	if err := d.Update(drivers.Luminosity); err != errSaturated {
		t.Errorf("Update(Luminosity) on saturated data = %v, want errSaturated", err)
	}
}

// TestClearInterrupt pins that clearing an interrupt uses the
// special-function transaction type (COMMAND bits 6:5 = 0b11), not the
// normal register-access type, and writes no data byte - the single
// easiest place to get the two transaction types confused.
func TestClearInterrupt(t *testing.T) {
	fake := newFakeTSL2591()
	d := New(fake)

	if err := d.ClearInterrupt(); err != nil {
		t.Fatal(err)
	}

	want := []uint8{sfClearALSAndNoPersistInterrupt}
	if len(fake.specialFuncsHit) != len(want) || fake.specialFuncsHit[0] != want[0] {
		t.Errorf("special functions issued = %v, want %v", fake.specialFuncsHit, want)
	}
	if len(fake.regWrites) != 0 {
		t.Errorf("ClearInterrupt performed a normal register write: %v", fake.regWrites)
	}
}
