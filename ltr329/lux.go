package ltr329

// Lux calculation.
//
// Lite-On publish this separately from the main datasheet, in "LTR-303ALS &
// LTR-329ALS Appendix A" (Rev1.0, 22 Feb 2013). It is a three-band
// piecewise-linear fit of the CH0 (visible+IR) and CH1 (IR-only) counts to
// lux, selected by the ratio CH1/(CH0+CH1):
//
//	ratio < 0.45:         lux = (1.7743*ch0 + 1.1059*ch1) / gain / intFactor
//	0.45 <= ratio < 0.64: lux = (4.2785*ch0 - 1.9548*ch1) / gain / intFactor
//	0.64 <= ratio < 0.85: lux = (0.5926*ch0 + 0.1185*ch1) / gain / intFactor
//	ratio >= 0.85:        lux = 0
//
// where intFactor is the integration time in units of 100ms (1.0 at the
// 100ms default, 0.5 at 50ms, and so on). ESPHome's independently written
// ltr_als_ps component uses these identical thresholds and coefficients,
// which is the closest thing available to a second source for them.
//
// As with the TSL2591's coefficients (see tsl2591/lux.go), these are
// Lite-On's own empirical fit against a reference meter under their test
// illuminants, not a physical derivation - expect the usual tens-of-percent
// spread against a real lux meter, and worse under narrow-band light
// sources such as many LEDs.
func calculateMilliLux(ch0, ch1 uint16, gain, integrationMs int32) int32 {
	c0, c1 := int64(ch0), int64(ch1)

	// Coefficients scaled by 10,000 so Lite-On's four-decimal-place
	// constants are exact in integer math. Comparing ratio thresholds by
	// cross-multiplication (100*c1 vs N*(c0+c1)) avoids a fractional
	// ratio entirely, including at c0=c1=0, which falls through every
	// band and correctly reports 0 lux for a dark reading.
	var coefA, coefB int64
	sum := c0 + c1
	switch {
	case 100*c1 < 45*sum:
		coefA, coefB = 17743, 11059
	case 100*c1 < 64*sum:
		coefA, coefB = 42785, -19548
	case 100*c1 < 85*sum:
		coefA, coefB = 5926, 1185
	default:
		return 0
	}

	numerator := coefA*c0 + coefB*c1
	if numerator <= 0 {
		// The middle band's negative coefficient can drive this
		// negative near its own boundary; a negative lux isn't a
		// meaningful measurement.
		return 0
	}

	// milliLux = 1000 * lux = 1000 * (numerator/10000) / gain / (integrationMs/100)
	//          = numerator * 10 / (gain * integrationMs)
	denominator := int64(gain) * int64(integrationMs)
	return int32((numerator*10 + denominator/2) / denominator)
}
