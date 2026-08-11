package ltr329

import "errors"

// Gain selects the analog gain applied to both photodiode channels before
// integration. Its value maps directly onto the ALS_GAIN[2:0] bits (4:2)
// of the ALS_CONTR register; codes 0b100 and 0b101 are reserved.
type Gain uint8

const (
	GainX1  Gain = 0x00 << alsContrGainShift // 1x nominal gain: ~1 to 64,000 lux
	GainX2  Gain = 0x01 << alsContrGainShift // 2x nominal gain: ~0.5 to 32,000 lux
	GainX4  Gain = 0x02 << alsContrGainShift // 4x nominal gain: ~0.25 to 16,000 lux
	GainX8  Gain = 0x03 << alsContrGainShift // 8x nominal gain: ~0.125 to 8,000 lux
	GainX48 Gain = 0x06 << alsContrGainShift // 48x nominal gain: ~0.02 to 1,300 lux
	GainX96 Gain = 0x07 << alsContrGainShift // 96x nominal gain: ~0.01 to 600 lux
)

var errInvalidGain = errors.New("ltr329: invalid gain")

func (g Gain) valid() bool {
	switch g {
	case GainX1, GainX2, GainX4, GainX8, GainX48, GainX96:
		return true
	}
	return false
}

// nominalMultiplier returns the ALS_GAIN constant the Lite-On lux formula
// (see lux.go) divides by for each gain setting.
func (g Gain) nominalMultiplier() int32 {
	switch g {
	case GainX2:
		return 2
	case GainX4:
		return 4
	case GainX8:
		return 8
	case GainX48:
		return 48
	case GainX96:
		return 96
	default:
		return 1
	}
}
