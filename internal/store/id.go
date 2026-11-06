package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// newID generates a sortable, unique id with the given prefix: a UTC timestamp
// (so ids are naturally time-ordered) plus random hex for uniqueness.
func newID(prefix string) string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s-%s-%s", prefix, time.Now().UTC().Format("20060102T150405"), hex.EncodeToString(b[:]))
}
