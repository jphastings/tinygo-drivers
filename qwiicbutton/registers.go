package qwiicbutton

// DefaultAddress is the factory-default 7-bit I2C address of the Qwiic
// Button. It can be changed at runtime via the REG_I2C_ADDRESS register.
const DefaultAddress uint8 = 0x6F

// DeviceID is the value the REG_ID register reads on a Qwiic Button.
const DeviceID uint8 = 0x5D

// Register addresses, taken from the register map in the SparkFun firmware:
// https://github.com/sparkfun/Qwiic_Button/blob/master/Firmware/Qwiic_Button/registers.h
const (
	REG_ID                    uint8 = 0x00
	REG_FIRMWARE_MINOR        uint8 = 0x01
	REG_FIRMWARE_MAJOR        uint8 = 0x02
	REG_BUTTON_STATUS         uint8 = 0x03
	REG_INTERRUPT_CONFIG      uint8 = 0x04
	REG_BUTTON_DEBOUNCE_TIME  uint8 = 0x05
	REG_PRESSED_QUEUE_STATUS  uint8 = 0x07
	REG_PRESSED_QUEUE_FRONT   uint8 = 0x08
	REG_PRESSED_QUEUE_BACK    uint8 = 0x0C
	REG_CLICKED_QUEUE_STATUS  uint8 = 0x10
	REG_CLICKED_QUEUE_FRONT   uint8 = 0x11
	REG_CLICKED_QUEUE_BACK    uint8 = 0x15
	REG_LED_BRIGHTNESS        uint8 = 0x19
	REG_LED_PULSE_GRANULARITY uint8 = 0x1A
	REG_LED_PULSE_CYCLE_TIME  uint8 = 0x1B
	REG_LED_PULSE_OFF_TIME    uint8 = 0x1D
	REG_I2C_ADDRESS           uint8 = 0x1F
)

// REG_BUTTON_STATUS bits
const (
	statusEventAvailable uint8 = 1 << 0
	statusHasBeenClicked uint8 = 1 << 1
	statusIsPressed      uint8 = 1 << 2
)
