package ltr329

import "errors"

// MeasurementRate selects how often the ALS_DATA registers are refreshed in
// active mode - the interval between conversions, not the duration of any
// one of them (that's IntegrationTime). The datasheet requires
// IntegrationTime <= MeasurementRate and says the device silently clamps
// the integration time down to match if it is not; Configure rejects that
// combination instead, since a silently-shortened integration time would
// otherwise change the caller's dynamic range without any error to say so.
//
// Unlike Gain and IntegrationTime, these constants are deliberately NOT the
// raw ALS_MEAS_RATE[2:0] register bits: they're reordered so the zero
// value, MeasurementRate500ms, is both the value ALS_MEAS_RATE itself
// powers up with and the only measurement rate compatible with the zero
// value of IntegrationTime (100ms) - so a zero-value Config is always a
// valid, hardware-default configuration. registerBits does the
// translation to the real register encoding.
type MeasurementRate uint8

const (
	MeasurementRate500ms MeasurementRate = iota // default
	MeasurementRate50ms
	MeasurementRate100ms
	MeasurementRate200ms
	MeasurementRate1000ms
	MeasurementRate2000ms
)

var errInvalidMeasurementRate = errors.New("ltr329: invalid measurement rate")

func (r MeasurementRate) valid() bool {
	return r <= MeasurementRate2000ms
}

func (r MeasurementRate) milliseconds() int32 {
	switch r {
	case MeasurementRate50ms:
		return 50
	case MeasurementRate100ms:
		return 100
	case MeasurementRate200ms:
		return 200
	case MeasurementRate1000ms:
		return 1000
	case MeasurementRate2000ms:
		return 2000
	default: // MeasurementRate500ms
		return 500
	}
}

// registerBits returns this measurement rate's ALS_MEAS_RATE[2:0] encoding.
// Register codes 0b101 and 0b111 are undocumented duplicates of 0b110
// (2000ms, per the datasheet's own table and confirmed by Adafruit's
// CircuitPython driver); this driver only ever writes the documented code.
func (r MeasurementRate) registerBits() uint8 {
	switch r {
	case MeasurementRate50ms:
		return 0x00
	case MeasurementRate100ms:
		return 0x01
	case MeasurementRate200ms:
		return 0x02
	case MeasurementRate1000ms:
		return 0x04
	case MeasurementRate2000ms:
		return 0x06
	default: // MeasurementRate500ms
		return 0x03
	}
}
