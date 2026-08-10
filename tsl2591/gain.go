package tsl2591

import "errors"

// Gain selects the analog gain applied to both photodiode channels before
// integration. Its value maps directly onto the AGAIN[1:0] bits (5:4) of
// the CONFIG register.
type Gain uint8

const (
	GainLow    Gain = 0x00 // 1x nominal gain
	GainMedium Gain = 0x10 // 25x nominal gain
	GainHigh   Gain = 0x20 // 428x nominal gain
	GainMax    Gain = 0x30 // 9876x nominal gain
)

var errInvalidGain = errors.New("tsl2591: invalid gain")

func (g Gain) valid() bool {
	switch g {
	case GainLow, GainMedium, GainHigh, GainMax:
		return true
	}
	return false
}

// nominalMultiplier returns the nominal gain AMS specifies for each AGAIN
// setting (datasheet figure "Gain scaling, relative to 1x gain setting").
// Actual analog gain varies from device to device and with temperature;
// this nominal value is what the lux calculation uses, and it is also a
// reasonable value to sanity-check measured gain ratios against.
func (g Gain) nominalMultiplier() int32 {
	switch g {
	case GainMedium:
		return 25
	case GainHigh:
		return 428
	case GainMax:
		return 9876
	default:
		return 1
	}
}
