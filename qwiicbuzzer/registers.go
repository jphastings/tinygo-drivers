package qwiicbuzzer

// DefaultAddress is the factory-default 7-bit I2C address of the Qwiic
// Buzzer. It can be changed at runtime with SetAddress.
const DefaultAddress uint8 = 0x34

// DeviceID is the value the REG_ID register reads on a Qwiic Buzzer.
const DeviceID uint8 = 0x5E

// ResonantFrequency is the onboard piezo element's natural resonant
// frequency in Hz, where it sounds loudest. The firmware itself defaults to
// this tone after a factory reset.
const ResonantFrequency uint16 = 2730

// Register addresses, taken from the register map in the SparkFun firmware:
// https://github.com/sparkfun/SparkFun_Qwiic_Buzzer/blob/main/Firmware/QwiicBuzzerFirmware/sfeQwiicBuzzerFirmware.h
const (
	REG_ID                 uint8 = 0x00
	REG_FIRMWARE_MINOR     uint8 = 0x01
	REG_FIRMWARE_MAJOR     uint8 = 0x02
	REG_TONE_FREQUENCY_MSB uint8 = 0x03
	REG_TONE_FREQUENCY_LSB uint8 = 0x04
	REG_VOLUME             uint8 = 0x05
	REG_DURATION_MSB       uint8 = 0x06
	REG_DURATION_LSB       uint8 = 0x07
	REG_ACTIVE             uint8 = 0x08
	REG_SAVE_SETTINGS      uint8 = 0x09
	REG_I2C_ADDRESS        uint8 = 0x0A
)

// Volume selects one of the buzzer's four discrete loudness settings. Each
// step is switched by its own transistor rather than a PWM duty cycle.
// VolumeOff mutes the buzzer even while it is otherwise active.
type Volume uint8

const (
	VolumeOff Volume = 0
	VolumeMin Volume = 1
	VolumeLow Volume = 2
	VolumeMid Volume = 3
	VolumeMax Volume = 4
)

// valid reports whether v is one of the four levels the firmware switches.
// Out-of-range values (5-7 fit in the register's 3 protected bits) aren't
// rejected by the firmware, they just leave every volume transistor off, so
// this driver rejects them itself rather than passing through a silently
// muted "success".
func (v Volume) valid() bool {
	return v <= VolumeMax
}
