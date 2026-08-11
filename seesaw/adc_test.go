package seesaw

import (
	"testing"

	"github.com/frankban/quicktest"
)

// On this hardware family the ADC "channel" register offset is the seesaw pin number itself, not
// a separate index - so reading pin 4 must land on FunctionAdcChannelOffset+4, not some other
// small channel number. Getting this wrong silently reads a different pin with no error.
func TestDevice_ReadADC(t *testing.T) {
	mocked := newMockDev(t, 0x49,
		when([]byte{0x09, 0x0B}, nil, nil), // ADC base, channel offset (0x07) + pin 4
		when(nil, []byte{0x03, 0xFF}, nil),
	)

	sut := New(mocked)
	sut.ReadDelay = 0

	value, err := sut.ReadADC(4)
	qt := quicktest.New(t)
	qt.Assert(err, quicktest.IsNil)
	qt.Assert(value, quicktest.Equals, uint16(1023))
}

func TestDevice_ReadADC_highChannel(t *testing.T) {
	mocked := newMockDev(t, 0x49,
		when([]byte{0x09, 0x17}, nil, nil), // channel offset (0x07) + pin 16
		when(nil, []byte{0x00, 0x00}, nil),
	)

	sut := New(mocked)
	sut.ReadDelay = 0

	value, err := sut.ReadADC(16)
	qt := quicktest.New(t)
	qt.Assert(err, quicktest.IsNil)
	qt.Assert(value, quicktest.Equals, uint16(0))
}

func TestDevice_ReadADC_invalidPin(t *testing.T) {
	mocked := newMockDev(t, 0x49) // no handlers: any I2C transaction fails the test

	_, err := New(mocked).ReadADC(6) // pin 6 is a reserved interrupt pin, not an ADC input
	quicktest.New(t).Assert(err, quicktest.Equals, errNotADCPin)
}
