package mpr121

// AN3944's recommended thresholds (section C), applied by Configure
// when Config leaves TouchThreshold/ReleaseThreshold at zero.
const (
	defaultTouchThreshold   uint8 = 0x0F // 15
	defaultReleaseThreshold uint8 = 0x0A // 10
)

// SetThresholds applies the same touch and release trip points, each a
// count of delta below baseline (0-255), to all 12 electrodes. AN3944
// recommends 15/10 (Configure's default when Config leaves them zero);
// the datasheet suggests touch thresholds of roughly 4-16 work for most
// electrode shapes and sizes, with release set several counts below
// touch for hysteresis.
//
// release must be strictly less than touch: the datasheet defines
// touched as (baseline - filtered) > touch and released as
// (baseline - filtered) < release, so release >= touch would leave no
// delta at which a touched electrode is ever considered released again.
func (d *Device) SetThresholds(touch, release uint8) error {
	if release >= touch {
		return errReleaseNotBelowTouch
	}
	return d.withStopMode(func() error {
		for e := uint8(0); e < NumElectrodes; e++ {
			if err := d.setElectrodeThresholdsRaw(e, touch, release); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetElectrodeThresholds applies touch and release trip points to a
// single electrode (0-11), or via ElectrodeProximity to the combined
// proximity channel; see SetThresholds.
func (d *Device) SetElectrodeThresholds(electrode, touch, release uint8) error {
	if electrode > ElectrodeProximity {
		return errInvalidElectrode
	}
	if release >= touch {
		return errReleaseNotBelowTouch
	}
	return d.withStopMode(func() error {
		return d.setElectrodeThresholdsRaw(electrode, touch, release)
	})
}

func (d *Device) setElectrodeThresholdsRaw(electrode, touch, release uint8) error {
	if err := d.writeRegister(touchThresholdReg(electrode), touch); err != nil {
		return err
	}
	return d.writeRegister(releaseThresholdReg(electrode), release)
}

// SetDebounce sets how many consecutive samples must agree before a
// touch or release is latched into the status registers read by
// Touched/IsTouched (0-7 each; the datasheet's own reset default is 0,
// no debounce).
func (d *Device) SetDebounce(touchSamples, releaseSamples uint8) error {
	if touchSamples > 7 || releaseSamples > 7 {
		return errInvalidDebounce
	}
	value := releaseSamples<<4 | touchSamples
	return d.withStopMode(func() error {
		return d.writeRegister(regDebounce, value)
	})
}
