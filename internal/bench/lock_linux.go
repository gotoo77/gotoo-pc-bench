//go:build linux

package bench

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// LockCPU prevents overlapping CPU measurements across separate CLI processes.
// Kernel flock locks are automatically released on exit, including crashes.
func LockCPU(dir string) (func() error, error) {
	p := filepath.Join(dir, ".pc-bench-cpu.lock")
	st, err := os.Lstat(p)
	if err == nil && !st.Mode().IsRegular() {
		return nil, fmt.Errorf("benchmark lock path is not a regular file: %s", p)
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another CPU benchmark may already be running: %w", err)
	}
	return func() error {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		returnErr := f.Close()
		if err != nil {
			return err
		}
		return returnErr
	}, nil
}
