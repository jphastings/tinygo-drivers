package lc709203

import "errors"

// PowerMode selects between full operation and a low-power sleep in which
// only I2C communication continues - RSOC/voltage tracking pauses, and
// picks back up from the data gathered before sleep once back in Operate.
type PowerMode uint16

const (
	PowerModeOperate PowerMode = 0x0001
	PowerModeSleep   PowerMode = 0x0002
)

var errInvalidPowerMode = errors.New("lc709203: invalid power mode")

func (m PowerMode) valid() bool {
	return m == PowerModeOperate || m == PowerModeSleep
}
