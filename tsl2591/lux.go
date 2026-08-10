package tsl2591

// Lux calculation.
//
// This implements the coefficient-based formula AMS originally published
// for the TSL2591 and that Adafruit's actively maintained CircuitPython
// driver (adafruit_tsl2591) still uses:
//
//	cpl  = ATIME(ms) * AGAIN / 408
//	lux1 = (ch0 - 1.64*ch1) / cpl
//	lux2 = (0.59*ch0 - 0.86*ch1) / cpl
//	lux  = max(lux1, lux2)
//
// Adafruit's Arduino library (Adafruit_TSL2591_Library) ships a different,
// undocumented "alternate" formula instead (from a 2018 GitHub issue
// reporting negative lux values): lux = (ch0-ch1)*(1-ch1/ch0)/cpl. The two
// disagree, sometimes substantially, particularly at high IR fractions.
// This driver follows the coefficient-based formula, since it is both the
// one AMS itself published and the one still active in Adafruit's other,
// more recently touched driver.
//
// Whichever formula is used, this is a device-specific empirical fit to
// the human eye's response under AMS's test illuminants, not a physical
// law - AMS themselves describe the coefficients as "preliminary". Expect
// results within tens of percent of a reference lux meter under ordinary
// white/daylight-ish sources, and considerably worse under narrowband or
// unusual-spectrum light (e.g. some LEDs).
const luxDF = 408 // AMS "device factor"

// Coefficients from the formula above, scaled by 100 so AMS's published
// two-decimal constants (1.64, 0.59, 0.86) are exact in integer math.
const (
	coefB100 = 164
	coefC100 = 59
	coefD100 = 86
)

// calculateMilliLux converts raw channel counts to milliLux (1/1000 lux),
// given the integration time in milliseconds and the nominal gain
// multiplier. It returns 0 if both candidate lux values are non-positive,
// which the formula above can produce for very IR-heavy readings - a
// negative lux is not a meaningful measurement.
//
// It is not responsible for saturation: callers must check
// IntegrationTime.fullScaleCount() themselves before trusting the result.
func calculateMilliLux(ch0, ch1 uint16, integrationMs, gain int32) int32 {
	c0, c1 := int64(ch0), int64(ch1)

	// milliLux = 1000 * lux = 1000 * DF * (numerator/100) / (integrationMs * gain).
	// Folding the constant part of that (1000*DF/100) into a single factor
	// keeps the whole calculation in exact integers.
	const scale = 1000 * luxDF / 100 // = 4080

	n1 := scale * (100*c0 - coefB100*c1)
	n2 := scale * (coefC100*c0 - coefD100*c1)
	numerator := n1
	if n2 > numerator {
		numerator = n2
	}
	if numerator <= 0 {
		return 0
	}

	denominator := int64(integrationMs) * int64(gain)
	// Round to the nearest milliLux rather than truncating toward zero.
	return int32((numerator + denominator/2) / denominator)
}
