// Displays some text on a SparkFun Qwiic Alphanumeric Display, then cycles
// its brightness.
package main

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/vk16k33"
)

func main() {
	bus := machine.I2C0
	err := bus.Configure(machine.I2CConfig{})
	if err != nil {
		println("could not configure I2C:", err.Error())
		return
	}

	display := vk16k33.New(bus)

	err = display.Configure()
	if err != nil {
		println("could not configure vk16k33 driver:", err.Error())
		return
	}

	err = display.WriteString("TINY")
	if err != nil {
		println("could not write to display:", err.Error())
		return
	}

	for {
		for brightness := uint8(0); brightness < 16; brightness++ {
			display.SetBrightness(brightness)
			time.Sleep(100 * time.Millisecond)
		}
	}
}
