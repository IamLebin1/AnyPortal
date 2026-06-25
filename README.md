# AnyPortal

AnyPortal is an open-source, zero-configuration remote game streaming orchestrator for Windows. It provides a seamless "All-in-One" out-of-the-box user experience by orchestrating **Sunshine** (host server), **Moonlight** (client decoder), and **Tailscale** (mesh network) without requiring users to install separate clients or kernel-level virtual network drivers (TUN/TAP).

## Phase 2: Absolute LAN Isolation & Micro-Stutter Optimization
AnyPortal now features **Absolute LAN Isolation**. Sunshine and Moonlight are strictly bound to local `127.0.0.1` interfaces, completely eliminating any open ports on your physical LAN. All traffic is intercepted by a unified userspace TCP/UDP reverse proxy running over `tailscale.com/tsnet`.

To ensure flawless 60fps/120fps streaming, the UDP relay engine utilizes a **Zero-Allocation Memory Pool** (`sync.Pool`), eliminating Garbage Collection spikes and micro-stutters.

Additionally, the project features a **Modular System Tray Integration** for headless or GUI-driven deployments.

## Quick Start

You don't need a Go development environment to run AnyPortal!

1. Go to the [GitHub Releases](../../releases) or the **Actions** tab.
2. Download the `AnyPortal-Windows-Builds` artifact containing the pre-compiled `.exe` binaries.
3. Extract the contents to a folder. You will find:
   - `AnyPortal-CLI.exe`: Standard headless background daemon.
   - `AnyPortal-Tray.exe`: Hidden-console version with a native Windows System Tray GUI.
   - `config.json.example`: The template configuration file.
4. Rename `config.json.example` to `config.json` and enter your Tailscale Auth Key (and Target IP for the client).
5. Run your preferred `.exe`.
