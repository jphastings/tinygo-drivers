package seesaw

import (
	"testing"

	"github.com/frankban/quicktest"
)

func TestDevice_SetPWM(t *testing.T) {
	mocked := newMockDev(t, 0x49,
		when([]byte{0x08, 0x01, 0x01, 0xBE, 0xEF}, nil, nil),
	)

	err := New(mocked).SetPWM(1, 0xBEEF)
	quicktest.New(t).Assert(err, quicktest.IsNil)
}

func TestDevice_SetPWM_invalidPin(t *testing.T) {
	mocked := newMockDev(t, 0x49) // no handlers: any I2C transaction fails the test

	err := New(mocked).SetPWM(2, 0xFFFF) // pin 2 is not one of this board's PWM pins
	quicktest.New(t).Assert(err, quicktest.Equals, errNotPWMPin)
}

func TestDevice_SetPWMFrequency(t *testing.T) {
	mocked := newMockDev(t, 0x49,
		when([]byte{0x08, 0x02, 0x01, 0x03, 0xE8}, nil, nil),
	)

	err := New(mocked).SetPWMFrequency(1, 1000)
	quicktest.New(t).Assert(err, quicktest.IsNil)
}

func TestDevice_SetPWMFrequency_invalidPin(t *testing.T) {
	mocked := newMockDev(t, 0x49) // no handlers: any I2C transaction fails the test

	err := New(mocked).SetPWMFrequency(6, 1000) // pin 6 is a reserved interrupt pin
	quicktest.New(t).Assert(err, quicktest.Equals, errNotPWMPin)
}
