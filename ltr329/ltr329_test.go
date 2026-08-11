package ltr329

import (
	"encoding/binary"
	"errors"
	"testing"

	"tinygo.org/x/drivers"
)

// fakeLTR329 emulates an LTR-329ALS-01 on the I2C bus, including the
// behaviours real silicon enforces but a naive fake would happily ignore:
// ALS_DATA only appears as a single 4-byte auto-incrementing burst
// starting at CH1's low byte (see readRegisters), and the new-data status
// flag clears itself once that burst is read.
//
// Once active, every ALS_DATA read immediately re-arms the new-data flag,
// simulating a sensor that always has its next conversion ready by the
// time it's asked - real hardware takes measurable time between
// conversions, but nothing here needs that modelled; suppressNewData
// exists for the one test that does.
type fakeLTR329 struct {
	regs      [0x8D]byte
	regWrites []regWrite
	reads     []readCall

	// suppressNewData keeps ALS_STATUS's new-data flag clear regardless
	// of ALS_CONTR writes or ALS_DATA reads, to exercise the
	// conversion-timeout path.
	suppressNewData bool

	// staleCh0/staleCh1, armed by simulateStaleReadAfterConfigure, are
	// returned by exactly the next ALS_DATA burst read in place of
	// whatever setChannels last set - simulating a conversion that
	// completed under the previous configuration and was never read.
	staleCh0, staleCh1 uint16
	hasStale           bool
}

type regWrite struct {
	reg   uint8
	value uint8
}

type readCall struct {
	reg uint8
	n   int
}

func newFakeLTR329() *fakeLTR329 {
	f := &fakeLTR329{}
	f.regs[regPartID] = partNumberID<<4 | 0x0 // 0xA0, revision 0
	f.regs[regManufacID] = manufacturerID
	return f
}

func (f *fakeLTR329) setChannels(ch0, ch1 uint16) {
	f.regs[regALSDataCH1L], f.regs[regALSDataCH1H] = byte(ch1), byte(ch1>>8)
	f.regs[regALSDataCH0L], f.regs[regALSDataCH0H] = byte(ch0), byte(ch0>>8)
}

func (f *fakeLTR329) simulateStaleReadAfterConfigure(ch0, ch1 uint16) {
	f.staleCh0, f.staleCh1, f.hasStale = ch0, ch1, true
}

func (f *fakeLTR329) Tx(addr uint16, w, r []byte) error {
	if addr != I2CAddress {
		return errors.New("wrong I2C address")
	}
	if len(w) == 0 {
		return errors.New("empty write")
	}
	reg := w[0]

	switch {
	case len(w) == 2 && len(r) == 0:
		f.regWrites = append(f.regWrites, regWrite{reg, w[1]})
		f.regs[reg] = w[1]
		if reg == regALSContr && w[1]&alsContrModeActive != 0 && !f.suppressNewData {
			f.regs[regALSStatus] |= statusNewData
		}
		return nil

	case len(w) == 1 && len(r) > 0:
		f.reads = append(f.reads, readCall{reg, len(r)})
		isDataBurst := reg == regALSDataCH1L && len(r) == 4
		switch {
		case isDataBurst && f.hasStale:
			binary.LittleEndian.PutUint16(r[0:2], f.staleCh1)
			binary.LittleEndian.PutUint16(r[2:4], f.staleCh0)
			f.hasStale = false
		default:
			copy(r, f.regs[reg:])
		}
		if isDataBurst {
			f.regs[regALSStatus] &^= statusNewData
			if !f.suppressNewData {
				f.regs[regALSStatus] |= statusNewData
			}
		}
		return nil
	}

	return errors.New("unsupported transaction")
}

func TestConnected(t *testing.T) {
	fake := newFakeLTR329()
	d := New(fake)

	if !d.Connected() {
		t.Error("expected device with correct PART_ID/MANUFAC_ID to be connected")
	}

	fake.regs[regPartID] = 0x90 // wrong part number nibble
	if d.Connected() {
		t.Error("expected device with wrong PART_ID to not be connected")
	}

	fake.regs[regPartID] = partNumberID << 4
	fake.regs[regManufacID] = 0x99
	if d.Connected() {
		t.Error("expected device with wrong MANUFAC_ID to not be connected")
	}
}

