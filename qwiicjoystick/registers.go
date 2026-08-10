package qwiicjoystick

// DefaultAddress is the factory-default 7-bit I2C address of the Qwiic
// Joystick. It can be changed at runtime with SetAddress.
const DefaultAddress uint8 = 0x20

// DeviceID is the value the REG_ID register reads on a Qwiic Joystick.
// Unlike qwiicbutton's 0x5D or qwiicbuzzer's 0x5E, this is not an
// arbitrary chip-identity byte: the firmware hard-codes REG_ID to its own
// compiled-in default I2C address (0x20) and, unlike REG_I2C_ADDRESS,
// never updates it when the live address is changed. Every board running
// this firmware reads 0x20 here, regardless of the address it currently
// answers on.
const DeviceID uint8 = 0x20

// Center is the resting value of a raw axis reading from RawPosition, the
// midpoint of its 0-1023 range. Position subtracts this to report each
// axis as a signed offset from rest.
const Center uint16 = 512

// Register addresses, taken from the register map in the SparkFun
// firmware:
//
//	https://github.com/sparkfun/Qwiic_Joystick/blob/master/Firmware/ATtiny85%20Firmware/Qwiic_Joystick_v26/Qwiic_Joystick_v26.ino
const (
	REG_ID             uint8 = 0x00
	REG_FIRMWARE_MAJOR uint8 = 0x01
	REG_FIRMWARE_MINOR uint8 = 0x02
	REG_X_MSB          uint8 = 0x03
	REG_X_LSB          uint8 = 0x04
	REG_Y_MSB          uint8 = 0x05
	REG_Y_LSB          uint8 = 0x06
	REG_BUTTON         uint8 = 0x07
	REG_STATUS         uint8 = 0x08
	REG_I2C_LOCK       uint8 = 0x09
	REG_I2C_ADDRESS    uint8 = 0x0A
)

// i2cUnlockValue must be written to REG_I2C_LOCK immediately before
// REG_I2C_ADDRESS, or the firmware leaves the address unchanged.
const i2cUnlockValue uint8 = 0x13

// statusHasBeenPressed is REG_STATUS's only defined bit. The firmware sets
// it the moment the button pin reads low following a change interrupt -
// i.e. on the press edge - despite its own source comment claiming the bit
// means the button was "released". See HasBeenPressed.
const statusHasBeenPressed uint8 = 1 << 0
