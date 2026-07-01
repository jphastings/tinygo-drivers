package main

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/trustm"
)

const hexDigits = "0123456789ABCDEF"

func main() {
	bus := machine.I2C0
	err := bus.Configure(machine.I2CConfig{})
	if err != nil {
		println("could not configure I2C:", err.Error())
		return
	}

	chip := trustm.New(bus)

	err = chip.Configure()
	if err != nil {
		println("could not configure trustm driver:", err.Error())
		return
	}

	uid, err := chip.UID()
	if err != nil {
		println("could not read UID:", err.Error())
		return
	}
	print("found OPTIGA Trust M, coprocessor UID: ")
	printHex(uid[:])

	var rnd [16]byte
	for {
		err = chip.GetRandom(rnd[:])
		if err != nil {
			println("could not get random bytes:", err.Error())
			return
		}
		print("random: ")
		printHex(rnd[:])

		time.Sleep(time.Second)
	}
}

func printHex(data []byte) {
	for _, b := range data {
		print(string(hexDigits[b>>4]), string(hexDigits[b&0xF]))
	}
	println()
}
