//go:build !linux

package bench

import "errors"

func LockCPU(string) (func() error, error) {
	return nil, errors.New("CPU runner available only on Linux")
}
