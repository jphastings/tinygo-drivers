package main

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/qwiicjoystick"
)

func main() {
	bus := machine.I2C0
	err := bus.Configure(machine.I2CConfig{})
	if err != nil {
		println("could not configure I2C:", err.Error())
		return
	}

	joystick := qwiicjoystick.New(bus, qwiicjoystick.DefaultAddress)

	// If this board is mounted rotated relative to its silkscreen-upright
	// reference orientation, set Rotation here so Position reports up/down/
	// left/right correctly - see the qwiicjoystick package doc.
	err = joystick.Configure(qwiicjoystick.Config{Rotation: qwiicjoystick.Rotation0})
	if err != nil {
		println("could not configure qwiicjoystick driver:", err.Error())
		return
	}

	if major, minor, err := joystick.FirmwareVersion(); err == nil {
		println("Qwiic Joystick firmware version:", major, ".", minor)
	}

	for {
		x, y, err := joystick.Position()
		if err != nil {
			println("could not read joystick position:", err.Error())
		} else {
			println("X:", x, "Y:", y)
		}

		if pressed, err := joystick.IsPressed(); err == nil && pressed {
			println("button pressed")
		}

		time.Sleep(100 * time.Millisecond)
	}
}
