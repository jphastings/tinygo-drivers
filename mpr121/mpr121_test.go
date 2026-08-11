package mpr121

import (
	"errors"
	"testing"
)

// fakeMPR121 emulates an MPR121 on the I2C bus, including the two
// behaviours a naive fake would happily ignore but real hardware
// enforces: registers 0x2B-0x7F (other than the GPIO/LED block and ECR
// itself) silently discard writes while ECR is non-zero (Run Mode), and
// a soft reset restores every register to its power-on value, which is
// non-zero only for regAFEConfig1 and regFilterConfig.
type fakeMPR121 struct {
	regs               [0x81]byte
	regWrites          []regWrite
	writesWhileRunning []regWrite
}

type regWrite struct {
	reg   uint8
	value uint8
}

func newFakeMPR121() *fakeMPR121 {
	f := &fakeMPR121{}
	f.powerOnReset()
	return f
}

func (f *fakeMPR121) powerOnReset() {
	for i := range f.regs {
		f.regs[i] = 0
	}
	f.regs[regAFEConfig1] = afeConfig1PowerOnDefault
	f.regs[regFilterConfig] = filterConfigPowerOnDefault
}

// requiresStopMode mirrors datasheet section 5.1: registers 0x2B-0x7F
// are gated on Stop Mode except the GPIO/LED block (0x73-0x7A) and ECR
// (0x5E) itself, which accept writes at any time.
func requiresStopMode(reg uint8) bool {
	if reg == regECR || (reg >= 0x73 && reg <= 0x7A) {
		return false
	}
	return reg >= 0x2B && reg <= 0x7F
}

func (f *fakeMPR121) Tx(addr uint16, w, r []byte) error {
	if addr != uint16(DefaultAddress) {
		return errors.New("NACK: wrong I2C address")
	}
	if len(w) == 0 {
		return errors.New("empty write")
	}
	reg := w[0]

	if len(w) == 1 && len(r) > 0 {
		copy(r, f.regs[reg:])
		return nil
	}

	for i, b := range w[1:] {
		target := reg + uint8(i)
		switch {
		case target == regSoftReset:
			if b == softResetMagic {
				f.powerOnReset()
			}
		case target == regTouchStatusH && b == overCurrentWriteBit:
			f.regs[regTouchStatusH] &^= overCurrentWriteBit
		case requiresStopMode(target) && f.regs[regECR] != 0:
			f.writesWhileRunning = append(f.writesWhileRunning, regWrite{target, b})
		default:
			f.regs[target] = b
			f.regWrites = append(f.regWrites, regWrite{target, b})
		}
	}
	return nil
}

func configured(t *testing.T) (*fakeMPR121, *Device) {
	t.Helper()
	fake := newFakeMPR121()
	d := New(fake, DefaultAddress)
	if err := d.Configure(Config{}); err != nil {
		t.Fatal(err)
	}
	return fake, &d
}

func TestConnected(t *testing.T) {
	fake := newFakeMPR121()
	d := New(fake, DefaultAddress)
	if !d.Connected() {
		t.Error("expected device answering at Address to be connected")
	}

	wrongAddr := New(fake, DefaultAddress+1)
	if wrongAddr.Connected() {
		t.Error("expected device at the wrong address to not be connected")
	}
}

func TestConfigureRejectsUnreachableDevice(t *testing.T) {
	d := New(newFakeMPR121(), DefaultAddress+1)
	if err := d.Configure(Config{}); err == nil {
		t.Error("expected Configure to fail for an unreachable device")
	}
}

func TestConfigureValidatesElectrodeCount(t *testing.T) {
	fake := newFakeMPR121()
	d := New(fake, DefaultAddress)

	if err := d.Configure(Config{ElectrodeCount: 13}); err != errInvalidElectrodeCount {
		t.Errorf("Configure(ElectrodeCount: 13) = %v, want errInvalidElectrodeCount", err)
	}
	if len(fake.regWrites) != 0 {
		t.Errorf("invalid Configure touched the bus: %v", fake.regWrites)
	}
}

func TestConfigureValidatesThresholds(t *testing.T) {
	fake := newFakeMPR121()
	d := New(fake, DefaultAddress)

	cfg := Config{TouchThreshold: 10, ReleaseThreshold: 20}
	if err := d.Configure(cfg); err != errReleaseNotBelowTouch {
		t.Errorf("Configure(release >= touch) = %v, want errReleaseNotBelowTouch", err)
	}
	if len(fake.regWrites) != 0 {
		t.Errorf("invalid Configure touched the bus: %v", fake.regWrites)
	}
}

