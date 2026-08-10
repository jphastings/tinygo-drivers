package main

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/st25dv"
)

const hexDigits = "0123456789ABCDEF"

func main() {
	bus := machine.I2C0
	err := bus.Configure(machine.I2CConfig{})
	if err != nil {
		println("could not configure I2C:", err.Error())
		return
	}

	tag := st25dv.New(bus)

	err = tag.Configure()
	if err != nil {
		println("could not configure st25dv driver:", err.Error())
		return
	}

	uid, err := tag.UID()
	if err != nil {
		println("could not read UID:", err.Error())
		return
	}
	print("found ST25DV, ", tag.Size(), " bytes, UID: ")
	for _, b := range uid {
		print(string(hexDigits[b>>4]), string(hexDigits[b&0xF]))
	}
	println()

	// Store a URL on the tag: tap it with a smartphone to open the site
	err = tag.WriteNDEFURI("https://tinygo.org")
	if err != nil {
		println("could not write NDEF message:", err.Error())
		return
	}
	println("NDEF URL written, tap the tag with a phone")

	for {
		time.Sleep(time.Second)
	}
}
