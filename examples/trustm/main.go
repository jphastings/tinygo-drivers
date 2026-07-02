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

	var cert [600]byte
	n, err := chip.GetDataObject(trustm.OID_DEVICE_CERTIFICATE, 0, cert[:])
	if err != nil {
		println("could not read device certificate:", err.Error())
		return
	}
	println("device certificate: read", n, "bytes")

	message := []byte("hello from TinyGo")
	digest, err := chip.CalcHash(message)
	if err != nil {
		println("could not hash message:", err.Error())
		return
	}
	print("SHA-256 of the message: ")
	printHex(digest[:])

	var pub [80]byte
	pubLen, err := chip.GenKeyPair(trustm.OID_USER_KEY_1, trustm.P256, trustm.KeyUsageSign, pub[:])
	if err != nil {
		println("could not generate key pair:", err.Error())
		return
	}
	print("generated P-256 key pair, public key: ")
	printHex(pub[:pubLen])

	var sig [80]byte
	sigLen, err := chip.CalcSign(trustm.OID_USER_KEY_1, digest[:], sig[:])
	if err != nil {
		println("could not sign digest:", err.Error())
		return
	}
	print("signature: ")
	printHex(sig[:sigLen])

	err = chip.VerifySign(trustm.P256, pub[:pubLen], digest[:], sig[:sigLen])
	if err != nil {
		println("could not verify signature:", err.Error())
		return
	}
	println("signature verified")

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
