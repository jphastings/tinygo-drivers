package main

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/mpr121"
)

func main() {
	bus := machine.I2C0
	err := bus.Configure(machine.I2CConfig{})
	if err != nil {
		println("could not configure I2C:", err.Error())
		return
	}

	touch := mpr121.New(bus, mpr121.DefaultAddress)

	err = touch.Configure(mpr121.Config{})
	if err != nil {
		println("could not configure MPR121:", err.Error())
		return
	}

	for {
		status, err := touch.Touched()
		if err != nil {
			println("could not read touch status:", err.Error())
			time.Sleep(200 * time.Millisecond)
			continue
		}

		if over, _ := touch.OverCurrent(); over {
			println("over-current fault, clearing")
			touch.ClearOverCurrent()
		}

		for e := uint8(0); e < mpr121.NumElectrodes; e++ {
			if status&(1<<e) != 0 {
				print("ELE", e, " ")
			}
		}
		println()

		time.Sleep(100 * time.Millisecond)
	}
}
