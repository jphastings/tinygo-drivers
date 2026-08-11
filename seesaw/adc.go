package seesaw

import "errors"

// ADCPins are the seesaw pin numbers wired to the ADC on the Adafruit ATtiny1616 Breakout with
// seesaw (product ID 5690; the mechanically similar ATtiny816 breakout, PID 5681, shares the
// same map). Every reading is a 10-bit count (0-1023).
//
// Source: https://learn.adafruit.com/adafruit-attiny817-seesaw/attinyx16-breakout-pinouts
// ("ADC (10-bit) ... 0-5, 14, 15, 16 - There are nine 10-bit ADC pins"), which matches
// Adafruit_CircuitPython_seesaw's attinyx16.py pin map (analog_pins) exactly.
var ADCPins = [...]uint8{0, 1, 2, 3, 4, 5, 14, 15, 16}

var errNotADCPin = errors.New("seesaw: pin is not one of this board's ADC-capable pins")

// ReadADC reads the raw 10-bit ADC count (0-1023) seen on the given pin.
//
// On this hardware family (seesaw HW ID 0x87 and its siblings - ATtiny807/817/816/806/1616/1617,
// which covers this board) the ADC "channel" register offset the wire protocol expects is the
// seesaw pin number itself, not a separate small channel index: Adafruit_seesaw.cpp's
// analogRead() sets `p = pin` directly for exactly these hardware IDs, and
// Adafruit_CircuitPython_seesaw's seesaw.py mirrors it (`offset = pin` whenever chip_id !=
// SAMD09). Neither source contains a lookup table for this family - only the older, 4-channel
// SAMD09 seesaw maps a small set of ADC_INPUT_n pins onto channels 0-3 by array index. Getting
// this wrong silently reads a different, possibly unconnected, pin, so ReadADC rejects any pin
// outside ADCPins rather than guess.
func (d *Device) ReadADC(pin uint8) (uint16, error) {
	if !isADCPin(pin) {
		return 0, errNotADCPin
	}

	var buf [2]byte
	if err := d.Read(ModuleAdcBase, FunctionAdcChannelOffset+FunctionAddress(pin), buf[:]); err != nil {
		return 0, err
	}
	return uint16(buf[0])<<8 | uint16(buf[1]), nil
}

func isADCPin(pin uint8) bool {
	for _, p := range ADCPins {
		if p == pin {
			return true
		}
	}
	return false
}
