package token

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"time"
)

const Version = "v1"

type Window struct {
	Time          time.Time
	Bucket        int64
	Previous      int64
	Next          int64
	BucketSeconds int64
	Start         time.Time
	End           time.Time
	Age           time.Duration
	Remaining     time.Duration
}

func Bucket(t time.Time, bucketSeconds int64) int64 {
	return t.Unix() / bucketSeconds
}

func InspectWindow(t time.Time, bucketSeconds int64) Window {
	bucket := Bucket(t, bucketSeconds)
	start := time.Unix(bucket*bucketSeconds, 0)
	end := start.Add(time.Duration(bucketSeconds) * time.Second)
	return Window{
		Time:          t,
		Bucket:        bucket,
		Previous:      bucket - 1,
		Next:          bucket + 1,
		BucketSeconds: bucketSeconds,
		Start:         start,
		End:           end,
		Age:           t.Sub(start),
		Remaining:     end.Sub(t),
	}
}

func Compute(psk, service, clientID string, bucket int64) string {
	msg := fmt.Sprintf("%s|%s|%s|%s|%d|%s", psk, Version, service, clientID, bucket, psk)
	sum := sha512.Sum512([]byte(msg))
	return hex.EncodeToString(sum[:])
}
