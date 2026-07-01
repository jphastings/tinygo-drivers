package mmc5983

import "errors"

// controlRegisterShadow mirrors the contents of the chip's four write-only
// control registers, so bits can be changed without read-modify-write.
type controlRegisterShadow struct {
	internalControl0 uint8
	internalControl1 uint8
	internalControl2 uint8
	internalControl3 uint8
}

type operation uint8

const (
	opClear operation = 0b00
	opSet   operation = 0b01
	opWrite operation = 0b10
)

var errRegisterNotShadowed = errors.New("mmc5983: register is not shadowed")

// operateWithShadow sets (opSet) or clears (opClear) the bits of bitMask in
// the shadow copy of a control register, and, when opWrite is or'ed in,
// writes the whole updated register to the chip. If the write fails the
// shadow is rolled back.
func (d *Device) operateWithShadow(registerAddr uint8, op operation, bitMask uint8) error {
	shadowRegister := d.shadowFor(registerAddr)
	if shadowRegister == nil {
		return errRegisterNotShadowed
	}

	previous := *shadowRegister

	if (op & opSet) == opSet {
		*shadowRegister |= bitMask
	} else {
		*shadowRegister = *shadowRegister &^ bitMask
	}

	if (op & opWrite) == opWrite {
		d.wbuf[0] = registerAddr
		d.wbuf[1] = *shadowRegister
		if err := d.bus.Tx(d.Address, d.wbuf[:2], nil); err != nil {
			*shadowRegister = previous
			return err
		}
	}

	return nil
}

func (d *Device) isShadowBitSet(registerAddr uint8, bitMask uint8) bool {
	shadowRegister := d.shadowFor(registerAddr)
	if shadowRegister == nil {
		return false
	}
	return (*shadowRegister & bitMask) != 0
}

func (d *Device) shadowFor(registerAddr uint8) *uint8 {
	switch registerAddr {
	case INT_CTRL_0_REG:
		return &d.memoryShadow.internalControl0
	case INT_CTRL_1_REG:
		return &d.memoryShadow.internalControl1
	case INT_CTRL_2_REG:
		return &d.memoryShadow.internalControl2
	case INT_CTRL_3_REG:
		return &d.memoryShadow.internalControl3
	default:
		return nil
	}
}
