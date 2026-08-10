package main

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/mmc5983"
)

func main() {
	bus := machine.I2C0
	err := bus.Configure(machine.I2CConfig{})
	if err != nil {
		println("could not configure I2C:", err.Error())
		return
	}

	mag := mmc5983.New(bus)

	err = mag.Configure(mmc5983.Config{})
	if err != nil {
		println("could not configure MMC5983MA:", err.Error())
		return
	}

	for {
		x, y, z, err := mag.ReadMagneticField()
		if err != nil {
			println("could not read magnetic field:", err.Error())
		} else {
			println("field (nT): X =", x, "Y =", y, "Z =", z)
		}

		heading, err := mag.ReadCompass()
		if err == nil {
			println("heading:", heading/1000, "degrees")
		}

		time.Sleep(100 * time.Millisecond)
	}
}
