//go:build !linux

package bench

import "errors"

func LockBenchmark(string) (func() error, error) {
	return nil, errors.New("benchmark runner available only on Linux")
}

func LockCPU(dir string) (func() error, error) { return LockBenchmark(dir) }
