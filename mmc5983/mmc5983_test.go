package mmc5983

import (
	"errors"
	"testing"
)

type write struct {
	register uint8
	value    uint8
}

// fakeMMC emulates an MMC5983MA on the I2C bus: readable output registers,
// write-only control registers (reads of them return garbage), and
// measurements that complete as soon as they are triggered.
type fakeMMC struct {
	regs         [0x30]byte
	writes       []write
	controlReads int
}

func newFakeMMC() *fakeMMC {
	f := &fakeMMC{}
	f.regs[PROD_ID_REG] = PROD_ID
	return f
}

func (f *fakeMMC) Tx(addr uint16, w, r []byte) error {
	if addr != I2C_ADDR {
		return errors.New("wrong I2C address")
	}

	if len(w) == 2 { // register write
		f.writes = append(f.writes, write{w[0], w[1]})
		if w[0] == INT_CTRL_0_REG {
			if w[1]&BITS_TM_M != 0 {
				f.regs[STATUS_REG] |= BITS_MEAS_M_DONE
			}
			if w[1]&BITS_TM_T != 0 {
				f.regs[STATUS_REG] |= BITS_MEAS_T_DONE
			}
		}
		return nil
	}

	if len(w) == 1 && len(r) > 0 { // register read
		reg := w[0]
		if reg >= INT_CTRL_0_REG && reg <= INT_CTRL_3_REG {
			// The control registers are write-only: return garbage.
			f.controlReads++
			for i := range r {
				r[i] = 0xFF
			}
			return nil
		}
		copy(r, f.regs[reg:])
		return nil
	}

	return errors.New("unsupported transaction")
}

func TestConnected(t *testing.T) {
	fake := newFakeMMC()
	d := New(fake)

	if !d.Connected() {
		t.Error("expected device with correct product ID to be connected")
	}

	fake.regs[PROD_ID_REG] = 0x99
	if d.Connected() {
		t.Error("expected device with wrong product ID to not be connected")
	}
}

func TestReadMagneticField(t *testing.T) {
	fake := newFakeMMC()
	d := New(fake)

	// X = 131071 raw counts (1 count below null field, exercising the low
	// bits from XYZ_OUT_2), Y = 147456 (exactly +1 gauss), Z = 131075.
	fake.regs[X_OUT_0_REG] = 0x7F
	fake.regs[X_OUT_1_REG] = 0xFF
	fake.regs[Y_OUT_0_REG] = 0x90
	fake.regs[Y_OUT_1_REG] = 0x00
	fake.regs[Z_OUT_0_REG] = 0x80
	fake.regs[Z_OUT_1_REG] = 0x00
	fake.regs[XYZ_OUT_2_REG] = 0b11_00_11_00 // Xout[1:0] = 3, Zout[1:0] = 3

	x, y, z, err := d.ReadMagneticField()
	if err != nil {
		t.Fatal(err)
	}

	// Expected nT: (counts - 131072) * 100_000 / 16384.
	if x != -6 {
		t.Errorf("x = %d nT, want -6", x)
	}
	if y != 100000 {
		t.Errorf("y = %d nT, want 100000", y)
	}
	if z != 18 {
		t.Errorf("z = %d nT, want 18", z)
	}
}

func TestControlRegistersWrittenNeverRead(t *testing.T) {
	fake := newFakeMMC()
	d := New(fake)

	if err := d.SetBandwidth(Bandwidth800Hz); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := d.ReadMagneticField(); err != nil {
		t.Fatal(err)
	}
	if err := d.PerformSet(); err != nil {
		t.Fatal(err)
	}

	if fake.controlReads != 0 {
		t.Errorf("control registers were read %d times, they are write-only", fake.controlReads)
	}

	want := []write{
		{INT_CTRL_1_REG, BITS_BW0 | BITS_BW1}, // 800Hz bandwidth
		{INT_CTRL_0_REG, BITS_TM_M},           // trigger measurement
		{INT_CTRL_0_REG, BITS_SET_OPERATION},  // TM_M cleared in shadow first
	}
	if len(fake.writes) != len(want) {
		t.Fatalf("control register writes = %v, want %v", fake.writes, want)
	}
	for i, w := range want {
		if fake.writes[i] != w {
			t.Errorf("write %d = %v, want %v", i, fake.writes[i], w)
		}
	}

	if d.Bandwidth() != Bandwidth800Hz {
		t.Errorf("Bandwidth() = %d, want Bandwidth800Hz", d.Bandwidth())
	}
}

func TestConfigureResetsAndSetsBandwidth(t *testing.T) {
	fake := newFakeMMC()
	d := New(fake)

	if err := d.Configure(Config{Bandwidth: Bandwidth400Hz}); err != nil {
		t.Fatal(err)
	}

	want := []write{
		{INT_CTRL_1_REG, BITS_SW_RST},
		{INT_CTRL_1_REG, BITS_BW1}, // 400Hz: BW[1:0] = 0b10, on a clean shadow
	}
	if len(fake.writes) != len(want) {
		t.Fatalf("control register writes = %v, want %v", fake.writes, want)
	}
	for i, w := range want {
		if fake.writes[i] != w {
			t.Errorf("write %d = %v, want %v", i, fake.writes[i], w)
		}
	}

	noDevice := New(&fakeMMC{})
	if err := noDevice.Configure(Config{}); err != errNotConnected {
		t.Errorf("Configure without device = %v, want errNotConnected", err)
	}
}

func TestReadTemperature(t *testing.T) {
	cases := []struct {
		raw  uint8
		want int32 // milli °C
	}{
		{0x00, -75000},
		{0xFF, 125000},
		{0x80, 25392}, // -75 + 128*200/255 ≈ 25.39°C
	}

	for _, c := range cases {
		fake := newFakeMMC()
		fake.regs[T_OUT_REG] = c.raw
		d := New(fake)

		got, err := d.ReadTemperature()
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("ReadTemperature(raw %#02x) = %d, want %d", c.raw, got, c.want)
		}
	}
}
