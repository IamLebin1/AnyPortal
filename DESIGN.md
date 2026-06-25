# Design: AnyPortal

This document covers the UX philosophy, configuration validation rules, error handling, and console output design for AnyPortal.

---

## 1. Design Philosophy: Zero-Config, Zero-Anxiety

Remote streaming tools fail users in two distinct ways: they're too hard to set up, or they fail silently and leave users with no idea what went wrong. AnyPortal is designed to avoid both.

**Zero-Config** means the user's only cognitive burden is filling in three fields in a JSON file. No driver installs, no router configuration, no DDNS setup. AnyPortal handles everything else.

**Zero-Anxiety** means every state the application can be in is communicated clearly. The console output should feel like a calm, informative assistant — not a wall of logs.

---

## 2. UX Rules

### No installer
AnyPortal ships as a zip. Extracting it is the install. Deleting the folder is the uninstall. No registry entries, no startup services, no leftover files.

### Single config file
All state lives in `config.json`. Users should never have to look anywhere else. Advanced options (like changing ports or logging verbosity) are future concerns — for Phase 1, the three required fields are the only interface.

### Meaningful console output
Every console line should tell the user something actionable. Technical networking jargon is hidden. The output should read like a status tracker, not a debug log.

**Good console output:**
```
[AnyPortal] Starting up...
[AnyPortal] Connecting to Tailscale mesh network...
[AnyPortal] Mesh IP assigned: 100.x.x.x
[AnyPortal] Launching Sunshine encoder...
[AnyPortal] ✓ Ready. Waiting for client connections.
```

**Bad console output (avoid):**
```
tsnet: starting with hostname "anyportal-host", varRoot "C:\..."
tsnet: coordinating with control.tailscale.com...
tsnet: direct connection established (peer 100.x.x.x via [::]:41641)
sunshine: [2025-01-01 00:00:00.000]: Info: config: 'bind_address' = 100.x.x.x
```

Raw tsnet and Sunshine logs should be redirected to a log file (`anyportal.log`) rather than the console.

### Ephemeral by default
The default generated auth key instructions in the README specify ephemeral keys. This is the right default for most users — it keeps the Tailscale admin panel clean without requiring manual cleanup.

---

## 3. Config Validation

AnyPortal validates `config.json` at startup before doing anything else. Validation is strict and fails fast with a clear message.

### Field validation rules

**`role`**
- Must be exactly `"server"` or `"client"` (case-sensitive)
- Any other value → exit with error

**`auth_key`**
- Must be a non-empty string
- Must begin with `tskey-auth-`
- Does not validate the full key format (that's Tailscale's job) — but catches obvious typos like pasting the wrong thing

**`target_ip`**
- Required only when `role == "client"`
- Must match the pattern `100.\d+\.\d+\.\d+` (Tailscale's CGNAT range)
- If provided in server mode, it is silently ignored (no error — allows using the same config file on both machines with just the `role` field changed)

### Validation error format

```
[CONFIG ERROR] 'auth_key' is missing or invalid.
  Expected: a Tailscale auth key starting with "tskey-auth-"
  Found: "tskey-client-xxx" (wrong key type — use an Auth Key, not a Client Key)

  Fix: Generate a new key at https://login.tailscale.com/admin/settings/keys
       Select "Auth Key" and check "Ephemeral".
```

Validation errors include: what was wrong, what was expected, what was actually found (truncated for security — show the prefix and type, not the full key), and a direct link to fix the issue.

---

## 4. Error States & Recovery

### AUTH_FAILED — Invalid or expired auth key

**Trigger:** `tsnet.Server.Up()` returns an authentication error from the Tailscale coordination server.

**Console output:**
```
[ERROR] Tailscale authentication failed.

  Your auth key may be expired, already used, or invalid.
  Auth keys can only be used a limited number of times (default: 1 use for ephemeral keys).

  Fix: Generate a new key at https://login.tailscale.com/admin/settings/keys
       Update 'auth_key' in config.json and restart.
```

**Behaviour:** Exit immediately. Do not attempt retry — a bad key won't become good on its own.

### HOST_UNREACHABLE — Client cannot reach the host

**Trigger:** After the client's tsnet node is up and the mesh tunnel is established, a ping/connection check to `target_ip` fails.

**Console output:**
```
[WARNING] Cannot reach home PC at 100.x.x.x.
  Retrying in 5 seconds... (attempt 1/12)
```

**Behaviour:** Retry every 5 seconds, up to 12 attempts (1 minute total). After 12 failures, exit with:
```
[ERROR] Could not reach home PC after 1 minute.

  Check that AnyPortal is running on your home PC and that
  the mesh IP in config.json (100.x.x.x) matches what was printed there.
```

**Rationale:** The host may take a few seconds to start. Auto-retry removes the need to coordinate startup order between the two machines.

### SUNSHINE_CRASH — Sunshine exits unexpectedly

**Trigger:** The `sunshine.exe` child process exits with a non-zero code.

**Console output:**
```
[ERROR] Sunshine encoder stopped unexpectedly (exit code 1).

  Check anyportal.log for details.
  Common causes:
    - GPU driver crash (try restarting AnyPortal)
    - Another instance of Sunshine is already running
    - Insufficient permissions for screen capture
```

**Behaviour:** AnyPortal exits. The tsnet node is closed cleanly before exit to avoid ghost devices.

### BIND_FAILURE — Sunshine cannot bind to mesh IP

**Trigger:** Sunshine exits immediately after launch with an RTSP bind error (port 48010).

**Detection:** Monitor Sunshine's log file in the first 3 seconds after launch. If it exits and the log contains "failed to bind" or "port in use", surface this specific error.

**Console output:**
```
[ERROR] Sunshine failed to bind to 100.x.x.x.

  This usually means AnyPortal started Sunshine before the mesh tunnel was ready.
  Restarting in 5 seconds...
```

**Behaviour:** Wait for the mesh IP to be re-confirmed, re-write the config, and relaunch Sunshine. Retry up to 3 times before giving up.

---

## 5. Log File

All raw output from `tsnet`, Sunshine, and Moonlight is written to `anyportal.log` in the same directory as the binary. The file is overwritten on each launch (not appended) to avoid unbounded growth. If users need persistent logs for debugging, they should copy the file before restarting.

The log file path is printed to the console at startup:
```
[AnyPortal] Logging to: C:\...\anyportal.log
```
