package is31fl3741

// Registers. Names taken from the datasheet:
// https://www.lumissil.com/assets/pdf/core/IS31FL3741_DS.pdf
const (
	// ADDR pin connected to GND
	I2C_ADDRESS_GND uint8 = 0x30
	// ADDR pin connected to SCL
	I2C_ADDRESS_SCL uint8 = 0x31
	// ADDR pin connected to SDA
	I2C_ADDRESS_SDA uint8 = 0x32
	// ADDR pin connected to VCC
	I2C_ADDRESS_VCC uint8 = 0x33

	// ID register, reads back the I2C address shifted left by one bit
	ID_REGISTER uint8 = 0xFC

	// Main command register, selects the currently active page
	COMMAND uint8 = 0xFD

	// The command register locks itself after every write; writing
	// COMMAND_UNLOCK to this register unlocks it for a single write
	COMMAND_WRITE_LOCK uint8 = 0xFE
	COMMAND_UNLOCK     uint8 = 0xC5

	// Pages selectable via the command register
	PAGE_PWM_1     uint8 = 0x00 // PWM values for LED channels 0-179
	PAGE_PWM_2     uint8 = 0x01 // PWM values for LED channels 180-350
	PAGE_SCALING_1 uint8 = 0x02 // Current scaling for LED channels 0-179
	PAGE_SCALING_2 uint8 = 0x03 // Current scaling for LED channels 180-350
	PAGE_FUNCTION  uint8 = 0x04 // Function registers

	// Function registers (on PAGE_FUNCTION):
	FUNC_CONFIGURATION  uint8 = 0x00
	FUNC_GLOBAL_CURRENT uint8 = 0x01
	FUNC_RESET          uint8 = 0x3F

	// Configuration register values: software shutdown (LEDs off, registers
	// kept) and normal operation with all 9 switch lines active
	CONFIGURATION_SHUTDOWN uint8 = 0x00
	CONFIGURATION_NORMAL   uint8 = 0x01

	// Writing this value to FUNC_RESET restores all registers to their
	// default values
	RESET_TRIGGER uint8 = 0xAE
)

// LEDCount is the total number of LED (PWM) channels the chip drives:
// 39 current sinks x 9 switch lines
const LEDCount = 351

// Number of LED channels on the first PWM/scaling page; the remaining
// channels live on the second page
const firstPageLEDCount = 180

// Sentinel for "currently selected page unknown"
const pageUnknown uint8 = 0xFF
