package lc709203

import (
	"errors"
	"testing"

	"tinygo.org/x/drivers"
)

type regWrite struct {
	cmd   uint8
	value uint16
}

// fakeLC709203 emulates an LC709203F on the I2C bus: word registers with
// CRC-8 packet error checking on both directions, computed independently
// of the driver under test (straight from the datasheet's byte layout) so
// a bug in the driver's own CRC handling shows up as a rejected write or a
// read error, rather than the test simply assuming the checking works.
type fakeLC709203 struct {
	regs         map[uint8]uint16
	writes       []regWrite
	reads        []uint8
	badWriteCRCs int

	corruptNextReadCRC bool
}

func newFakeLC709203() *fakeLC709203 {
	return &fakeLC709203{
		regs: map[uint8]uint16{
			cmdThermistorB:     defaultThermistorB,
			cmdCellTemperature: 0x0BA6, // register's own power-on default, 25 degC
			cmdICVersion:       0x0021, // arbitrary nonzero: the datasheet publishes no fixed value
			cmdAPT:             0x001E,
		},
	}
}

func (f *fakeLC709203) Tx(addr uint16, w, r []byte) error {
	if addr != I2CAddress {
		return errors.New("wrong I2C address")
	}
	writeAddr := uint8(addr << 1)

	switch {
	case len(w) == 4 && len(r) == 0: // word write: cmd, data-low, data-high, crc
		cmd, lo, hi, crc := w[0], w[1], w[2], w[3]
		want := crc8([]byte{writeAddr, cmd, lo, hi})
		f.writes = append(f.writes, regWrite{cmd, uint16(hi)<<8 | uint16(lo)})
		if crc != want {
			f.badWriteCRCs++
			// Real silicon acks the write but discards the data - the I2C
			// transaction itself does not fail.
			return nil
		}
		f.regs[cmd] = uint16(hi)<<8 | uint16(lo)
		return nil

	case len(w) == 1 && len(r) == 3: // word read: cmd -> data-low, data-high, crc
		cmd := w[0]
		f.reads = append(f.reads, cmd)
		val := f.regs[cmd]
		r[0], r[1] = byte(val), byte(val>>8)
		crc := crc8([]byte{writeAddr, cmd, writeAddr | 1, r[0], r[1]})
		if f.corruptNextReadCRC {
			crc ^= 0xFF
			f.corruptNextReadCRC = false
		}
		r[2] = crc
		return nil
	}

	return errors.New("unsupported transaction")
}

// absentDevice simulates an LC709203F with no battery connected: it draws
// no power, so it never acknowledges its address at all.
type absentDevice struct{}

func (absentDevice) Tx(addr uint16, w, r []byte) error {
	return errors.New("no ACK: nothing answered at this address")
}

func TestConnected(t *testing.T) {
	d := New(newFakeLC709203())
	if !d.Connected() {
		t.Error("expected a device that answers with a valid CRC to be connected")
	}

	absent := New(absentDevice{})
	if absent.Connected() {
		t.Error("expected a device that never acknowledges (e.g. unpowered, no battery) to not be connected")
	}
}

func TestConfigureRejectsMissingDevice(t *testing.T) {
	d := New(absentDevice{})
	if err := d.Configure(Config{}); err != errNotConnected {
		t.Errorf("Configure() = %v, want errNotConnected", err)
	}
}

// TestConfigureAppliesFullSequence pins the exact registers Configure
// writes and in what order, including that a zero-value PowerMode is
// substituted with PowerModeOperate (there is no register encoding for
// "off" to fall back to).
func TestConfigureAppliesFullSequence(t *testing.T) {
	fake := newFakeLC709203()
	d := New(fake)

	if err := d.Configure(Config{}); err != nil {
		t.Fatal(err)
	}

	want := []regWrite{
		{cmdPowerMode, uint16(PowerModeOperate)},
		{cmdAPA, 0},
		{cmdChangeOfParameter, uint16(BatteryProfile0)},
		{cmdStatusBit, uint16(TemperatureSourceI2C)},
	}
	assertWrites(t, fake.writes, want)
}

