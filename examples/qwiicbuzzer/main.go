package main

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/qwiicbuzzer"
)

func main() {
	bus := machine.I2C0
	err := bus.Configure(machine.I2CConfig{})
	if err != nil {
		println("could not configure I2C:", err.Error())
		return
	}

	buzzer := qwiicbuzzer.New(bus, qwiicbuzzer.DefaultAddress)

	err = buzzer.Configure()
	if err != nil {
		println("could not configure qwiicbuzzer driver:", err.Error())
		return
	}

	if major, minor, err := buzzer.FirmwareVersion(); err == nil {
		println("Qwiic Buzzer firmware version:", major, ".", minor)
	}

	// Beep at the piezo's resonant frequency, its loudest, once every 2s.
	for {
		err := buzzer.Play(qwiicbuzzer.ResonantFrequency, qwiicbuzzer.VolumeMax, 200*time.Millisecond)
		if err != nil {
			println("could not play tone:", err.Error())
		}
		time.Sleep(2 * time.Second)
	}
}
