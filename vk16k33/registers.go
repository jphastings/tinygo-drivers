package vk16k33

// DefaultAddress is the 7-bit I2C address of the display with both address
// jumpers open. The board's jumpers select 0x70-0x73; the VK16K33 itself
// supports addresses up to 0x77.
const DefaultAddress uint16 = 0x70

// Commands, from the VK16K33 datasheet:
// https://www.szvinka.com/uploadfile/Datasheet/LED/VK16K33/VK16K33_V1.2-EN.pdf
const (
	// CMD_SYSTEM_SETUP bit 0 turns the system oscillator on (normal mode)
	// or off (standby mode)
	CMD_SYSTEM_SETUP uint8 = 0x20

	// CMD_DISPLAY_SETUP bits 2:1 select the blink rate, bit 0 turns the
	// display on or off
	CMD_DISPLAY_SETUP uint8 = 0x80

	// CMD_DIMMING_SETUP bits 3:0 select the display brightness as a pulse
	// width of (duty+1)/16
	CMD_DIMMING_SETUP uint8 = 0xE0
)

// BlinkRate is a display blink frequency, set with SetBlink
type BlinkRate uint8

// Blink rates from the display setup command in the datasheet
const (
	BLINK_OFF   BlinkRate = 0b00
	BLINK_2HZ   BlinkRate = 0b01
	BLINK_1HZ   BlinkRate = 0b10
	BLINK_0_5HZ BlinkRate = 0b11
)
