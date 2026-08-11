package mpr121

// NumElectrodes is the number of independent capacitive electrodes the
// MPR121 supports (ELE0-ELE11).
const NumElectrodes uint8 = 12

// ElectrodeProximity is a pseudo-electrode index for the combined,
// 13th "proximity" channel (all enabled electrodes summed together for
// a larger sensing area and range). Its filtered data, baseline and
// threshold registers sit exactly where ELE12's would, so every
// per-electrode accessor in this package also accepts it.
const ElectrodeProximity uint8 = NumElectrodes

// ProximityMode selects which electrodes, if any, are combined into the
// 13th proximity channel (ECR's ELEPROX_EN field).
type ProximityMode uint8

const (
	// ProximityDisabled turns the proximity channel off. This is the
	// datasheet's power-on default.
	ProximityDisabled ProximityMode = iota
	// ProximityELE0To1 combines ELE0-ELE1.
	ProximityELE0To1
	// ProximityELE0To3 combines ELE0-ELE3.
	ProximityELE0To3
	// ProximityAll combines ELE0-ELE11.
	ProximityAll
)

// clLoadFromFirstReading is ECR's CL (Calibration Lock) field, 2b10:
// baseline tracking stays enabled, and the initial baseline is loaded
// from the first electrode reading rather than starting at zero.
//
// AN3944 (Freescale, 2010) itself configures CL = 2b00 - "baseline
// tracking enabled, initial baseline value is current value in baseline
// value register" (which POR/Reset has just zeroed). The current
// datasheet's ECR section documents that choice as leaving "a short
// period of no response to touch" immediately after Run Mode starts,
// while the baseline ramps up from zero, and recommends CL = 2b10
// instead to shorten it. Configure follows the datasheet here rather
// than AN3944's literal value; Adafruit's MPR121 library independently
// makes the same substitution.
const clLoadFromFirstReading uint8 = 0b10

// Config holds the configuration applied by Configure.
type Config struct {
	// ElectrodeCount enables ELE0..ELE(N-1) for touch detection, from 1
	// to NumElectrodes. The zero value enables all 12, AN3944's default.
	ElectrodeCount uint8

	// TouchThreshold and ReleaseThreshold set the same trip points, in
	// counts of delta below baseline, on every electrode; see
	// SetThresholds. The zero value for either applies AN3944's
	// recommended defaults, 15 and 10.
	TouchThreshold   uint8
	ReleaseThreshold uint8

	// Proximity enables and selects the combined proximity channel. The
	// zero value, ProximityDisabled, matches the power-on default.
	// Configure does not set proximity-specific thresholds; enabling
	// this without also calling SetElectrodeThresholds(ElectrodeProximity,
	// ...) leaves them at their power-on default of zero.
	Proximity ProximityMode
}

func (cfg Config) resolve() (electrodeCount, touch, release uint8, err error) {
	electrodeCount = cfg.ElectrodeCount
	if electrodeCount == 0 {
		electrodeCount = NumElectrodes
	}
	if electrodeCount > NumElectrodes {
		return 0, 0, 0, errInvalidElectrodeCount
	}

	touch = cfg.TouchThreshold
	if touch == 0 {
		touch = defaultTouchThreshold
	}
	release = cfg.ReleaseThreshold
	if release == 0 {
		release = defaultReleaseThreshold
	}
	if release >= touch {
		return 0, 0, 0, errReleaseNotBelowTouch
	}

	if cfg.Proximity > ProximityAll {
		return 0, 0, 0, errInvalidProximityMode
	}

	return electrodeCount, touch, release, nil
}
