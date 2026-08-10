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
// Only the shortest integration time (100ms) saturates below the 16-bit
// register's natural limit; every longer setting saturates at 65535, the
// register maximum itself.
//
// The 100ms figure is measured, not quoted: driving a part into saturation
// at maximum gain clamps both channels at exactly 37888 for 100ms and at
// 65535 for every longer setting. That matches the Apr 2013 (ams163.5)
// revision of the AMS datasheet, not the 2018-06-05 (v2-04) revision or
// Adafruit's CircuitPython driver, which both give 36863. Taking the
// smaller value would report a genuine reading between the two as
// saturated.
func (t IntegrationTime) fullScaleCount() uint16 {
	if t == IntegrationTime100ms {
		return 37888
	}
	return 65535
}