func TestConfigureValidatesProximityMode(t *testing.T) {
	fake := newFakeMPR121()
	d := New(fake, DefaultAddress)
	if err := d.Configure(Config{Proximity: ProximityAll + 1}); err != errInvalidProximityMode {
		t.Errorf("Configure(invalid proximity) = %v, want errInvalidProximityMode", err)
	}
}

// TestConfigureAppliesRecommendedDefaults pins Configure's default
// register writes against AN3944, including the ECR construction
// (CL = 2b10, all 12 electrodes, proximity disabled) and the deliberate
// deviation from AN3944's literal regFilterConfig (0x5D) value - see
// the comment on Configure.
func TestConfigureAppliesRecommendedDefaults(t *testing.T) {
	fake := newFakeMPR121()
	d := New(fake, DefaultAddress)
	if err := d.Configure(Config{}); err != nil {
		t.Fatal(err)
	}

	if got, want := fake.regs[regECR], uint8(0b10_00_1100); got != want {
		t.Errorf("ECR = %#08b, want %#08b (CL=10, proximity disabled, 12 electrodes)", got, want)
	}
	if got := fake.regs[touchThresholdReg(0)]; got != defaultTouchThreshold {
		t.Errorf("ELE0 touch threshold = %d, want %d", got, defaultTouchThreshold)
	}
	if got := fake.regs[releaseThresholdReg(0)]; got != defaultReleaseThreshold {
		t.Errorf("ELE0 release threshold = %d, want %d", got, defaultReleaseThreshold)
	}
	if got := fake.regs[touchThresholdReg(11)]; got != defaultTouchThreshold {
		t.Errorf("ELE11 touch threshold = %d, want %d", got, defaultTouchThreshold)
	}
	if got := fake.regs[regNCLFalling]; got != 0xFF {
		t.Errorf("NCL Falling = %#02x, want 0xFF (AN3944 section B)", got)
	}
	if got := fake.regs[regFDLFalling]; got != 0x02 {
		t.Errorf("FDL Falling = %#02x, want 0x02 (AN3944 section B)", got)
	}

	// AN3944 section D recommends regFilterConfig = 0x04, but that value
	// zeroes the global charge time without also enabling the
	// auto-configuration this driver doesn't implement, which would
	// disable capacitance sensing entirely; Configure deliberately
	// leaves it at its power-on default instead.
	if got := fake.regs[regFilterConfig]; got != filterConfigPowerOnDefault {
		t.Errorf("regFilterConfig = %#02x, want power-on default %#02x, not AN3944's literal 0x04",
			got, filterConfigPowerOnDefault)
	}
}

func TestConfigureCustomElectrodeCount(t *testing.T) {
	fake := newFakeMPR121()
	d := New(fake, DefaultAddress)
	if err := d.Configure(Config{ElectrodeCount: 4}); err != nil {
		t.Fatal(err)
	}
	if got, want := fake.regs[regECR]&0x0F, uint8(4); got != want {
		t.Errorf("ECR electrode count = %d, want %d", got, want)
	}
}

// TestBaselineShift pins the 8-bit-register/10-bit-value scaling: the
// register stores only the baseline's upper 8 bits, so reading it
// without correction makes the baseline look about 4x smaller than it
// really is - see the comment on Baseline.
func TestBaselineShift(t *testing.T) {
	fake, d := configured(t)
	fake.regs[baselineReg(3)] = 0x50 // upper 8 bits of a 10-bit baseline

	got, err := d.Baseline(3)
	if err != nil {
		t.Fatal(err)
	}
	if want := uint16(0x50) << 2; got != want {
		t.Errorf("Baseline(3) = %d, want %d (register value << 2, not the raw byte)", got, want)
	}
}

func TestFilteredData(t *testing.T) {
	fake, d := configured(t)
	fake.regs[filteredDataReg(5)], fake.regs[filteredDataReg(5)+1] = 0x34, 0x03 // 0x0334 = 820

	got, err := d.FilteredData(5)
	if err != nil {
		t.Fatal(err)
	}
	if got != 820 {
		t.Errorf("FilteredData(5) = %d, want 820", got)
	}
}

