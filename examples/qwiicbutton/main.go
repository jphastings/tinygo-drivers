package main

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/qwiicbutton"
)

func main() {
	bus := machine.I2C0
	err := bus.Configure(machine.I2CConfig{})
	if err != nil {
		println("could not configure I2C:", err.Error())
		return
	}

	button := qwiicbutton.New(bus, qwiicbutton.DefaultAddress)

	err = button.Configure()
	if err != nil {
		println("could not configure qwiicbutton driver:", err.Error())
		return
	}

	// Light the button's LED while it is held down
	wasPressed := false
	for {
		pressed, err := button.IsPressed()
		if err != nil {
			println("could not read button:", err.Error())
		} else if pressed != wasPressed {
			wasPressed = pressed
			if pressed {
				println("button pressed")
				button.LEDOn(255)
			} else {
				button.LEDOff()
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
}
