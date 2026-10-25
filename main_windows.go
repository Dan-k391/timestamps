package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"timestamps/internal/win"
)

const dataDirHelp = `%LOCALAPPDATA%\Timestamps`

func platformInit() { win.SetDPIAware() }

func defaultDataDir() string { return filepath.Join(os.Getenv("LOCALAPPDATA"), "Timestamps") }

// singleInstance takes a mutex named after the data folder; false means
// another copy already uses that folder.
func singleInstance(dataDir string) bool {
	return win.SingleInstance(`Local\Timestamps-` + strconv.FormatUint(uint64(hashString(strings.ToLower(dataDir))), 16))
}

func alert(title, text string) { win.MessageBox(title, text) }
