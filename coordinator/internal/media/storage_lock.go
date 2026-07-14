package media

import (
	"crypto/sha256"
	"sync"
)

// A coordinator process is the sole owner of mediaDir. Publication and
// deletion nevertheless run in different goroutines, so both sides share one
// keyed lock across their filesystem plus database acknowledgement boundary.
// This prevents an absent-path cleanup receipt from racing a stale publisher
// that is about to create that same canonical path.
var canonicalStorageLocks [128]sync.Mutex

func canonicalStorageLock(storageKey string) *sync.Mutex {
	digest := sha256.Sum256([]byte(storageKey))
	return &canonicalStorageLocks[int(digest[0])%len(canonicalStorageLocks)]
}
