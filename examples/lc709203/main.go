package main

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/lc709203"
)

func main() {
	bus := machine.I2C0
	err := bus.Configure(machine.I2CConfig{})
	if err != nil {
		println("could not configure I2C:", err.Error())
		return
	}

	gauge := lc709203.New(bus)

	// PackSize and BatteryProfile must match the physical battery or RSOC
	// and ITE will not be accurate - see the lc709203 package doc. These
	// values are for a typical 500 mAh 3.7V/4.2V Li-ion/LiPo cell.
	err = gauge.Configure(lc709203.Config{
		PackSize:       lc709203.PackSize500mAh,
		BatteryProfile: lc709203.BatteryProfile0,
	})
	if err != nil {
		println("could not configure LC709203F:", err.Error())
		println("the gauge is powered from the battery cell, not the I2C bus - check one is connected")
		return
	}

	for {
		mv, err := gauge.CellVoltage()
		if err != nil {
			println("could not read cell voltage:", err.Error())
		} else {
			println("cell voltage (mV):", mv)
		}

		rsoc, err := gauge.RSOC()
		if err != nil {
			println("could not read RSOC:", err.Error())
		} else {
			println("state of charge (%):", rsoc)
		}

		milliC, err := gauge.CellTemperature()
		if err != nil {
			println("could not read temperature:", err.Error())
		} else {
			println("temperature (m degC):", milliC)
		}

		time.Sleep(time.Second)
	}
}