func TestConfigureRejectsMissingDevice(t *testing.T) {
	d := New(&fakeLTR329{})
	if err := d.Configure(Config{}); err != errNotConnected {
		t.Errorf("Configure() = %v, want errNotConnected", err)
	}
}

// TestConfigureZeroValueIsValid pins that a zero-value Config is not just
// accepted but reproduces the chip's own power-on-reset register values -
// the whole reason MeasurementRate's constants are reordered away from the
// raw register bits (see measurementrate.go).
func TestConfigureZeroValueIsValid(t *testing.T) {
	fake := newFakeLTR329()
	d := New(fake)

	if err := d.Configure(Config{}); err != nil {
		t.Fatal(err)
	}
	if d.Gain() != GainX1 || d.IntegrationTime() != IntegrationTime100ms || d.MeasurementRate() != MeasurementRate500ms {
		t.Errorf("after Configure(Config{}): gain=%v integrationTime=%v measurementRate=%v", d.Gain(), d.IntegrationTime(), d.MeasurementRate())
	}
	if got := fake.regs[regALSMeasRate]; got != 0x03 {
		t.Errorf("ALS_MEAS_RATE = %#02x, want %#02x (the datasheet's own reset value)", got, 0x03)
	}
	if got := fake.regs[regALSContr]; got != 0x01 {
		t.Errorf("ALS_CONTR = %#02x, want %#02x (gain 1x, active)", got, 0x01)
	}
}

func TestConfigureAndSetters(t *testing.T) {
	fake := newFakeLTR329()
	d := New(fake)

	if err := d.Configure(Config{Gain: GainX48, IntegrationTime: IntegrationTime200ms, MeasurementRate: MeasurementRate500ms}); err != nil {
		t.Fatal(err)
	}
	if d.Gain() != GainX48 || d.IntegrationTime() != IntegrationTime200ms || d.MeasurementRate() != MeasurementRate500ms {
		t.Errorf("after Configure: gain=%v integrationTime=%v measurementRate=%v", d.Gain(), d.IntegrationTime(), d.MeasurementRate())
	}

	if err := d.SetGain(GainX96); err != nil {
		t.Fatal(err)
	}
	if d.Gain() != GainX96 || d.IntegrationTime() != IntegrationTime200ms {
		t.Errorf("SetGain changed integration time: gain=%v integrationTime=%v", d.Gain(), d.IntegrationTime())
	}

	if err := d.SetIntegrationTime(IntegrationTime300ms); err != nil {
		t.Fatal(err)
	}
	if d.IntegrationTime() != IntegrationTime300ms || d.Gain() != GainX96 {
		t.Errorf("SetIntegrationTime changed gain: gain=%v integrationTime=%v", d.Gain(), d.IntegrationTime())
	}

	if err := d.SetMeasurementRate(MeasurementRate1000ms); err != nil {
		t.Fatal(err)
	}
	if d.MeasurementRate() != MeasurementRate1000ms || d.IntegrationTime() != IntegrationTime300ms {
		t.Errorf("SetMeasurementRate changed integration time: integrationTime=%v measurementRate=%v", d.IntegrationTime(), d.MeasurementRate())
	}

	if err := d.SetGain(Gain(0xFF)); err != errInvalidGain {
		t.Errorf("SetGain(invalid) = %v, want errInvalidGain", err)
	}
	if err := d.SetIntegrationTime(IntegrationTime(0xFF)); err != errInvalidIntegrationTime {
		t.Errorf("SetIntegrationTime(invalid) = %v, want errInvalidIntegrationTime", err)
	}
	if err := d.SetMeasurementRate(MeasurementRate(0xFF)); err != errInvalidMeasurementRate {
		t.Errorf("SetMeasurementRate(invalid) = %v, want errInvalidMeasurementRate", err)
	}
}