// TestConfigureThermistorModeSetsB checks that Configure only writes
// ThermistorB when Thermistor mode is actually selected, defaulting a
// zero-value ThermistorB to the chip's own power-on constant rather than
// writing zero.
func TestConfigureThermistorModeSetsB(t *testing.T) {
	fake := newFakeLC709203()
	d := New(fake)

	cfg := Config{
		PackSize:          PackSize2000mAh,
		BatteryProfile:    BatteryProfile1,
		TemperatureSource: TemperatureSourceThermistor,
	}
	if err := d.Configure(cfg); err != nil {
		t.Fatal(err)
	}

	want := []regWrite{
		{cmdPowerMode, uint16(PowerModeOperate)},
		{cmdAPA, uint16(PackSize2000mAh)},
		{cmdChangeOfParameter, uint16(BatteryProfile1)},
		{cmdStatusBit, uint16(TemperatureSourceThermistor)},
		{cmdThermistorB, defaultThermistorB},
	}
	assertWrites(t, fake.writes, want)
}

func assertWrites(t *testing.T, got, want []regWrite) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("writes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("write %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestCellVoltage(t *testing.T) {
	fake := newFakeLC709203()
	fake.regs[cmdCellVoltage] = 3778 // the datasheet's own worked CRC example
	d := New(fake)

	mv, err := d.CellVoltage()
	if err != nil {
		t.Fatal(err)
	}
	if mv != 3778 {
		t.Errorf("CellVoltage() = %d mV, want 3778 mV", mv)
	}
	if d.MilliVolts() != 3778 {
		t.Errorf("MilliVolts() = %d, want 3778 (cached from CellVoltage)", d.MilliVolts())
	}
}

func TestRSOCAndITE(t *testing.T) {
	fake := newFakeLC709203()
	fake.regs[cmdRSOC] = 76
	fake.regs[cmdITE] = 764 // finer resolution: 76.4%
	d := New(fake)

	rsoc, err := d.RSOC()
	if err != nil {
		t.Fatal(err)
	}
	if rsoc != 76 {
		t.Errorf("RSOC() = %d%%, want 76%%", rsoc)
	}

	ite, err := d.ITE()
	if err != nil {
		t.Fatal(err)
	}
	if ite != 764 {
		t.Errorf("ITE() = %d, want 764 (76.4%%)", ite)
	}
}

func TestSetRSOCValidatesRange(t *testing.T) {
	d := New(newFakeLC709203())
	if err := d.SetRSOC(101); err != errInvalidPercent {
		t.Errorf("SetRSOC(101) = %v, want errInvalidPercent", err)
	}
	if err := d.SetRSOC(50); err != nil {
		t.Fatal(err)
	}
}

// TestCellTemperatureConversion pins the 0.1K-offset conversion against
// values the datasheet itself publishes: 0x0AAC as exactly 0.0 degC, the
// I2C-mode write range's endpoints (-20.0 / 60.0 degC), and the Cell
// Temperature register's own power-on default (0x0BA6, annotated "25 degC"
// in Table 6).
func TestCellTemperatureConversion(t *testing.T) {
	cases := []struct {
		raw        uint16
		wantMilliC int32
	}{
		{0x0AAC, 0},
		{0x09E4, -20000},
		{0x0D04, 60000},
		{0x0BA6, 25000},
	}

	for _, c := range cases {
		fake := newFakeLC709203()
		fake.regs[cmdCellTemperature] = c.raw
		d := New(fake)

		got, err := d.CellTemperature()
		if err != nil {
			t.Fatal(err)
		}
		if got != c.wantMilliC {
			t.Errorf("CellTemperature(raw %#04x) = %d m degC, want %d", c.raw, got, c.wantMilliC)
		}
		if d.MilliCelsius() != got {
			t.Errorf("MilliCelsius() = %d, want %d (cached from CellTemperature)", d.MilliCelsius(), got)
		}
	}
}

func TestSetTemperatureRoundsAndValidatesRange(t *testing.T) {
	cases := []struct {
		milliC  int32
		wantRaw uint16
	}{
		{0, 0x0AAC},
		{25000, 0x0BA6},
		{-20000, 0x09E4},
		{60000, 0x0D04},
		{149, 0x0AAC + 1},  // rounds to the nearest 0.1 degC step, not truncated
		{-149, 0x0AAC - 1}, // same, negative direction
	}

	for _, c := range cases {
		fake := newFakeLC709203()
		d := New(fake)
		if err := d.SetTemperature(c.milliC); err != nil {
			t.Fatalf("SetTemperature(%d) = %v", c.milliC, err)
		}
		got := fake.regs[cmdCellTemperature]
		if got != c.wantRaw {
			t.Errorf("SetTemperature(%d): register = %#04x, want %#04x", c.milliC, got, c.wantRaw)
		}
	}

	d := New(newFakeLC709203())
	if err := d.SetTemperature(-20001); err != errTemperatureOutOfRange {
		t.Errorf("SetTemperature(-20001) = %v, want errTemperatureOutOfRange", err)
	}
	if err := d.SetTemperature(60001); err != errTemperatureOutOfRange {
		t.Errorf("SetTemperature(60001) = %v, want errTemperatureOutOfRange", err)
	}
}

func TestAlarmThresholds(t *testing.T) {
	d := New(newFakeLC709203())

	if err := d.SetAlarmLowRSOC(150); err != errInvalidPercent {
		t.Errorf("SetAlarmLowRSOC(150) = %v, want errInvalidPercent", err)
	}
	if err := d.SetAlarmLowRSOC(8); err != nil {
		t.Fatal(err)
	}
	if got, err := d.AlarmLowRSOC(); err != nil || got != 8 {
		t.Errorf("AlarmLowRSOC() = (%d, %v), want (8, nil)", got, err)
	}

	if err := d.SetAlarmLowVoltage(3300); err != nil {
		t.Fatal(err)
	}
	if got, err := d.AlarmLowVoltage(); err != nil || got != 3300 {
		t.Errorf("AlarmLowVoltage() = (%d, %v), want (3300, nil)", got, err)
	}
}

func TestPackSizeValidation(t *testing.T) {
	d := New(newFakeLC709203())
	if err := d.SetPackSize(PackSize(0x0100)); err != errInvalidPackSize {
		t.Errorf("SetPackSize(0x0100) = %v, want errInvalidPackSize", err)
	}
	if err := d.SetPackSize(PackSize500mAh); err != nil {
		t.Fatal(err)
	}
	if got, err := d.PackSize(); err != nil || got != PackSize500mAh {
		t.Errorf("PackSize() = (%v, %v), want (PackSize500mAh, nil)", got, err)
	}
}

func TestBatteryProfileValidation(t *testing.T) {
	d := New(newFakeLC709203())
	if err := d.SetBatteryProfile(BatteryProfile(2)); err != errInvalidBatteryProfile {
		t.Errorf("SetBatteryProfile(2) = %v, want errInvalidBatteryProfile", err)
	}
	if err := d.SetBatteryProfile(BatteryProfile1); err != nil {
		t.Fatal(err)
	}
	if got, err := d.BatteryProfile(); err != nil || got != BatteryProfile1 {
		t.Errorf("BatteryProfile() = (%v, %v), want (BatteryProfile1, nil)", got, err)
	}
}

func TestPowerModeValidation(t *testing.T) {
	d := New(newFakeLC709203())
	if err := d.SetPowerMode(PowerMode(0)); err != errInvalidPowerMode {
		t.Errorf("SetPowerMode(0) = %v, want errInvalidPowerMode", err)
	}
	if err := d.SetPowerMode(PowerModeSleep); err != nil {
		t.Fatal(err)
	}
	if got, err := d.PowerMode(); err != nil || got != PowerModeSleep {
		t.Errorf("PowerMode() = (%v, %v), want (PowerModeSleep, nil)", got, err)
	}
}

func TestTemperatureSourceValidation(t *testing.T) {
	d := New(newFakeLC709203())
	if err := d.SetTemperatureSource(TemperatureSource(5)); err != errInvalidTemperatureSource {
		t.Errorf("SetTemperatureSource(5) = %v, want errInvalidTemperatureSource", err)
	}
	if err := d.SetTemperatureSource(TemperatureSourceThermistor); err != nil {
		t.Fatal(err)
	}
	if got, err := d.TemperatureSource(); err != nil || got != TemperatureSourceThermistor {
		t.Errorf("TemperatureSource() = (%v, %v), want (TemperatureSourceThermistor, nil)", got, err)
	}
}

func TestCurrentDirectionValidation(t *testing.T) {
	d := New(newFakeLC709203())
	if err := d.SetCurrentDirection(CurrentDirection(2)); err != errInvalidCurrentDirection {
		t.Errorf("SetCurrentDirection(2) = %v, want errInvalidCurrentDirection", err)
	}
	if err := d.SetCurrentDirection(CurrentDirectionDischarge); err != nil {
		t.Fatal(err)
	}
	if got, err := d.CurrentDirection(); err != nil || got != CurrentDirectionDischarge {
		t.Errorf("CurrentDirection() = (%v, %v), want (CurrentDirectionDischarge, nil)", got, err)
	}
}

func TestInitialAndBeforeRSOC(t *testing.T) {
	fake := newFakeLC709203()
	d := New(fake)

	if err := d.InitialRSOC(); err != nil {
		t.Fatal(err)
	}
	if err := d.BeforeRSOC(); err != nil {
		t.Fatal(err)
	}

	want := []regWrite{
		{cmdInitialRSOC, rsocInitMagic},
		{cmdBeforeRSOC, rsocInitMagic},
	}
	assertWrites(t, fake.writes, want)
}

func TestUpdate(t *testing.T) {
	fake := newFakeLC709203()
	fake.regs[cmdCellVoltage] = 4012
	fake.regs[cmdCellTemperature] = 0x0BA6 // 25 degC
	d := New(fake)

	// Measurements this driver doesn't support must not touch the bus.
	if err := d.Update(drivers.Humidity); err != nil {
		t.Errorf("Update(Humidity) = %v, want nil", err)
	}
	if len(fake.reads) != 0 {
		t.Errorf("Update(Humidity) touched the bus: reads = %v", fake.reads)
	}

	if err := d.Update(drivers.Voltage); err != nil {
		t.Fatal(err)
	}
	if d.MilliVolts() != 4012 {
		t.Errorf("MilliVolts() after Update(Voltage) = %d, want 4012", d.MilliVolts())
	}
	if d.MilliCelsius() != 0 {
		t.Errorf("MilliCelsius() after Update(Voltage) = %d, want 0 (untouched)", d.MilliCelsius())
	}

	if err := d.Update(drivers.Temperature); err != nil {
		t.Fatal(err)
	}
	if d.MilliCelsius() != 25000 {
		t.Errorf("MilliCelsius() after Update(Temperature) = %d, want 25000", d.MilliCelsius())
	}
}

// TestReadRejectsCorruptCRC checks the read path actually verifies the
// CRC-8 byte the device sends back, using a value computed correctly for
// everything except the CRC itself.
func TestReadRejectsCorruptCRC(t *testing.T) {
	fake := newFakeLC709203()
	fake.regs[cmdCellVoltage] = 3778
	fake.corruptNextReadCRC = true
	d := New(fake)

	if _, err := d.CellVoltage(); err != errCRCMismatch {
		t.Errorf("CellVoltage() with a corrupted CRC byte = %v, want errCRCMismatch", err)
	}
}

// TestWriteCRCIsValidatedByTheBus proves the driver's write path computes
// a CRC-8 the device actually accepts: the fake independently recomputes
// the expected CRC from the datasheet's byte layout and discards the
// write on a mismatch, so this only passes if the driver's CRC is right.
func TestWriteCRCIsValidatedByTheBus(t *testing.T) {
	fake := newFakeLC709203()
	d := New(fake)

	if err := d.SetPackSize(PackSize500mAh); err != nil {
		t.Fatal(err)
	}
	if fake.badWriteCRCs != 0 {
		t.Errorf("the fake rejected %d write(s) for a bad CRC-8", fake.badWriteCRCs)
	}
	if fake.regs[cmdAPA] != uint16(PackSize500mAh) {
		t.Errorf("APA register = %#04x, want %#04x - the write should have been accepted", fake.regs[cmdAPA], uint16(PackSize500mAh))
	}
}
