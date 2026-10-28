//go:build unix

package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// lock takes an exclusive lock beside the archives so a manual upgrade,
// rollback or schedule never runs alongside the nightly job. The lock ends
// with the process, so a crash cannot leave it held.
func (u *upgrader) lock() (func(), error) {
	job, err := u.readService()
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(filepath.Dir(job.root), "upgrade.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errors.New("another upgrade, rollback or schedule is already running; try again when it finishes")
		}
		return nil, err
	}
	return func() { file.Close() }, nil
}
