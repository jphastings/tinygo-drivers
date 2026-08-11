package seesaw

import "errors"

// PinMode selects how a GPIO pin behaves once configured with SetPinMode or SetPinsMode.
type PinMode uint8

const (
	// Input configures the pin as a high-impedance input with no pull resistor.
	Input PinMode = iota
	// Output configures the pin to drive its output latch (see WritePin / WritePins).
	Output
	// InputPullup configures the pin as an input with its pull resistor enabled, pulling high.
	InputPullup
	// InputPulldown configures the pin as an input with its pull resistor enabled, pulling low.
	//
	// AVR-based seesaw boards - every board with HW ID 0x87 and its siblings, i.e. the
	// ATtiny8x7/ATtinyx16 family this package targets - only implement a pull-UP resistor in
	// silicon (the megaAVR/tinyAVR PORT peripheral has a single PULLUPEN bit, no pull-down).
	// Requesting InputPulldown on that hardware still runs the documented two-step sequence
	// below, and the seesaw firmware accepts it without error, but the pin keeps reading high:
	// there is no pull-down to switch to. SAMD09-based seesaw boards do have a real pull-down,
	// so the mode is kept for those; just don't expect it to do anything on this family.
	InputPulldown
)

var (
	errInvalidPinMode = errors.New("seesaw: invalid pin mode")
	errPinOutOfRange  = errors.New("seesaw: pin must be 0-31")
)

// GPIOPins are the seesaw pin numbers broken out with selectable pull-up resistors on the
// Adafruit ATtiny1616 Breakout with seesaw (product ID 5690; the mechanically similar ATtiny816
// breakout, PID 5681, shares the same map).
//
// Source: https://learn.adafruit.com/adafruit-attiny817-seesaw/attinyx16-breakout-pinouts
// ("GPIO ... 0-5, 11, 14, 15, 16 - These are the 10 GPIO pins available with selectable pullup
// resistors"). It is provided for reference and for validating pin choices; SetPinMode/WritePin
// do not enforce it, since this Device type is shared by every seesaw board this package
// supports and a pin list hardcoded to one board would be wrong for the others.
//
// This board also drives three pins for purposes other than general GPIO, which this package
// deliberately leaves off the list and which should not be driven: pin 6 is a reserved interrupt
// output the firmware holds low, and pins 12/13 select I2C address bits - toggling them changes
// which address the board answers on, or worse, contends with whatever set them low in hardware.
// Pin 7 (PWM-only, see PWMPins) and pin 10 (an onboard status LED) are safe to drive but are not
// part of the general pulled-up GPIO set documented above.
var GPIOPins = [...]uint8{0, 1, 2, 3, 4, 5, 11, 14, 15, 16}

// SetPinMode configures a single GPIO pin.
func (d *Device) SetPinMode(pin uint8, mode PinMode) error {
	if pin > 31 {
		return errPinOutOfRange
	}
	return d.SetPinsMode(1<<uint32(pin), mode)
}

// SetPinsMode configures every pin set in pins (bit N selects seesaw pin N) at once.
//
// Enabling a pull resistor (InputPullup/InputPulldown) takes two seesaw operations, not one:
// PULLENSET only connects the resistor - its direction (pulling up vs pulling down) comes from
// the pin's output latch, the same latch WritePin/WritePins drive, which must be written while
// the pin is configured as an input. Skipping that write leaves the pull direction undefined
// (in practice: pulling down), which is a well known seesaw GPIO gotcha. Adafruit's reference
// implementation (Adafruit_seesaw.cpp, pinModeBulk) does exactly this: DIRCLR_BULK, then
// PULLENSET, then BULK_SET for INPUT_PULLUP or BULK_CLR for INPUT_PULLDOWN.
func (d *Device) SetPinsMode(pins uint32, mode PinMode) error {
	switch mode {
	case Output:
		return d.writeGpioMask(FunctionGpioDirsetBulk, pins)
	case Input:
		return d.writeGpioMask(FunctionGpioDirclrBulk, pins)
	case InputPullup:
		return d.setPullMode(pins, FunctionGpioBulkSet)
	case InputPulldown:
		return d.setPullMode(pins, FunctionGpioBulkClr)
	default:
		return errInvalidPinMode
	}
}

func (d *Device) setPullMode(pins uint32, latchOp FunctionAddress) error {
	if err := d.writeGpioMask(FunctionGpioDirclrBulk, pins); err != nil {
		return err
	}
	if err := d.writeGpioMask(FunctionGpioPullenset, pins); err != nil {
		return err
	}
	return d.writeGpioMask(latchOp, pins)
}

// WritePin sets the output latch of a single pin. It only affects pins configured as Output.
func (d *Device) WritePin(pin uint8, high bool) error {
	if pin > 31 {
		return errPinOutOfRange
	}
	return d.WritePins(1<<uint32(pin), high)
}

// WritePins sets the output latch of every pin set in pins (bit N selects seesaw pin N) at once.
func (d *Device) WritePins(pins uint32, high bool) error {
	if high {
		return d.writeGpioMask(FunctionGpioBulkSet, pins)
	}
	return d.writeGpioMask(FunctionGpioBulkClr, pins)
}

// TogglePins inverts the output latch of every pin set in pins.
func (d *Device) TogglePins(pins uint32) error {
	return d.writeGpioMask(FunctionGpioBulkToggle, pins)
}

// ReadPin returns the current level of a single pin, whether it's configured as an input or output.
func (d *Device) ReadPin(pin uint8) (bool, error) {
	if pin > 31 {
		return false, errPinOutOfRange
	}
	pins, err := d.ReadPins()
	if err != nil {
		return false, err
	}
	return pins&(1<<uint32(pin)) != 0, nil
}

// ReadPins returns the current level of every GPIO pin as a bitmask (bit N is seesaw pin N).
func (d *Device) ReadPins() (uint32, error) {
	var buf [4]byte
	if err := d.Read(ModuleGpioBase, FunctionGpioBulk, buf[:]); err != nil {
		return 0, err
	}
	return uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3]), nil
}

func (d *Device) writeGpioMask(function FunctionAddress, pins uint32) error {
	buf := [4]byte{byte(pins >> 24), byte(pins >> 16), byte(pins >> 8), byte(pins)}
	return d.Write(ModuleGpioBase, function, buf[:])
}
