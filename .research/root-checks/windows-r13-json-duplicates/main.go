package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

func main() {
	var sidecar struct {
		Reason int `json:"reason"`
	}
	decoder := json.NewDecoder(strings.NewReader(`{"reason":0,"reason":1}`))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&sidecar)
	fmt.Printf("error=%v reason=%d\n", err, sidecar.Reason)
}
