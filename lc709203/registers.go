package lc709203

// I2CAddress is the LC709203F's only I2C address; it is fixed and not
// configurable in hardware.
const I2CAddress uint16 = 0x0B

// Command codes, see Table 6 "Function of Registers" in the datasheet.
// Every one of these addresses a 16-bit word register, read or written two
// bytes at a time (low byte first) plus a CRC-8 byte; see io.go.
const (
	cmdBeforeRSOC          uint8 = 0x04
	cmdThermistorB         uint8 = 0x06
	cmdInitialRSOC         uint8 = 0x07
	cmdCellTemperature     uint8 = 0x08
	cmdCellVoltage         uint8 = 0x09
	cmdCurrentDirection    uint8 = 0x0A
	cmdAPA                 uint8 = 0x0B
	cmdAPT                 uint8 = 0x0C
	cmdRSOC                uint8 = 0x0D
	cmdITE                 uint8 = 0x0F
	cmdICVersion           uint8 = 0x11
	cmdChangeOfParameter   uint8 = 0x12
	cmdAlarmLowRSOC        uint8 = 0x13
	cmdAlarmLowCellVoltage uint8 = 0x14
	cmdPowerMode           uint8 = 0x15
	cmdStatusBit           uint8 = 0x16
	cmdNumberOfParameter   uint8 = 0x1A
)

// rsocInitMagic is the fixed value that must be written to Before RSOC
// (0x04) or Initial RSOC (0x07) to trigger recomputing RSOC from the
// battery's open-circuit voltage.
const rsocInitMagic uint16 = 0xAA55
