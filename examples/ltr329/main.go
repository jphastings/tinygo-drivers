package main

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/ltr329"
)

func main() {
	bus := machine.I2C0
	err := bus.Configure(machine.I2CConfig{})
	if err != nil {
		println("could not configure I2C:", err.Error())
		return
	}

	light := ltr329.New(bus)

	err = light.Configure(ltr329.Config{
		Gain:            ltr329.GainX8,
		IntegrationTime: ltr329.IntegrationTime100ms,
		MeasurementRate: ltr329.MeasurementRate500ms,
	})
	if err != nil {
		println("could not configure LTR-329ALS-01:", err.Error())
		return
	}

	for {
		milliLux, err := light.Illuminance()
		if err != nil {
			println("could not read illuminance:", err.Error())
		} else {
			ch0, ch1 := light.Channels()
			println("illuminance (mlx):", milliLux, "ch0:", ch0, "ch1:", ch1)
		}

		time.Sleep(500 * time.Millisecond)
	}
}
