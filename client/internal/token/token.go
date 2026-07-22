package token

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"time"
)

const Version = "v1"

func Bucket(t time.Time, bucketSeconds int64) int64 {
	return t.Unix() / bucketSeconds
}

func Compute(psk, service, clientID string, bucket int64) string {
	msg := fmt.Sprintf("%s|%s|%s|%s|%d|%s", psk, Version, service, clientID, bucket, psk)
	sum := sha512.Sum512([]byte(msg))
	return hex.EncodeToString(sum[:])
}
