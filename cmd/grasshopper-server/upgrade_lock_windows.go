package main

// lock is a no-op on Windows, where upgrade refuses to run.
func (u *upgrader) lock() (func(), error) { return func() {}, nil }
