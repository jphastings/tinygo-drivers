package seesaw

import "errors"

// PWMPins are the seesaw pin numbers capable of PWM output on the Adafruit ATtiny1616 Breakout
// with seesaw (product ID 5690; the mechanically similar ATtiny816 breakout, PID 5681, shares the
// same map).
//
// Source: https://learn.adafruit.com/adafruit-attiny817-seesaw/attinyx16-breakout-pinouts
// ("PWM (8-bit) ... 0, 1, 7, 11, 16 - There are five 8-bit PWM output pins"). Note pin 7 is
// PWM-only - it isn't in GPIOPins.
//
// Adafruit_CircuitPython_seesaw's attinyx16.py pin map (also keyed to PID 5690/5681) lists three
// additional pins, 4/5/6, as a second "16-bit PWM mode" group. We deliberately do not include
// them: pin 6 is documented elsewhere on this same board as a reserved interrupt-output pin the
// firmware drives low (confirmed on real hardware - it reads low and must not be driven), which
// directly contradicts the CircuitPython pin map's claim for that pin. Given that contradiction,
// we trust the board's own pinout page over what looks like a stale or overly generic library
// pin map, and only expose the five pins Adafruit documents for this product.
var PWMPins = [...]uint8{0, 1, 7, 11, 16}

var errNotPWMPin = errors.New("seesaw: pin is not one of this board's PWM-capable pins")

// SetPWM sets the duty cycle of a PWM-capable pin to value/65535.
//
// The wire protocol always carries a 16-bit duty cycle here: Adafruit_seesaw.cpp's analogWrite()
// only ever sends the compact 8-bit form when explicitly asked for a non-16 width, and
// Adafruit_CircuitPython_seesaw's attinyx16.py pin map sets pwm_width=16 for this board, so its
// analog_write() always packs the full 16-bit value too. Adafruit's own product page calls these
// "8-bit PWM output pins" - that most likely describes the timer hardware's effective resolution
// behind the scenes, not the wire format, so don't expect all 65536 steps to be distinguishable.
func (d *Device) SetPWM(pin uint8, value uint16) error {
	if !isPWMPin(pin) {
		return errNotPWMPin
	}
	buf := [3]byte{pin, byte(value >> 8), byte(value)}
	return d.Write(ModuleTimerBase, FunctionTimerPwm, buf[:])
}

// SetPWMFrequency sets the PWM frequency, in Hz, of a PWM-capable pin. Adafruit's documentation
// notes that on this chip family PWM pins sharing a timer also share a frequency, so changing one
// pin's frequency can affect its neighbours.
func (d *Device) SetPWMFrequency(pin uint8, freqHz uint16) error {
	if !isPWMPin(pin) {
		return errNotPWMPin
	}
	buf := [3]byte{pin, byte(freqHz >> 8), byte(freqHz)}
	return d.Write(ModuleTimerBase, FunctionTimerFreq, buf[:])
}

func isPWMPin(pin uint8) bool {
	for _, p := range PWMPins {
		if p == pin {
			return true
		}
	}
	return false
}
