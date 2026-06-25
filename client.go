package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"tailscale.com/tsnet"
)

func RunClient(cfg *Config, logWriter io.Writer, internalLog *log.Logger, meshIPChan chan<- string, exitChan <-chan struct{}) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := &tsnet.Server{
		Hostname:  "anyportal-client",
		AuthKey:   cfg.AuthKey,
		Ephemeral: true,
		Dir:       filepath.Join(".", ".tsnet-anyportal-client"),
		Logf:      internalLog.Printf,
	}

	if err := os.MkdirAll(srv.Dir, 0700); err != nil {
		meshIPChan <- ""
		return fmt.Errorf("failed to create tsnet state directory: %w", err)
	}

	printStatus("Connecting to Tailscale mesh network...")

	status, err := srv.Up(ctx)
	if err != nil {
		meshIPChan <- ""
		return fmt.Errorf(
			"Tailscale authentication failed.\n\n"+
				"  Your auth key may be expired, already used, or invalid.\n"+
				"  Auth keys can only be used a limited number of times\n"+
				"  (default: 1 use for ephemeral keys).\n\n"+
				"  Fix: Generate a new key at https://login.tailscale.com/admin/settings/keys\n"+
				"       Update 'auth_key' in config.json and restart.\n\n"+
				"  Internal error: %w", err,
		)
	}
	_ = status

	ip4, _ := srv.TailscaleIPs()
	var meshIP string
	if ip4.IsValid() {
		meshIP = ip4.String()
		printStatus("Mesh IP assigned: %s", meshIP)
	}
	printStatus("Mesh tunnel established.")

	// Send Mesh IP to unblock main thread tray loop
	meshIPChan <- meshIP

	var proxyWg sync.WaitGroup
	printStatus("Activating Client Local Forwarding Proxy Engine...")
	StartClientProxy(ctx, srv, cfg.TargetIP, internalLog, &proxyWg)

	printStatus("Invoking Moonlight system frame. Targeted tunnel bound locally onto 127.0.0.1")

	moonlightExe := filepath.Join("core", "moonlight", "Moonlight.exe")

	cmd := exec.CommandContext(ctx, moonlightExe,
		"stream", "127.0.0.1", "Desktop", "--quit-after",
	)
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	if err := cmd.Start(); err != nil {
		srv.Close()
		return fmt.Errorf(
			"Failed to launch Moonlight.\n\n"+
				"  Ensure that Moonlight.exe exists at '%s'.\n"+
				"  Download the Moonlight Portable release from:\n"+
				"  https://github.com/moonlight-stream/moonlight-qt/releases\n\n"+
				"  Internal error: %w",
			moonlightExe, err,
		)
	}

	printStatus("✓ Moonlight launched. Streaming from %s via 127.0.0.1.", cfg.TargetIP)

	return supervise(cmd, srv, "Moonlight", exitChan, cancel, &proxyWg)
}
