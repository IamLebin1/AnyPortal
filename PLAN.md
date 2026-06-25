# Roadmap: AnyPortal

---

## Current Status: Phase 1 (Alpha)

Phase 1 focuses on proving the core concept: a single binary that orchestrates a Tailscale tunnel and a Sunshine/Moonlight stream with zero user-facing network configuration.

---

## Phase 1 — Core Orchestration (current)

**Goal:** Headless, CLI-only operation. No GUI. Proves the fundamental concept.

- [x] Integrate `tailscale.com/tsnet` into a compiled Go binary
- [x] Parse and validate `config.json`
- [x] Implement correct startup sequence: wait for `tsnet.Up()` before writing Sunshine config
  - *This is the most critical implementation detail — see ARCHITECTURE.md §4*
- [x] Dynamically write `sunshine.conf` with `bind_address` set to the tsnet mesh IP
- [x] Spawn and supervise `sunshine.exe` as a child process
- [x] Spawn `moonlight.exe` with direct CLI connection to target mesh IP
- [x] Graceful teardown: SIGINT/SIGTERM → kill Sunshine → close tsnet node
- [ ] Retry logic for `HOST_UNREACHABLE` state (client mode)
- [ ] Detection and user-friendly messaging for `BIND_FAILURE` state
- [ ] Structured log file output (`anyportal.log`)
- [ ] Config validation with actionable error messages (see DESIGN.md §3)

**Known limitations in Phase 1:**
- Mesh IP can change between sessions (ephemeral nodes). The user must manually check the host's console output for the current IP and update `config.json` on the client before each session. Phase 2 addresses this.
- No GPU encoder auto-detection. Assumes hardware encoding is available. If not, Sunshine falls back to software encoding silently.

---

## Phase 2 — Desktop GUI & Quality of Life

**Goal:** Replace the terminal workflow with a minimal desktop UI. Target: non-technical users.

- [ ] System tray icon with status indicator (connecting / ready / streaming / error)
- [ ] Minimal settings window: role selector, auth key field, copy-paste IP from clipboard
- [ ] **Auto-IP sharing**: when in server mode, write the current mesh IP to a known location (e.g. a Tailscale-shared file, or a tiny HTTP endpoint on the mesh) so the client can retrieve it automatically — no manual IP copying required
- [ ] Real-time stream health overlay: latency, estimated bitrate, frame drop rate (sourced from Sunshine's API)
- [ ] Auth key expiry warning: check key age and surface a warning before it expires
- [ ] One-click session restart without editing config files
- [ ] GUI framework candidates: **Wails** (Go-native, ships as a single .exe) or **Fyne** (cross-platform, simpler but less polished). Decision deferred to Phase 2 planning.

---

## Phase 3 — Cross-Platform & Mobile

**Goal:** Expand beyond Windows. Enable iPad/tablet use as a client.

- [ ] Linux host support (Sunshine already supports Linux; `tsnet` is cross-platform)
- [ ] macOS client support (Moonlight has a macOS CLI)
- [ ] iOS/iPadOS client: compile the Go orchestration core to a C-archive via `gomobile` and integrate into a custom Moonlight fork
  - *High complexity — `tsnet` exports are limited via `gomobile`; may require a different binding strategy (e.g. a background Go process communicating via local socket)*
- [ ] Handle iOS/iPadOS background suspension: keep the tsnet tunnel alive while the app is backgrounded using a VoIP push background mode or similar workaround

---

## Testing & Verification Plan

### Before each release tag, verify:

**Startup sequencing**
- [ ] Confirm that Sunshine never starts before `tsnet.Up()` returns
- [ ] Simulate slow coordination server (network throttling) — Sunshine must still start correctly
- [ ] Verify that `bind_address` in the written Sunshine config matches the actual tsnet IP

**Lifecycle & teardown**
- [ ] Kill AnyPortal with Ctrl+C mid-stream → confirm Sunshine exits cleanly with no dangling capture locks
- [ ] Kill AnyPortal with Task Manager (SIGKILL equivalent) → confirm restart works without a reboot
- [ ] Simulate Sunshine crash mid-stream → confirm AnyPortal surfaces the correct error and exits

**Error handling**
- [ ] Invalid auth key → correct AUTH_FAILED message, no hang
- [ ] Expired auth key → correct AUTH_FAILED message
- [ ] Wrong `target_ip` (host not running) → HOST_UNREACHABLE retry loop, then clean exit
- [ ] `target_ip` formatted incorrectly → config validation error at startup, before any network calls

**Memory & stability**
- [ ] 24-hour continuous stream: monitor AnyPortal's memory usage for leaks from the tsnet userspace stack
- [ ] Multiple session restart cycles (10+): confirm no IP binding failures or dangling processes

**Ephemeral node cleanup**
- [ ] After clean exit: confirm the node disappears from the Tailscale admin panel within ~60 seconds
- [ ] After unexpected kill: confirm the node eventually disappears (Tailscale's expiry window for ephemeral nodes is ~30 minutes)

---

## Out of Scope (intentionally)

- **Relay server / DERP self-hosting**: Tailscale's built-in DERP infrastructure is sufficient. Self-hosted DERP adds significant operational burden with marginal latency benefit.
- **Multi-client streaming**: Sunshine supports multiple simultaneous clients, but AnyPortal currently manages a single Sunshine instance. Supporting multiple clients would require a more complex process model.
- **Non-Windows host in Phase 1**: Sunshine's Windows implementation is the most mature. Linux/macOS host support is deferred to Phase 3.
- **In-app Tailscale account management**: Users manage their Tailscale account through the Tailscale admin console. AnyPortal only consumes an auth key.
