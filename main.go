package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

const version = "0.2.0-beta"

const logPrefix = "[AnyPortal]"

func printStatus(format string, args ...any) {
	fmt.Printf("%s %s\n", logPrefix, fmt.Sprintf(format, args...))
}

func printError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[ERROR] %s\n", fmt.Sprintf(format, args...))
}

func setupLogFile() (io.Writer, func(), error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to determine executable path: %w", err)
	}
	logPath := filepath.Join(filepath.Dir(exePath), "anyportal.log")

	f, err := os.Create(logPath)
	if err != nil {
		logPath = "anyportal.log"
		f, err = os.Create(logPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create log file: %w", err)
		}
	}

	printStatus("Logging to: %s", logPath)
	cleanup := func() { f.Close() }
	return f, cleanup, nil
}

func main() {
	printStatus("AnyPortal v%s", version)
	printStatus("Starting up...")

	cfg, err := LoadConfig("config.json")
	if err != nil {
		printError("%v", err)
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	logWriter, logCleanup, err := setupLogFile()
	if err != nil {
		printError("%v", err)
		os.Exit(1)
	}
	defer logCleanup()

	internalLog := log.New(logWriter, "[internal] ", log.LstdFlags)

	// Channel to receive the mesh IP after network initialization
	meshIPChan := make(chan string, 1)
	// Channel to signal application exit
	exitChan := make(chan struct{})

	// The quit callback triggers graceful teardown of child processes and network proxy
	onQuit := func() {
		close(exitChan)
	}

	switch cfg.Role {
	case "server":
		printStatus("Mode: Host (server)")
		go func() {
			if err := RunServer(cfg, logWriter, internalLog, meshIPChan, exitChan); err != nil {
				printError("%v", err)
				os.Exit(1)
			}
			// Trigger exit routine naturally if it concludes
			select {
			case <-exitChan:
			default:
				onQuit()
			}
		}()
	case "client":
		printStatus("Mode: Client")
		go func() {
			if err := RunClient(cfg, logWriter, internalLog, meshIPChan, exitChan); err != nil {
				printError("%v", err)
				os.Exit(1)
			}
			// Trigger exit routine naturally if it concludes
			select {
			case <-exitChan:
			default:
				onQuit()
			}
		}()
	}

	// Wait for the orchestration network to assign a Mesh IP (or fail)
	assignedMeshIP := <-meshIPChan

	// If empty string is sent, the initialization failed and we should exit.
	if assignedMeshIP == "" {
		os.Exit(1)
	}

	// Run the system tray (GUI or Stub). This blocks the main thread.
	RunSystemTrayLoop(assignedMeshIP, onQuit)
}
