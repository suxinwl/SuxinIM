//go:build !linux

package enterprise

import "errors"

func sampleCapacity(string) (capacitySample, error) {
	return capacitySample{}, errors.New("Linux filesystem measurement required")
}
