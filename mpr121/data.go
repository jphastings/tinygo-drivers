package mpr121

import "encoding/binary"

// FilteredData returns the internal second-stage filter output for a
// single electrode (0-11), or via ElectrodeProximity the combined
// proximity channel: a 10-bit value, inversely proportional to
// capacitance, that Baseline is compared against to detect a touch.
func (d *Device) FilteredData(electrode uint8) (uint16, error) {
	if electrode > ElectrodeProximity {
		return 0, errInvalidElectrode
	}
	if err := d.readRegisters(filteredDataReg(electrode), d.rbuf[:2]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(d.rbuf[:2]) & 0x03FF, nil
}

// Baseline returns an electrode's slow-moving capacitance baseline
// (0-11, or ElectrodeProximity), in the same 10-bit units as
// FilteredData so the two can be compared directly: touched is
// (baseline - filtered) > touch threshold.
//
// The register only stores the upper 8 bits of the internal 10-bit
// baseline - the low 2 bits always read back as zero. Comparing the raw
// register byte directly against FilteredData therefore makes the
// baseline look about 4x smaller than it really is, which reads like a
// badly out-of-calibration (or dead) sensor rather than the register
// format it actually is. Baseline corrects for this by shifting the
// register value left 2 bits before returning it.
func (d *Device) Baseline(electrode uint8) (uint16, error) {
	if electrode > ElectrodeProximity {
		return 0, errInvalidElectrode
	}
	raw, err := d.readRegister(baselineReg(electrode))
	if err != nil {
		return 0, err
	}
	return uint16(raw) << 2, nil
}
