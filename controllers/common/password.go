package common

import (
	"bytes"
	"math/rand"
	"time"
)

func GeneratePassword(length int) []byte {
	rand.Seed(time.Now().UnixNano())
	chars := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789" + "!@#$%&*")
	var b bytes.Buffer
	for i := 0; i < length; i++ {
		b.WriteRune(chars[rand.Intn(len(chars))])
	}
	return b.Bytes()
}
