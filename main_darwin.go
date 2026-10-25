package main

import (
	"os"
	"path/filepath"
	"syscall"

	"timestamps/internal/mac"
)

const dataDirHelp = "~/Library/Application Support/Timestamps"

func platformInit() { mac.Prepare() }

func defaultDataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "Timestamps")
}

var instanceLock *os.File // held while the app runs

// singleInstance locks a file in the data folder; false means another copy
// already uses that folder.
func singleInstance(dataDir string) bool {
	f, err := os.OpenFile(filepath.Join(dataDir, ".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return true
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		f.Close()
		return false
	}
	instanceLock = f
	return true
}

func alert(title, text string) { mac.Alert(title, text) }
