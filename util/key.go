package util

import (
	"crypto/sha512"
	"encoding/hex"
	"math/rand"
	"strings"
)

const domain = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

func CreateKey(len int) (string, error) {
	// TODO: provided that CreateTripCode still uses sha512, the max len can be 128 at most.
	if len > 128 {
		panic("len > 128")
	}

	str := CreateTripCode(RandomID(len))
	return str[:len], nil
}

func CreateTripCode(input string) string {
	out := sha512.Sum512([]byte(input))

	return hex.EncodeToString(out[:])
}

func RandomID(size int) string {
	rng := size
	newID := strings.Builder{}

	for i := 0; i < rng; i++ {
		newID.WriteByte(domain[rand.Intn(len(domain))])
	}

	return newID.String()
}
