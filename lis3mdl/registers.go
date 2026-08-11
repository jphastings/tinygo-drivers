package lis3mdl

// Register addresses, from the LIS3MDL datasheet Table 15 "Register address
// map". OFFSET_*_REG and INT_* registers exist but are not used by this
// driver.
const (
	regWhoAmI    = 0x0F
	regCtrlReg1  = 0x20
	regCtrlReg2  = 0x21
	regCtrlReg3  = 0x22
	regCtrlReg4  = 0x23
	regCtrlReg5  = 0x24
	regStatusReg = 0x27
	regOutXL     = 0x28
	regTempOutL  = 0x2E
)

// autoIncrement is the sub-address MSb (Datasheet section 5.1, "I2C serial
// interface"): "an 8-bit subaddress (SUB) is transmitted: the 7 LSb
// represent the actual register address while the MSb enables address
// autoincrement. If the MSb of the SUB field is 1, the SUB (register
// address) is automatically increased to allow multiple data read/write."
// Without it, every byte of a multi-byte transaction reads or writes the
// same register.
const autoIncrement = 0x80

// deviceID is the fixed value of WHO_AM_I (Table 16).
const deviceID = 0x3D

// AddressLow and AddressHigh are the two possible 7-bit I2C addresses,
// selected by the level of the SDO/SA1 pin (datasheet section 5.1). The
// Adafruit LSM6DS3TR-C + LIS3MDL breakout ties it low.
const (
	AddressLow  uint16 = 0x1C
	AddressHigh uint16 = 0x1E
)

// CTRL_REG1 (20h) bit positions (Table 17).
const (
	shiftTempEn  = 7
	shiftOM      = 5
	shiftDO      = 2
	shiftFastODR = 1
	shiftST      = 0
)

// CTRL_REG2 (21h) bit positions (Table 22).
const (
	shiftFS      = 5
	shiftReboot  = 3
	shiftSoftRst = 2
)

// CTRL_REG3 (22h) bit positions (Table 25).
const (
	shiftMD = 0
)

// CTRL_REG4 (23h) bit positions (Table 28).
const (
	shiftOMZ = 2
)

// CTRL_REG5 (24h) bit positions (Table 31).
const (
	shiftBDU = 6
)

// STATUS_REG (27h) bit positions (Table 33).
const (
	shiftZYXDA = 3
)
