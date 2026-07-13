package winprobe

import (
	"crypto/sha256"
	"fmt"
	"io"
)

func ReadAndHashPickedFile(reader io.Reader, limit int64) (string, int64, error) {
	if reader == nil || limit < 0 {
		return "", 0, fmt.Errorf("invalid picker reader or byte limit")
	}
	hash := sha256.New()
	buffer := make([]byte, 64<<10)
	var total int64
	for {
		read, err := reader.Read(buffer)
		if read > 0 {
			total += int64(read)
			if total > limit {
				return "", total, fmt.Errorf("actual bytes exceed %d", limit)
			}
			_, _ = hash.Write(buffer[:read])
		}
		if err == io.EOF {
			if total == 0 {
				return "", 0, fmt.Errorf("picked file is empty")
			}
			break
		}
		if err != nil {
			return "", total, err
		}
		if read == 0 {
			return "", total, io.ErrNoProgress
		}
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), total, nil
}
