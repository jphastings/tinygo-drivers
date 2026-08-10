package lc709203

import "errors"

// CurrentDirection constrains which way RSOC is permitted to move. The
// LC709203F has no sense resistor of its own, so unlike a coulomb counter
// it cannot detect charge vs. discharge directly; this register lets the
// host tell it.
type CurrentDirection uint16

const (
	// CurrentDirectionAuto lets RSOC move in either direction and is the
	// register's power-on default. Correct for most applications: the
	// datasheet recommends it specifically because RSOC is temperature
	// dependent (a warmed cell reports more capacity than a cold one), and
	// Auto is what lets that show up correctly either way.
	CurrentDirectionAuto CurrentDirection = 0x0000
	// CurrentDirectionCharge prevents RSOC from decreasing. Using this
	// while the battery is actually discharging is documented to
	// introduce error.
	CurrentDirectionCharge CurrentDirection = 0x0001
	// CurrentDirectionDischarge prevents RSOC from increasing. Using this
	// while the battery is actually charging is documented to introduce
	// error.
	CurrentDirectionDischarge CurrentDirection = 0xFFFF
)

var errInvalidCurrentDirection = errors.New("lc709203: invalid current direction")

func (d CurrentDirection) valid() bool {
	switch d {
	case CurrentDirectionAuto, CurrentDirectionCharge, CurrentDirectionDischarge:
		return true
	}
	return false
}
