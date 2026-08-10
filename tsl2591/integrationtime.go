package tsl2591

import (
	"errors"
	"time"
)

// IntegrationTime selects how long each ADC conversion integrates the
// photodiode current for, from 100ms to 600ms in 100ms steps. Its value
// maps directly onto the ATIME[2:0] bits of the CONFIG register: the
// integration time in milliseconds is (value+1)*100.
type IntegrationTime uint8

const (
	IntegrationTime100ms IntegrationTime = iota // default: shortest integration, least likely to saturate
	IntegrationTime200ms
	IntegrationTime300ms
	IntegrationTime400ms
	IntegrationTime500ms
	IntegrationTime600ms
)

var errInvalidIntegrationTime = errors.New("tsl2591: invalid integration time")

func (t IntegrationTime) valid() bool {
	return t <= IntegrationTime600ms
}

// steps returns the number of 100ms integration steps, i.e. ATIME+1.
func (t IntegrationTime) steps() int32 {
	return int32(t) + 1
}

// milliseconds returns the nominal integration time in milliseconds, used
// by the lux calculation.
func (t IntegrationTime) milliseconds() int32 {
	return t.steps() * 100
}

// settleDuration is the worst-case time to wait for a conversion: the
// datasheet's AC characteristics give each integration step as typically
// 101ms and at most 108ms, so this allows a small margin above the
// documented per-step maximum.
func (t IntegrationTime) settleDuration() time.Duration {
	const maxStepDuration = 110 * time.Millisecond
	return time.Duration(t.steps()) * maxStepDuration
}

// fullScaleCount is the ADC count at or above which a channel must be
// treated as saturated for this integration time - not simply 65535 (the
// register's own maximum) at every setting.
//
// Per the CONTROL/CONFIG register's ATIME field description, only the
// shortest integration time (100ms) saturates below the 16-bit register's
// natural limit, at 36863 counts; every longer setting saturates at 65535,
// the register maximum itself. This matches Adafruit's actively maintained
// CircuitPython driver (adafruit_tsl2591, MAX_COUNT_100MS = 0x8FFF). An
// earlier (Apr 2013, ams163.5) revision of the AMS datasheet listed 37888
// for the 100ms case; the current (2018-06-05, v2-04) revision corrects
// this to 36863, which is the value used here.
func (t IntegrationTime) fullScaleCount() uint16 {
	if t == IntegrationTime100ms {
		return 36863
	}
	return 65535
}
