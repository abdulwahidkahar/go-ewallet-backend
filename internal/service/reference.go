package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func generateReference(prefix string) (string, error) {
	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%s-%s", prefix, time.Now().UTC().Format("20060102150405"), hex.EncodeToString(randomBytes)), nil
}
