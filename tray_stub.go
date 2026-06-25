//go:build !tray

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// RunSystemTrayLoop blocks the main thread waiting for an interrupt signal
// when AnyPortal is built without the GUI tray (e.g. standard build).
func RunSystemTrayLoop(meshIP string, onQuit func()) {
	printStatus("Running in Headless Mode (Press Ctrl+C to quit)")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	printStatus("Interrupt signal received. Exiting...")
	onQuit()
}
