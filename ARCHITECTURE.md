# Architecture: AnyPortal

This document covers the system design, data flow, component responsibilities, and critical implementation constraints for AnyPortal.

---

## 1. Philosophy: Complexity Hiding

Traditional game streaming setups require users to manually manage three distinct layers:

1. **Application layer** — Sunshine host configuration
2. **Network layer** — NAT traversal, port forwarding, dynamic DNS
3. **Security layer** — VPN setup, encryption, access control

AnyPortal collapses these into a single binary by implementing **Userspace Network Orchestration**: the network layer and security layer are handled automatically by an embedded `tsnet` node, and Sunshine is configured and launched as a managed child process.

---

## 2. Components

### 2.1 AnyPortal Orchestrator (Go)

The core binary. Dual-mode: runs as either a **Host Runner** or a **Client Spawner** depending on `config.json`. Responsibilities:

- Parse and validate `config.json` at startup
- Instantiate and manage the `tsnet.Server` node lifecycle
- Start the Userspace TCP/UDP Reverse Proxy Engine on local ports
- Dynamically write Sunshine's `bind_address` config and spawn `sunshine.exe` (host mode)
- Launch `moonlight.exe` with the target IP `127.0.0.1` once the tunnel is confirmed (client mode)
- Intercept OS signals to dispatch a graceful kill sequence to child processes and tunnel teardowns
- Present an optional CGo-free System Tray UI via conditionally compiled build tags

### 2.2 Tailscale `tsnet` Layer

`tsnet` is an official Go library (`tailscale.com/tsnet`) that runs a complete, self-contained Tailscale node inside the process using a gVisor userspace TCP/IP stack. Key properties:

- No kernel drivers or system daemons required
- No root/administrator privileges required for the network stack itself
- Each `tsnet.Server` instance gets its own `100.x.x.x` mesh IP address

### 2.3 The Client-Side Routing Gap & Userspace Reverse Proxy

**Critical Discovery:** `tsnet` is userspace-only. The `100.x.x.x` mesh IP exists only inside the Go process's gVisor stack. It is **not** a real OS network interface.
When Moonlight runs as a separate process, calling the OS `connect()` syscall to `100.x.x.x` will fail with no route, because no TUN interface exists.

**The Solution:** The AnyPortal Client spawns local `127.0.0.1` listeners for all 8 Sunshine ports. It intercepts Moonlight's traffic, and funnels it via `tsnet srv.Dial()` to the Server. The Server, in turn, listens on the mesh network via `srv.Listen()`, and uses standard OS `net.Dial()` to forward traffic to Sunshine bound strictly to `127.0.0.1`.

This achieves **Absolute LAN Isolation**. The physical home LAN never sees Sunshine's open ports, and Moonlight operates fully inside the localhost bounds.

---

## 3. System Layout

```text
┌─────────────────────────── HOST (home PC) ─────────────────────────────┐
│                                                                          │
│   AnyPortal.exe (Go)          core/sunshine.exe                      │
│   ┌───────────────────┐          ┌──────────────────────┐               │
│   │  Server Proxy     │──dial──▶ │  Bound to 127.0.0.1  │               │
│   │  (net.Dial)       │          │  NVENC/AMF encoder   │               │
│   ├───────────────────┤          └──────────────────────┘               │
│   │  tsnet.Server     │                                                  │
│   │  srv.Listen()     │                                                  │
│   └───────────────────┘                                                  │
│           │                                                              │
│    WireGuard tunnel (E2E encrypted, NAT traversal via Tailscale DERP)   │
└───────────│──────────────────────────────────────────────────────────────┘
            │
┌───────────│────────────── CLIENT (laptop) ─────────────────────────────┐
│           │                                                              │
│   AnyPortal.exe (Go)          core/moonlight.exe                     │
│   ┌───────────────────┐          ┌──────────────────────┐               │
│   │  tsnet.Server     │          │  stream 127.0.0.1    │               │
│   │  srv.Dial()       │          │  HW decoder          │               │
│   ├───────────────────┤          └──────────────────────┘               │
│   │  Client Proxy     │◀─connect─│                                      │
│   │  (net.Listen)     │                                                  │
│   └───────────────────┘                                                  │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 4. Startup Sequence

1. Parse config.json
2. Instantiate tsnet.Server with AuthKey
3. Call tsnet.Server.Up(ctx) — blocks until node is authenticated
4. Call tsnet.Server.LocalAddr() to retrieve the assigned mesh IP
5. Start the Userspace Proxy Engine (Server Proxy or Client Proxy)
6. Write sunshine.conf with `bind_address = 127.0.0.1` (Host Mode)
7. Spawn core/sunshine.exe or core/moonlight.exe (targeting 127.0.0.1)
8. Send the assigned mesh IP to unblock the main thread and initialize `RunSystemTrayLoop`.

---

## 5. Port Usage

The proxy matrix maps these ports exclusively on `127.0.0.1`:

| Port | Protocol | Purpose |
|---|---|---|
| 47984 | TCP | HTTPS (Web UI) |
| 47989 | TCP | HTTP |
| 47990 | TCP | Web UI |
| 48010 | TCP | RTSP control |
| 47998 | UDP | Video stream |
| 47999 | UDP | Control / Input |
| 48000 | UDP | Audio stream |
| 48002 | UDP | Mic back-channel |

---

## 6. Process Lifecycle & Teardown

On signal receipt (Ctrl+C, or Quit from Tray):
1. Send kill signals to the child process (Moonlight/Sunshine) to release hardware locks.
2. The proxy context is cancelled, shutting down all proxy mappings and memory pools.
3. `tsnet.Server.Close()` is called to cleanly deregister the ephemeral node from the tailnet.
4. Exit.
