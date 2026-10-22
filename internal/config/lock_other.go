//go:build !unix

package config

import "os"

// Elsewhere saves are not locked: the atomic rename still keeps the file
// whole, but two concurrent saves can lose one of their changes.
func tryLockFile(*os.File) (bool, error) { return true, nil }

func unlockFile(*os.File) {}