func TestConfigureRejectsIntegrationExceedingMeasurementRate(t *testing.T) {
	fake := newFakeLTR329()
	d := New(fake)

	err := d.Configure(Config{IntegrationTime: IntegrationTime400ms, MeasurementRate: MeasurementRate200ms})
	if err != errIntegrationExceedsMeasurementRate {
		t.Errorf("Configure() = %v, want errIntegrationExceedsMeasurementRate", err)
	}
	if len(fake.regWrites) != 0 {
		t.Errorf("Configure() with an invalid combination touched the bus: %v", fake.regWrites)
	}
}

// TestConfigureWritesMeasRateBeforeContr pins that ALS_MEAS_RATE is written
// before ALS_CONTR, so the sensor knows the requested integration and
// measurement rate before its first conversion starts, rather than briefly
// running under whatever was there before; see applyConfig.
func TestConfigureWritesMeasRateBeforeContr(t *testing.T) {
	fake := newFakeLTR329()
	d := New(fake)

	if err := d.Configure(Config{Gain: GainX4, IntegrationTime: IntegrationTime200ms, MeasurementRate: MeasurementRate500ms}); err != nil {
		t.Fatal(err)
	}

	want := []regWrite{
		{regALSMeasRate, IntegrationTime200ms.registerBits() | MeasurementRate500ms.registerBits()},
		{regALSContr, uint8(GainX4) | alsContrModeActive},
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

// TestReadChannelsCH1BeforeCH0 pins the datasheet's required read order for
// ALS_DATA: a single 4-byte burst starting at CH1's low byte (0x88), not a
// read starting at CH0 (0x8A) and not two separate reads. Confirmed against
// real hardware: a burst from 0x88 returns CH1_L, CH1_H, CH0_L, CH0_H.
func TestReadChannelsCH1BeforeCH0(t *testing.T) {
	fake := newFakeLTR329()
	d := New(fake)
	if err := d.Configure(Config{}); err != nil {
		t.Fatal(err)
	}
	fake.setChannels(27, 43) // real bench reading: dim, IR-rich room
	if _, _, err := d.ReadChannels(); err != nil {
		t.Fatal(err) // consume the post-configure discard; see TestReadChannelsDiscardsStaleDataAfterConfigure
	}

	fake.reads = nil
	ch0, ch1, err := d.ReadChannels()
	if err != nil {
		t.Fatal(err)
	}
	if ch0 != 27 || ch1 != 43 {
		t.Errorf("ReadChannels() = (%d, %d), want (27, 43)", ch0, ch1)
	}

	var dataReads []readCall
	for _, rc := range fake.reads {
		if rc.reg >= regALSDataCH1L && rc.reg <= regALSDataCH0H {
			dataReads = append(dataReads, rc)
		}
	}
	if len(dataReads) != 1 || dataReads[0] != (readCall{regALSDataCH1L, 4}) {
		t.Errorf("ALS_DATA reads = %v, want a single 4-byte burst from CH1L (%#02x)", dataReads, regALSDataCH1L)
	}
}

// TestReadChannelsDiscardsStaleDataAfterConfigure pins the fix for a bug
// found on real hardware: ALS_STATUS's new-data flag only clears on an
// ALS_DATA read, never on a config write, so a conversion that completed
// under the previous settings and was never read leaves that flag already
// set the moment a reconfigure returns. Without discarding it, the next
// ReadChannels call sees the flag, skips its wait, and hands back the
// previous configuration's counts as if they were a fresh reading taken
// under the new one.
func TestReadChannelsDiscardsStaleDataAfterConfigure(t *testing.T) {
	fake := newFakeLTR329()
	d := New(fake)
	if err := d.Configure(Config{Gain: GainX1, IntegrationTime: IntegrationTime100ms, MeasurementRate: MeasurementRate2000ms}); err != nil {
		t.Fatal(err)
	}
	fake.setChannels(27, 43) // reading taken under the old configuration

	fake.simulateStaleReadAfterConfigure(27, 43)
	if err := d.Configure(Config{Gain: GainX96, IntegrationTime: IntegrationTime400ms, MeasurementRate: MeasurementRate2000ms}); err != nil {
		t.Fatal(err)
	}
	fake.setChannels(9500, 17800) // what the new configuration actually sees
	fake.reads = nil

	ch0, ch1, err := d.ReadChannels()
	if err != nil {
		t.Fatal(err)
	}
	if ch0 != 9500 || ch1 != 17800 {
		t.Errorf("ReadChannels() after reconfigure = (%d, %d), want (9500, 17800) - not the previous configuration's (27, 43)", ch0, ch1)
	}

	var dataReads int
	for _, rc := range fake.reads {
		if rc == (readCall{regALSDataCH1L, 4}) {
			dataReads++
		}
	}
	if dataReads != 2 {
		t.Errorf("ALS_DATA burst reads after reconfigure = %d, want 2 (one discarded, one kept)", dataReads)
	}
}

// TestReadChannelsDoesNotDiscardWithoutReconfigure ensures the discard in
// TestReadChannelsDiscardsStaleDataAfterConfigure only happens once per
// configuration change, not on every read.
func TestReadChannelsDoesNotDiscardWithoutReconfigure(t *testing.T) {
	fake := newFakeLTR329()
	d := New(fake)
	if err := d.Configure(Config{}); err != nil {
		t.Fatal(err)
	}
	fake.setChannels(100, 50)
	if _, _, err := d.ReadChannels(); err != nil {
		t.Fatal(err) // consume the post-configure discard
	}

	fake.reads = nil
	fake.setChannels(200, 90)
	ch0, ch1, err := d.ReadChannels()
	if err != nil {
		t.Fatal(err)
	}
	if ch0 != 200 || ch1 != 90 {
		t.Errorf("ReadChannels() = (%d, %d), want (200, 90)", ch0, ch1)
	}

	var dataReads int
	for _, rc := range fake.reads {
		if rc == (readCall{regALSDataCH1L, 4}) {
			dataReads++
		}
	}
	if dataReads != 1 {
		t.Errorf("ALS_DATA burst reads without a reconfigure = %d, want 1 (nothing to discard)", dataReads)
	}
}

func TestReadChannelsTimesOutWithoutNewData(t *testing.T) {
	fake := newFakeLTR329()
	fake.suppressNewData = true
	d := New(fake)
	if err := d.Configure(Config{IntegrationTime: IntegrationTime50ms, MeasurementRate: MeasurementRate50ms}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := d.ReadChannels(); err != errConversionTimeout {
		t.Errorf("ReadChannels() = %v, want errConversionTimeout", err)
	}
}

// TestIlluminance pins the exact lux conversion (Lite-On's Appendix A
// ratio-band formula, see lux.go) against known inputs: one from a real
// bench reading, one reproducing the datasheet's own worked example, and
// one from each remaining ratio band, so a wrong coefficient, wrong band
// boundary or wrong gain/integration encoding shows up as a wrong number.
func TestIlluminance(t *testing.T) {
	cases := []struct {
		name            string
		ch0, ch1        uint16
		gain            Gain
		integrationTime IntegrationTime
		wantMilliLux    int32
	}{
		{
			name: "ratio<0.45 band",
			ch0:  200, ch1: 20,
			gain: GainX8, integrationTime: IntegrationTime100ms,
			wantMilliLux: 47122,
		},
		{
			// Real bench reading: dim, IR-heavy room, gain 1x/100ms.
			// Ratio 43/70=0.614 lands in the middle (subtractive)
			// coefficient band.
			name: "0.45<=ratio<0.64 band, real hardware reading",
			ch0:  27, ch1: 43,
			gain: GainX1, integrationTime: IntegrationTime100ms,
			wantMilliLux: 31463,
		},
		{
			name: "0.64<=ratio<0.85 band",
			ch0:  100, ch1: 250,
			gain: GainX4, integrationTime: IntegrationTime200ms,
			wantMilliLux: 11111,
		},
		{
			name: "ratio>=0.85: clamp to zero, not negative",
			ch0:  100, ch1: 1000,
			gain: GainX1, integrationTime: IntegrationTime100ms,
			wantMilliLux: 0,
		},
		{
			// Datasheet's own ADC-count-vs-lux example (Gain=96,
			// integration 50ms, Lux~200): CH0 typ range 3250-6100,
			// CH1 typ range 830-1550 at Lux=200. Midpoint counts
			// resolve to ~200.2 lux, confirming the formula end to
			// end against the datasheet rather than just Appendix A.
			name: "datasheet ADC-count-vs-lux midpoint cross-check",
			ch0:  4675, ch1: 1190,
			gain: GainX96, integrationTime: IntegrationTime50ms,
			wantMilliLux: 200227,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := newFakeLTR329()
			d := New(fake)
			if err := d.Configure(Config{Gain: c.gain, IntegrationTime: c.integrationTime, MeasurementRate: MeasurementRate2000ms}); err != nil {
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
// wrong ALS_GAIN encoding would break: for the same raw counts, a higher
// gain setting must scale the derived lux down by (roughly) its nominal
// multiplier, not up, and not by some other factor.
func TestIlluminanceGainScalesInversely(t *testing.T) {
	fake := newFakeLTR329()
	d := New(fake)
	if err := d.Configure(Config{Gain: GainX1, IntegrationTime: IntegrationTime100ms, MeasurementRate: MeasurementRate2000ms}); err != nil {
		t.Fatal(err)
	}
	fake.setChannels(20000, 1000)
	lowGainLux, err := d.Illuminance()
	if err != nil {
		t.Fatal(err)
	}

	if err := d.SetGain(GainX48); err != nil {
		t.Fatal(err)
	}
	fake.setChannels(20000, 1000)
	highGainLux, err := d.Illuminance()
	if err != nil {
		t.Fatal(err)
	}

	wantHighGainLux := lowGainLux / 48
	if highGainLux != wantHighGainLux {
		t.Errorf("48x gain lux = %d, want %d (1x gain lux / 48)", highGainLux, wantHighGainLux)
	}
}

func TestIlluminanceDataInvalid(t *testing.T) {
	fake := newFakeLTR329()
	d := New(fake)
	if err := d.Configure(Config{}); err != nil {
		t.Fatal(err)
	}
	fake.setChannels(30000, 1000)
	fake.regs[regALSStatus] |= statusDataInvalid

	milliLux, err := d.Illuminance()
	if err != errDataInvalid {
		t.Fatalf("Illuminance() error = %v, want errDataInvalid", err)
	}
	if milliLux != 0 {
		t.Errorf("Illuminance() = %d, want 0 when data is invalid", milliLux)
	}
	if !d.DataInvalid() {
		t.Error("DataInvalid() = false, want true")
	}
	if d.MilliLux() != 0 {
		t.Errorf("MilliLux() = %d, want 0 when data is invalid", d.MilliLux())
	}
}

func TestUpdate(t *testing.T) {
	fake := newFakeLTR329()
	d := New(fake)
	// Same datasheet cross-check scenario as TestIlluminance, so the
	// expected milliLux (200227) is the one already verified there.
	if err := d.Configure(Config{Gain: GainX96, IntegrationTime: IntegrationTime50ms, MeasurementRate: MeasurementRate2000ms}); err != nil {
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

	fake.setChannels(4675, 1190)
	if err := d.Update(drivers.Luminosity); err != nil {
		t.Fatal(err)
	}
	if d.MilliLux() != 200227 {
		t.Errorf("MilliLux() after Update = %d, want 200227", d.MilliLux())
	}
	ch0, ch1 := d.Channels()
	if ch0 != 4675 || ch1 != 1190 {
		t.Errorf("Channels() after Update = (%d, %d), want (4675, 1190)", ch0, ch1)
	}

	// Data-invalid must surface through Update's error too, not just
	// Illuminance's. Reading the previous conversion already cleared the
	// new-data flag, so a fresh (invalid) conversion must set it again.
	fake.regs[regALSStatus] = statusDataInvalid | statusNewData
	if err := d.Update(drivers.Luminosity); err != errDataInvalid {
		t.Errorf("Update(Luminosity) on invalid data = %v, want errDataInvalid", err)
	}
}

func TestStatus(t *testing.T) {
	fake := newFakeLTR329()
	d := New(fake)

	fake.regs[regALSStatus] = statusDataInvalid | statusNewData
	got, err := d.Status()
	if err != nil {
		t.Fatal(err)
	}
	if got != statusDataInvalid|statusNewData {
		t.Errorf("Status() = %#02x, want %#02x", got, statusDataInvalid|statusNewData)
	}
}
