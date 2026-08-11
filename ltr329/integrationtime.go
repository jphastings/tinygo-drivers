package ltr329

import "errors"

// IntegrationTime selects how long a single ALS conversion integrates the
// photodiode current for. Its value maps directly onto the
// ALS_INT_TIME[2:0] bits (5:3) of the ALS_MEAS_RATE register - a
// non-monotonic encoding (100ms is code 0, but the shortest time, 50ms, is
// code 1) that this driver preserves rather than reordering, so the
// register field can always be read straight off the constant.
type IntegrationTime uint8

const (
	IntegrationTime100ms IntegrationTime = iota // default
	IntegrationTime50ms
	IntegrationTime200ms
	IntegrationTime400ms
	IntegrationTime150ms
	IntegrationTime250ms
	IntegrationTime300ms
	IntegrationTime350ms
)

var errInvalidIntegrationTime = errors.New("ltr329: invalid integration time")

func (t IntegrationTime) valid() bool {
	return t <= IntegrationTime350ms
}

// milliseconds returns the integration time in milliseconds, used both by
// the lux calculation and to check IntegrationTime against MeasurementRate.
func (t IntegrationTime) milliseconds() int32 {
	return [...]int32{100, 50, 200, 400, 150, 250, 300, 350}[t]
}

// registerBits returns this integration time already shifted into place
// for OR-ing into ALS_MEAS_RATE alongside a MeasurementRate.
func (t IntegrationTime) registerBits() uint8 {
	const alsMeasRateIntShift = 3
	return uint8(t) << alsMeasRateIntShift
}
