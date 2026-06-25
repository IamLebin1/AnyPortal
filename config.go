package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Config holds the runtime configuration parsed from config.json.
type Config struct {
	Role     string `json:"role"`      // "server" or "client"
	AuthKey  string `json:"auth_key"`  // Tailscale ephemeral auth key (tskey-auth-...)
	TargetIP string `json:"target_ip"` // Required in client mode: host's 100.x.x.x mesh IP
}

// tailscaleIPRegex matches Tailscale CGNAT range addresses (100.x.x.x).
var tailscaleIPRegex = regexp.MustCompile(`^100\.\d{1,3}\.\d{1,3}\.\d{1,3}$`)

// LoadConfig reads and parses config.json from the given path.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf(
				"config file not found: %s\n\n"+
					"  Copy config.json.example to config.json and fill in your values:\n"+
					"  {\n"+
					"    \"role\": \"server\",\n"+
					"    \"auth_key\": \"tskey-auth-...\",\n"+
					"    \"target_ip\": \"\"\n"+
					"  }\n\n"+
					"  See README.md for details",
				path,
			)
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf(
			"failed to parse config.json: %w\n\n"+
				"  Ensure the file contains valid JSON",
			err,
		)
	}

	return &cfg, nil
}

// Validate checks all config fields against the documented rules (DESIGN.md §3).
// Returns nil if valid, or an actionable error message if not.
func (c *Config) Validate() error {
	// --- Role ---
	if c.Role != "server" && c.Role != "client" {
		found := c.Role
		if found == "" {
			found = "(empty)"
		}
		return fmt.Errorf(
			"[CONFIG ERROR] 'role' is invalid.\n"+
				"  Expected: \"server\" or \"client\" (case-sensitive)\n"+
				"  Found:    %q\n\n"+
				"  Fix: Set \"role\" to either \"server\" or \"client\" in config.json",
			found,
		)
	}

	// --- AuthKey ---
	if c.AuthKey == "" {
		return fmt.Errorf(
			"[CONFIG ERROR] 'auth_key' is missing.\n"+
				"  Expected: a Tailscale auth key starting with \"tskey-auth-\"\n\n"+
				"  Fix: Generate a new key at https://login.tailscale.com/admin/settings/keys\n"+
				"       Select \"Auth Key\" and check \"Ephemeral\"",
		)
	}
	if !strings.HasPrefix(c.AuthKey, "tskey-auth-") {
		// Show prefix only (first 15 chars max) for security
		preview := c.AuthKey
		if len(preview) > 15 {
			preview = preview[:15] + "..."
		}
		return fmt.Errorf(
			"[CONFIG ERROR] 'auth_key' is invalid.\n"+
				"  Expected: a Tailscale auth key starting with \"tskey-auth-\"\n"+
				"  Found:    %q (wrong key type or format)\n\n"+
				"  Fix: Generate a new key at https://login.tailscale.com/admin/settings/keys\n"+
				"       Select \"Auth Key\" and check \"Ephemeral\"",
			preview,
		)
	}

	// --- TargetIP (required for client mode only) ---
	if c.Role == "client" {
		if c.TargetIP == "" {
			return fmt.Errorf(
				"[CONFIG ERROR] 'target_ip' is required in client mode.\n"+
					"  Expected: the host's Tailscale mesh IP (e.g. \"100.64.0.1\")\n\n"+
					"  Fix: Run AnyPortal on your home PC in server mode first.\n"+
					"       Copy the mesh IP from its console output into config.json",
			)
		}
		if !tailscaleIPRegex.MatchString(c.TargetIP) {
			return fmt.Errorf(
				"[CONFIG ERROR] 'target_ip' is not a valid Tailscale IP.\n"+
					"  Expected: an address in the 100.x.x.x range (e.g. \"100.64.0.1\")\n"+
					"  Found:    %q\n\n"+
					"  Fix: Use the mesh IP printed by AnyPortal on your home PC.\n"+
					"       Tailscale mesh IPs always start with 100.",
				c.TargetIP,
			)
		}
	}
	// target_ip in server mode is silently ignored (allows shared config files)

	return nil
}
