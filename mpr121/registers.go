package mpr121

// Register addresses, see "Table 2. Register Map" in the datasheet.
// Filtered data, baseline and threshold registers repeat per electrode;
// see filteredDataReg, baselineReg and touchThresholdReg/releaseThresholdReg
// in io.go rather than addressing electrodes 1-11 (or the proximity
// channel) directly from here.
const (
	regTouchStatusL uint8 = 0x00 // ELE0-ELE7 touch status
	regTouchStatusH uint8 = 0x01 // ELE8-ELE11 (D3:D0), ELEPROX (D4), OVCF (D7)

	regFilteredData0 uint8 = 0x04 // ELE0 filtered data, LSB then MSB; +2 per electrode/proximity through 0x1D
	regBaseline0     uint8 = 0x1E // ELE0 baseline; +1 per electrode/proximity through 0x2A

	regMHDRising  uint8 = 0x2B
	regNHDRising  uint8 = 0x2C
	regNCLRising  uint8 = 0x2D
	regFDLRising  uint8 = 0x2E
	regMHDFalling uint8 = 0x2F
	regNHDFalling uint8 = 0x30
	regNCLFalling uint8 = 0x31
	regFDLFalling uint8 = 0x32
	regNHDTouched uint8 = 0x33
	regNCLTouched uint8 = 0x34
	regFDLTouched uint8 = 0x35

	regTouchThreshold0 uint8 = 0x41 // ELE0 touch threshold, release follows at +1; +2 per electrode/proximity through 0x5A

	regDebounce     uint8 = 0x5B
	regAFEConfig1   uint8 = 0x5C // CDC (charge current) and FFI; power-on default 0x10
	regFilterConfig uint8 = 0x5D // CDT (charge time), SFI and ESI; power-on default 0x24
	regECR          uint8 = 0x5E // Electrode Configuration Register: CL, ELEPROX_EN, ELE_EN

	regSoftReset uint8 = 0x80
)

// Power-on/soft-reset values. Every register clears to 0x00 on reset
// except these two - see Reset.
const (
	afeConfig1PowerOnDefault   uint8 = 0x10
	filterConfigPowerOnDefault uint8 = 0x24
)

// softResetMagic is the only value that register 0x80 accepts as a
// soft-reset command.
const softResetMagic uint8 = 0x63

// Touch status register bits.
const (
	// overCurrentWriteBit, written to regTouchStatusH, clears a latched
	// over-current fault (OVCF). All other bits in that register ignore
	// writes.
	overCurrentWriteBit uint8 = 1 << 7

	// overCurrentStatusBit is OVCF's position once regTouchStatusL and
	// regTouchStatusH are read together as one little-endian uint16.
	overCurrentStatusBit uint16 = 1 << 15
)
