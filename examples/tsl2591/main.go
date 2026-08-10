package main

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/tsl2591"
)

func main() {
	bus := machine.I2C0
	err := bus.Configure(machine.I2CConfig{})
	if err != nil {
		println("could not configure I2C:", err.Error())
		return
	}

	light := tsl2591.New(bus)

	err = light.Configure(tsl2591.Config{
		Gain:            tsl2591.GainMedium,
		IntegrationTime: tsl2591.IntegrationTime100ms,
	})
	if err != nil {
		println("could not configure TSL2591:", err.Error())
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
