//go:build tray

package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/gogpu/systray"
)

// RunSystemTrayLoop initializes and runs the systray GUI loop.
// This function blocks the main thread until onQuit is called.
func RunSystemTrayLoop(meshIP string, onQuit func()) {
	printStatus("Running in GUI Tray Mode")

	systray.Register(func() {
		// Initialize the tray icon and tooltips
		systray.SetTitle("AnyPortal")
		systray.SetTooltip("AnyPortal Zero-Config Remote Streaming Portal")

		mIP := systray.AddMenuItem(fmt.Sprintf("🌐 Mesh IP: %s", meshIP), "Click to copy IP Address")
		mAdmin := systray.AddMenuItem("🔗 Open Sunshine Admin Panel", "Open configuration dashboard")
		systray.AddSeparator()
		mLog := systray.AddMenuItem("📝 Open Log File", "Inspect operational diagnostics logs")
		mQuit := systray.AddMenuItem("⏹ Quit AnyPortal", "Safely unmount environment processes")

		for {
			select {
			case <-mIP.ClickedCh:
				// Execute cross-platform clip utility binding or syscall to copy meshIP
				copyToClipboard(meshIP)
			case <-mAdmin.ClickedCh:
				openBrowser("https://127.0.0.1:47990")
			case <-mLog.ClickedCh:
				openLogFile("anyportal.log")
			case <-mQuit.ClickedCh:
				onQuit()
				systray.Quit()
				return
			}
		}
	}, func() {
		// Clean destruction hook callback closure
	})
}

func copyToClipboard(text string) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("clip")
		in, err := cmd.StdinPipe()
		if err == nil {
			go func() {
				defer in.Close()
				in.Write([]byte(text))
			}()
			_ = cmd.Run()
		}
	}
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default: // "linux", "freebsd", "openbsd", "netbsd"
		err = exec.Command("xdg-open", url).Start()
	}
	if err != nil {
		printError("Failed to open browser: %v", err)
	}
}

func openLogFile(filename string) {
	if runtime.GOOS == "windows" {
		absPath, _ := filepath.Abs(filename)
		_ = exec.Command("notepad.exe", absPath).Start()
	}
}
