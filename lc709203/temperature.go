package lc709203

import "errors"

// TemperatureSource selects how the Cell Temperature register is
// populated. This is the datasheet's Status Bit register (0x16); despite
// the name it holds this one setting, not a bitfield of flags.
type TemperatureSource uint16

const (
	// TemperatureSourceI2C means the host must keep the Cell Temperature
	// register current itself, via SetTemperature; this is the register's
	// power-on default. The datasheet asks for it to be refreshed whenever
	// the true temperature moves by more than 1 degC during charge or
	// discharge, since RSOC accuracy depends on it.
	TemperatureSourceI2C TemperatureSource = 0x0000
	// TemperatureSourceThermistor has the chip measure a thermistor wired
	// to TSENSE/TSW itself, powering the thermistor only for the instant
	// of each measurement. Requires ThermistorB (see Config) to be set for
	// the thermistor in use.
	TemperatureSourceThermistor TemperatureSource = 0x0001
)

var errInvalidTemperatureSource = errors.New("lc709203: invalid temperature source")

func (s TemperatureSource) valid() bool {
	return s == TemperatureSourceI2C || s == TemperatureSourceThermistor
}

// defaultThermistorB is the Thermistor B register's own power-on default,
// 3380 (a typical B25/85 constant for a 10 kOhm NTC thermistor).
const defaultThermistorB uint16 = 0x0D34

// kelvinZeroRaw is the Cell Temperature register value the datasheet
// itself defines as exactly 0.0 degC: 0x0AAC, in 0.1K units (Table 6's
// "Initial Value" column even cross-checks this: the register's power-on
// default of 0x0BA6 is annotated "(25 degC)", and (0x0BA6-0x0AAC)*0.1 =
// 25.0 exactly). Anchoring on this published value, rather than deriving
// it from 273.15K afresh, keeps this conversion bit-for-bit consistent
// with the chip's own.
const kelvinZeroRaw = 0x0AAC

// rawToMilliCelsius converts a Cell Temperature register value (0.1K
// units) to milli-degrees Celsius.
func rawToMilliCelsius(raw uint16) int32 {
	return (int32(raw) - kelvinZeroRaw) * 100
}

// milliCelsiusToRaw is the inverse of rawToMilliCelsius, rounding to the
// register's 0.1 degC resolution rather than truncating toward zero.
func milliCelsiusToRaw(milliC int32) uint16 {
	if milliC >= 0 {
		return uint16((milliC+50)/100 + kelvinZeroRaw)
	}
	return uint16((milliC-50)/100 + kelvinZeroRaw)
}