func TestTouchedBitmaskAndPerElectrode(t *testing.T) {
	fake, d := configured(t)
	fake.regs[regTouchStatusL] = 0b1010_0000                       // ELE5, ELE7
	fake.regs[regTouchStatusH] = overCurrentWriteBit | 1<<4 | 1<<3 // OVCF, ELEPROX, ELE11

	status, err := d.Touched()
	if err != nil {
		t.Fatal(err)
	}
	const want = 1<<15 | 1<<12 | 1<<11 | 1<<7 | 1<<5
	if status != want {
		t.Errorf("Touched() = %016b, want %016b", status, want)
	}

	for electrode, wantTouched := range map[uint8]bool{
		5:                  true,
		7:                  true,
		6:                  false,
		11:                 true,
		ElectrodeProximity: true,
	} {
		touched, err := d.IsTouched(electrode)
		if err != nil {
			t.Fatal(err)
		}
		if touched != wantTouched {
			t.Errorf("IsTouched(%d) = %v, want %v", electrode, touched, wantTouched)
		}
	}

	if _, err := d.IsTouched(ElectrodeProximity + 1); err != errInvalidElectrode {
		t.Errorf("IsTouched(out of range) = %v, want errInvalidElectrode", err)
	}
}

func TestOverCurrentAndClear(t *testing.T) {
	fake, d := configured(t)
	runningECR := fake.regs[regECR]

	fake.regs[regTouchStatusH] = overCurrentWriteBit
	// The datasheet documents that an over-current fault also zeroes
	// ECR's electrode-enable bits, forcing Stop Mode.
	fake.regs[regECR] = 0

	over, err := d.OverCurrent()
	if err != nil {
		t.Fatal(err)
	}
	if !over {
		t.Fatal("expected OverCurrent() to report true")
	}

	if err := d.ClearOverCurrent(); err != nil {
		t.Fatal(err)
	}
	if over, _ := d.OverCurrent(); over {
		t.Error("expected OverCurrent() to be false after ClearOverCurrent")
	}
	if fake.regs[regECR] != runningECR {
		t.Errorf("ECR after ClearOverCurrent = %#02x, want %#02x (Run Mode resumed)", fake.regs[regECR], runningECR)
	}
}

// TestSetElectrodeThresholdsSurvivesRunMode pins the MPR121's most
// easily mishandled behaviour: writes to registers like the thresholds
// are silently ignored whenever ECR is non-zero (Run Mode), with no
// I2C-level error. It shows the fake reproducing that raw behaviour
// (via writesWhileRunning) and that the driver's setters route around
// it via withStopMode, leaving Run Mode resumed afterwards.
func TestSetElectrodeThresholdsSurvivesRunMode(t *testing.T) {
	fake, d := configured(t)
	if fake.regs[regECR] == 0 {
		t.Fatal("expected Configure to leave the device running")
	}

	if err := d.SetElectrodeThresholds(2, 30, 20); err != nil {
		t.Fatal(err)
	}

	if got := fake.regs[touchThresholdReg(2)]; got != 30 {
		t.Errorf("ELE2 touch threshold = %d, want 30", got)
	}
	if got := fake.regs[releaseThresholdReg(2)]; got != 20 {
		t.Errorf("ELE2 release threshold = %d, want 20", got)
	}
	if fake.regs[regECR] == 0 {
		t.Error("SetElectrodeThresholds left the device in Stop Mode")
	}
	if len(fake.writesWhileRunning) != 0 {
		t.Errorf("driver issued a raw write while running: %v", fake.writesWhileRunning)
	}
}

func TestSetThresholdsAppliesToAllElectrodes(t *testing.T) {
	fake, d := configured(t)
	if err := d.SetThresholds(40, 25); err != nil {
		t.Fatal(err)
	}
	for e := uint8(0); e < NumElectrodes; e++ {
		if got := fake.regs[touchThresholdReg(e)]; got != 40 {
			t.Errorf("ELE%d touch threshold = %d, want 40", e, got)
		}
		if got := fake.regs[releaseThresholdReg(e)]; got != 25 {
			t.Errorf("ELE%d release threshold = %d, want 25", e, got)
		}
	}
}

func TestSetThresholdsValidation(t *testing.T) {
	_, d := configured(t)
	if err := d.SetThresholds(10, 10); err != errReleaseNotBelowTouch {
		t.Errorf("SetThresholds(10, 10) = %v, want errReleaseNotBelowTouch", err)
	}
	if err := d.SetElectrodeThresholds(ElectrodeProximity+1, 30, 20); err != errInvalidElectrode {
		t.Errorf("SetElectrodeThresholds(out of range) = %v, want errInvalidElectrode", err)
	}
}

func TestSetDebounce(t *testing.T) {
	fake, d := configured(t)
	if err := d.SetDebounce(3, 5); err != nil {
		t.Fatal(err)
	}
	if got, want := fake.regs[regDebounce], uint8(5<<4|3); got != want {
		t.Errorf("debounce register = %#02x, want %#02x (release in D6:D4, touch in D2:D0)", got, want)
	}

	if err := d.SetDebounce(8, 0); err != errInvalidDebounce {
		t.Errorf("SetDebounce(8, 0) = %v, want errInvalidDebounce", err)
	}
}
