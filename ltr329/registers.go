package ltr329

// I2CAddress is the only I2C address the LTR-329ALS-01 answers on.
const I2CAddress uint16 = 0x29

// Register addresses, see the "Register Set" table in the datasheet.
const (
	regALSContr    uint8 = 0x80 // ALS operating mode, SW reset, gain
	regALSMeasRate uint8 = 0x85 // Integration time and measurement (repeat) rate
	regPartID      uint8 = 0x86 // Part number ID (7:4) and revision ID (3:0)
	regManufacID   uint8 = 0x87 // Manufacturer ID
	regALSDataCH1L uint8 = 0x88 // ALS CH1 (infrared-only) ADC, low byte
	regALSDataCH1H uint8 = 0x89 // ALS CH1 (infrared-only) ADC, high byte
	regALSDataCH0L uint8 = 0x8A // ALS CH0 (visible+infrared) ADC, low byte
	regALSDataCH0H uint8 = 0x8B // ALS CH0 (visible+infrared) ADC, high byte
	regALSStatus   uint8 = 0x8C // ALS data validity, gain-of-reading and new-data flag
)

// partNumberID is the fixed value bits 7:4 of PART_ID (0x86) always read
// back as, regardless of die revision (bits 3:0, which this driver does not
// check). The datasheet's own PART_ID register-diagram page states a
// default of 0x92 in its header, which is inconsistent both with the
// 1010b/0000b = 0xA0 the same page's bit-field table describes and with the
// 0xA0 reset value the datasheet's own register-summary table gives; 0xA0
// is also what this part reads back on the bench, so 0xA0 is what this
// driver trusts.
const partNumberID uint8 = 0x0A

// manufacturerID is the fixed value MANUFAC_ID (0x87) always reads back.
const manufacturerID uint8 = 0x05

// ALS_CONTR (0x80) bits.
const (
	alsContrModeActive uint8 = 1 << 0 // 0 = standby (default), 1 = active
	alsContrSWReset    uint8 = 1 << 1 // self-clearing; starts the initial startup procedure
	alsContrGainShift        = 2      // ALS_GAIN occupies bits 4:2
)

// ALS_STATUS (0x8C) bits.
const (
	statusNewData     uint8 = 1 << 2 // 1 = data not yet read since the last conversion
	statusGainShift         = 4      // gain of the current reading occupies bits 6:4
	statusDataInvalid uint8 = 1 << 7 // 1 = the latched reading is not usable
)
