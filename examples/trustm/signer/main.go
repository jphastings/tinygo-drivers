// Demonstrates using the OPTIGA Trust M as a crypto.Signer: keys generated
// and used on the chip, signatures verified with Go's standard crypto.
//
// Note that crypto/ecdsa and crypto/x509 cost several hundred kilobytes of
// flash; this requires one of the larger TinyGo targets.
package main

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"machine"
	"time"

	"tinygo.org/x/drivers/trustm"
	"tinygo.org/x/drivers/trustm/signer"
)

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

	cert, err := signer.DeviceCertificate(&chip)
	if err != nil {
		println("could not read device certificate:", err.Error())
		return
	}
	println("device certificate subject:", cert.Subject.String())

	s, err := signer.Generate(&chip, trustm.OID_USER_KEY_1, trustm.P256)
	if err != nil {
		println("could not generate key:", err.Error())
		return
	}

	digest := sha256.Sum256([]byte("signed on the chip"))
	sig, err := s.Sign(nil, digest[:], nil)
	if err != nil {
		println("could not sign:", err.Error())
		return
	}

	if ecdsa.VerifyASN1(s.Public().(*ecdsa.PublicKey), digest[:], sig) {
		println("signature verified with Go's crypto/ecdsa")
	} else {
		println("signature did NOT verify")
	}

	for {
		time.Sleep(time.Second)
	}
}
