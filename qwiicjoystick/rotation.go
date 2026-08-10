package qwiicjoystick

// Rotation compensates Position for how the board is physically mounted: set
// it to the angle, clockwise in 90 degree steps, that the board was turned
// away from its reference orientation (silkscreen text upright) before it
// was installed, and Position corrects for that turn on top of the axis
// normalization it already applies - so a push in a given physical
// direction is reported the same way no matter which of the four ways the
// board went in, once Rotation is set to match. See the package doc for the
// axis normalization and a worked example of both together.
//
// Rotation0, the reference orientation, is also Rotation's zero value: an
// unset Config or a Device that never calls SetRotation applies no extra
// rotation on top of the normalized axes.
//
// A Rotation value outside the four declared constants is treated as
// Rotation0 (no extra rotation) rather than rejected, since setting it never
// touches the hardware and so can never itself fail.
type Rotation uint8

const (
	Rotation0 Rotation = iota
	Rotation90
	Rotation180
	Rotation270
)

// apply rotates a normalized (x, y) reading to compensate for the mounting
// Rotation describes.
func (r Rotation) apply(x, y int16) (int16, int16) {
	switch r {
	case Rotation90:
		return y, -x
	case Rotation180:
		return -x, -y
	case Rotation270:
		return -y, x
	default:
		return x, y
	}
}
