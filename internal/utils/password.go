package utils

import (
	"bytes"
	"crypto/rand"
	"math/big"
)

func GeneratePassword(length int) []byte {
	chars := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789" + "!@#$%&*")
	var b bytes.Buffer
	for i := 0; i < length; i++ {
		index, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b.WriteRune(chars[index.Int64()])
	}
	return b.Bytes()
}
