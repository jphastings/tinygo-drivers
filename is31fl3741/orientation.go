package is31fl3741

import "errors"

// Reference dimensions of the Adafruit 13x9 RGB Matrix QT panel, with its
// silkscreen text upright: column (x) increasing to the right, row (y)
// increasing downwards. Rotation and Mirror remap caller coordinates onto
// this frame.
const (
	panelWidth  = 13
	panelHeight = 9
)

// Rotation describes how a DeviceAdafruitRGBMatrixQT13x9 panel is physically
// mounted, relative to its reference orientation: silkscreen text upright,
// as described on DeviceAdafruitRGBMatrixQT13x9.
//
// SetPixel always takes coordinates in the panel's current, as-mounted
// orientation: (0, 0) is the top-left corner as you look at the panel where
// it is actually mounted, x increasing to the right and y increasing
// downwards. Set the rotation once to match how the board is mounted, then
// draw as if that were the only orientation that existed - the driver remaps
// onto the wiring underneath. See Mirror for how mirroring composes with
// rotation.
//
// The zero value, Rotation0, is the identity: no compensation is applied.
//
// Worked example: a pixel drawn at (0, 0), the top-left corner as the viewer
// sees it, lands here on the silkscreen-upright panel:
//
//	Rotation0   (panel not rotated):    top-left
//	Rotation90  (panel mounted 90° CW): bottom-left
//	Rotation180 (panel mounted 180°):   bottom-right
//	Rotation270 (panel mounted 270° CW, i.e. 90° CCW): top-right
//
// Size() reports the panel's dimensions as they appear after rotation: 13x9
// at Rotation0/Rotation180, 9x13 at Rotation90/Rotation270.
type Rotation uint8

const (
	// Rotation0 is the reference orientation: silkscreen text upright, no
	// compensation applied. This is the zero value.
	Rotation0 Rotation = iota
	// Rotation90 compensates for a panel mounted rotated 90° clockwise from
	// its reference orientation.
	Rotation90
	// Rotation180 compensates for a panel mounted upside down.
	Rotation180
	// Rotation270 compensates for a panel mounted rotated 270° clockwise
	// (90° counter-clockwise) from its reference orientation.
	Rotation270
)

var errInvalidRotation = errors.New("is31fl3741: invalid rotation")

func (r Rotation) valid() bool {
	switch r {
	case Rotation0, Rotation90, Rotation180, Rotation270:
		return true
	}
	return false
}

// Mirror flips SetPixel's caller coordinates along one or both axes, on top
// of whatever Rotation is in force. MirrorHorizontal and MirrorVertical can
// be combined with a bitwise OR.
//
// Mirroring is defined in the caller's frame, applied after rotation: it
// flips the axis the caller is currently drawing along, not an axis of the
// physical panel. "Mirror horizontally" always swaps left and right as the
// caller sees them, whatever Rotation is set to; it does not, for example,
// flip top and bottom just because a 90° rotation is also in force.
//
// Worked example: a 13x9 panel at Rotation90 presents to the caller as 9
// wide, 13 tall (see Rotation). A pixel drawn at the caller's top-right
// corner, (8, 0):
//
//	Rotation90 alone:                  bottom-left of the silkscreen-upright panel
//	Rotation90 | MirrorHorizontal:     top-left (MirrorHorizontal swaps the caller's
//	                                   left/right, moving the drawn point from the
//	                                   caller's top-right to top-left before rotating)
//
// A panel viewed from behind through a diffuser, for instance, is mirrored
// on both axes at once: MirrorHorizontal | MirrorVertical. That combination
// is equivalent to Rotation180 with no mirroring - flipping both axes is the
// same transform as a half turn - so the 16 nominal (Rotation, Mirror)
// combinations only produce 8 distinct orientations. That's expected, not a
// bug: don't be surprised to find two settings that behave identically.
//
// The zero value, MirrorNone, is the identity: no flip is applied.
type Mirror uint8

const (
	// MirrorNone applies no flip. This is the zero value.
	MirrorNone Mirror = 0
	// MirrorHorizontal flips the caller's left/right axis.
	MirrorHorizontal Mirror = 1 << 0
	// MirrorVertical flips the caller's top/bottom axis.
	MirrorVertical Mirror = 1 << 1
)

var errInvalidMirror = errors.New("is31fl3741: invalid mirror")

func (m Mirror) valid() bool {
	return m&^(MirrorHorizontal|MirrorVertical) == 0
}

// SetRotation sets how the panel is physically mounted; see Rotation for how
// this affects SetPixel and Size.
func (d *DeviceAdafruitRGBMatrixQT13x9) SetRotation(r Rotation) error {
	if !r.valid() {
		return errInvalidRotation
	}
	d.rotation = r
	return nil
}

// Rotation returns the rotation most recently set with SetRotation.
func (d *DeviceAdafruitRGBMatrixQT13x9) Rotation() Rotation {
	return d.rotation
}

// SetMirror sets which axes SetPixel's coordinates are flipped along; see
// Mirror for how this composes with rotation.
func (d *DeviceAdafruitRGBMatrixQT13x9) SetMirror(m Mirror) error {
	if !m.valid() {
		return errInvalidMirror
	}
	d.mirror = m
	return nil
}

// Mirror returns the mirroring most recently set with SetMirror.
func (d *DeviceAdafruitRGBMatrixQT13x9) Mirror() Mirror {
	return d.mirror
}

// toPanel maps a pixel coordinate in the panel's current, as-mounted and
// mirrored orientation onto the reference-orientation coordinates SetPixel's
// wiring table expects. ok is false if the coordinate falls outside the
// panel as currently rotated (mirroring never changes the panel's size).
func (d *DeviceAdafruitRGBMatrixQT13x9) toPanel(x, y int16) (px, py int16, ok bool) {
	w, h := d.Size()
	if x < 0 || x >= w || y < 0 || y >= h {
		return 0, 0, false
	}

	// Mirroring is defined in the caller's frame, so it is undone here
	// before rotation, using the caller's (rotated) dimensions.
	if d.mirror&MirrorHorizontal != 0 {
		x = w - 1 - x
	}
	if d.mirror&MirrorVertical != 0 {
		y = h - 1 - y
	}

	switch d.rotation {
	case Rotation90:
		return y, panelHeight - 1 - x, true
	case Rotation180:
		return panelWidth - 1 - x, panelHeight - 1 - y, true
	case Rotation270:
		return panelWidth - 1 - y, x, true
	default:
		return x, y, true
	}
}
