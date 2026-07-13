package winprobe

import "fmt"

type InputMode string

const (
	InputDefault  InputMode = "default"
	InputSelected InputMode = "selected"
)

func ResolveInput(mode InputMode, defaultDeviceID string, devices []Device, selected int) (Device, error) {
	switch mode {
	case InputDefault:
		if defaultDeviceID == "" {
			return Device{}, fmt.Errorf("default capture device is not ready")
		}
		for _, device := range devices {
			if device.ID == defaultDeviceID {
				return device, nil
			}
		}
		return Device{ID: defaultDeviceID, Name: "system default"}, nil
	case InputSelected:
		if selected < 0 || selected >= len(devices) {
			return Device{}, fmt.Errorf("selected capture device index %d is unavailable", selected)
		}
		return devices[selected], nil
	default:
		return Device{}, fmt.Errorf("unknown input mode %q", mode)
	}
}
