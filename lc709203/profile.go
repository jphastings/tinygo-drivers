package lc709203

import "errors"

// BatteryProfile selects which of the IC's two factory-loaded battery
// profiles the HG-CVR algorithm uses (the datasheet's Change of the
// Parameter register, 0x12). What each profile number actually means -
// nominal/charging voltage and design capacity - depends on which part
// number was ordered; see Table 8 ("Battery Profile vs. Register") of the
// datasheet for the part in hand.
type BatteryProfile uint16

const (
	// BatteryProfile0 is the register's own power-on default.
	BatteryProfile0 BatteryProfile = 0x0000
	BatteryProfile1 BatteryProfile = 0x0001
)

var errInvalidBatteryProfile = errors.New("lc709203: invalid battery profile, must be 0 or 1")

func (p BatteryProfile) valid() bool {
	return p == BatteryProfile0 || p == BatteryProfile1
}

// PackSize sets the Adjustment Pack Application (APA) register: the design
// capacity of the connected battery, which the HG-CVR algorithm needs to
// compute an accurate RSOC. Table 7 of the datasheet ties APA byte values
// to design capacity differently per ordered part number; these named
// constants are its "Type-01, Type-03" column, the variant used by (for
// example) Adafruit's LC709203F breakout, product #4712. If a different
// part is in hand, look up the right byte in Table 7 and convert it to a
// PackSize directly rather than relying on these names.
type PackSize uint16

const (
	PackSize100mAh  PackSize = 0x08
	PackSize200mAh  PackSize = 0x0B
	PackSize500mAh  PackSize = 0x10
	PackSize1000mAh PackSize = 0x19
	PackSize2000mAh PackSize = 0x2D
	PackSize3000mAh PackSize = 0x36
)
