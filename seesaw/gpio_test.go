package seesaw

import (
	"testing"

	"github.com/frankban/quicktest"
)

func TestDevice_SetPinMode_output(t *testing.T) {
	mocked := newMockDev(t, 0x49,
		when([]byte{0x01, 0x02, 0x00, 0x00, 0x00, 0x08}, nil, nil),
	)

	err := New(mocked).SetPinMode(3, Output)
	quicktest.New(t).Assert(err, quicktest.IsNil)
}

func TestDevice_SetPinMode_input(t *testing.T) {
	mocked := newMockDev(t, 0x49,
		when([]byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x08}, nil, nil),
	)

	err := New(mocked).SetPinMode(3, Input)
	quicktest.New(t).Assert(err, quicktest.IsNil)
}

// This is the classic seesaw GPIO gotcha: enabling a pull resistor is not one register write.
// PULLENSET only connects the resistor - its direction comes from the output latch (BULK_SET /
// BULK_CLR), which must be written while the pin is an input. A driver that skips the final
// latch write, or writes it before switching to input, ends up with the wrong pull direction (or
// none at all). This matches Adafruit_seesaw.cpp's pinModeBulk() exactly: DIRCLR_BULK, then
// PULLENSET, then BULK_SET for pull-up.
func TestDevice_SetPinMode_inputPullup_isThreeSeparateWrites(t *testing.T) {
	mocked := newMockDev(t, 0x49,
		when([]byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x20}, nil, nil), // DIRCLR_BULK: become an input
		when([]byte{0x01, 0x0B, 0x00, 0x00, 0x00, 0x20}, nil, nil), // PULLENSET: connect the resistor
		when([]byte{0x01, 0x05, 0x00, 0x00, 0x00, 0x20}, nil, nil), // BULK_SET: ...pulling high
	)

	err := New(mocked).SetPinMode(5, InputPullup)
	quicktest.New(t).Assert(err, quicktest.IsNil)
}

// Same three-step sequence, but the final latch write clears instead of sets, asking for a
// pull-down. On the AVR-based boards this package targets that request is accepted but has no
// effect in silicon (see the InputPulldown doc comment) - this test only pins down the bytes this
// driver puts on the wire, which match Adafruit's reference implementation.
func TestDevice_SetPinMode_inputPulldown_isThreeSeparateWrites(t *testing.T) {
	mocked := newMockDev(t, 0x49,
		when([]byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x20}, nil, nil), // DIRCLR_BULK: become an input
		when([]byte{0x01, 0x0B, 0x00, 0x00, 0x00, 0x20}, nil, nil), // PULLENSET: connect the resistor
		when([]byte{0x01, 0x06, 0x00, 0x00, 0x00, 0x20}, nil, nil), // BULK_CLR: ...pulling low
	)

	err := New(mocked).SetPinMode(5, InputPulldown)
	quicktest.New(t).Assert(err, quicktest.IsNil)
}

func TestDevice_SetPinMode_invalidMode(t *testing.T) {
	mocked := newMockDev(t, 0x49) // no handlers: any I2C transaction fails the test

	err := New(mocked).SetPinMode(0, PinMode(99))
	quicktest.New(t).Assert(err, quicktest.Equals, errInvalidPinMode)
}

func TestDevice_SetPinMode_pinOutOfRange(t *testing.T) {
	mocked := newMockDev(t, 0x49) // no handlers: any I2C transaction fails the test

	err := New(mocked).SetPinMode(32, Output)
	quicktest.New(t).Assert(err, quicktest.Equals, errPinOutOfRange)
}

func TestDevice_WritePin(t *testing.T) {
	qt := quicktest.New(t)

	high := newMockDev(t, 0x49, when([]byte{0x01, 0x05, 0x00, 0x00, 0x00, 0x04}, nil, nil))
	qt.Assert(New(high).WritePin(2, true), quicktest.IsNil)

	low := newMockDev(t, 0x49, when([]byte{0x01, 0x06, 0x00, 0x00, 0x00, 0x04}, nil, nil))
	qt.Assert(New(low).WritePin(2, false), quicktest.IsNil)
}

func TestDevice_TogglePins(t *testing.T) {
	mocked := newMockDev(t, 0x49,
		when([]byte{0x01, 0x07, 0x00, 0x00, 0x00, 0x0F}, nil, nil),
	)

	err := New(mocked).TogglePins(0x0F)
	quicktest.New(t).Assert(err, quicktest.IsNil)
}

// The bulk register is a 32-bit big-endian mask, one bit per seesaw pin. This exercises pins on
// both sides of a byte boundary (0 and 20) to catch endianness and off-by-one bit-position bugs.
func TestDevice_ReadPins_decodesBigEndianMask(t *testing.T) {
	mocked := newMockDev(t, 0x49,
		when([]byte{0x01, 0x04}, nil, nil),
		when(nil, []byte{0x00, 0x10, 0x00, 0x01}, nil), // pins 0 and 20 set, everything else clear
	)

	sut := New(mocked)
	sut.ReadDelay = 0

	pins, err := sut.ReadPins()
	qt := quicktest.New(t)
	qt.Assert(err, quicktest.IsNil)
	qt.Assert(pins, quicktest.Equals, uint32(0x00100001))
}

func TestDevice_ReadPin(t *testing.T) {
	cases := []struct {
		name string
		pin  uint8
		buf  []byte
		want bool
	}{
		{"set bit in low byte", 0, []byte{0x00, 0x00, 0x00, 0x01}, true},
		{"clear bit in low byte", 1, []byte{0x00, 0x00, 0x00, 0x01}, false},
		{"set bit in high byte", 20, []byte{0x00, 0x10, 0x00, 0x00}, true},
		{"neighbouring bit stays clear", 21, []byte{0x00, 0x10, 0x00, 0x00}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mocked := newMockDev(t, 0x49,
				when([]byte{0x01, 0x04}, nil, nil),
				when(nil, c.buf, nil),
			)
			sut := New(mocked)
			sut.ReadDelay = 0

			got, err := sut.ReadPin(c.pin)
			qt := quicktest.New(t)
			qt.Assert(err, quicktest.IsNil)
			qt.Assert(got, quicktest.Equals, c.want)
		})
	}
}
