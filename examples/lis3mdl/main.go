package main

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/lis3mdl"
)

func main() {
	bus := machine.I2C0
	if err := bus.Configure(machine.I2CConfig{}); err != nil {
		println("could not configure I2C:", err.Error())
		return
	}

	mag := lis3mdl.New(bus)

	if err := mag.Configure(lis3mdl.Config{
		Scale:           lis3mdl.Scale4Gauss,
		PerformanceMode: lis3mdl.HighPerformance,
		DataRate:        lis3mdl.DataRate80Hz,
	}); err != nil {
		println("could not configure LIS3MDL:", err.Error())
		return
	}

	for {
		x, y, z, err := mag.ReadMagneticField()
		if err != nil {
			println("could not read magnetic field:", err.Error())
		} else {
			println("field (mG): X =", x, "Y =", y, "Z =", z)
		}

		time.Sleep(100 * time.Millisecond)
	}
}
