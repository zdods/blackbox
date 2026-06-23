package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "blackhaul-daemon"
	keyringAccount = "token"
	// keyringProbeAccount is a sentinel account used only by keyringAvailable
	// to test whether the keyring backend is reachable. It is never written.
	keyringProbeAccount = "__probe__"
)

// loadConfig reads a key = value config file at path.
// Missing keys are returned as empty strings (not an error).
// A missing file is also not an error.
func loadConfig(path string) (bastionURL, hostedPath string, err error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", nil
		}
		return "", "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "bastion_url":
			bastionURL = val
		case "hosted_path":
			hostedPath = val
		}
	}
	return bastionURL, hostedPath, scanner.Err()
}

// saveConfig writes bastion_url and hosted_path to path with 0600 permissions.
// The token is stored separately in the OS keyring — not written here.
func saveConfig(path, bastionURL, hostedPath string) error {
	content := fmt.Sprintf("bastion_url = %s\nhosted_path = %s\n", bastionURL, hostedPath)
	return os.WriteFile(path, []byte(content), 0600)
}

// keyringAvailable reports whether the OS keyring (Secret Service on Linux,
// Keychain on macOS, Credential Manager on Windows) is usable on this host.
// It probes with a read of a sentinel entry: ErrNotFound means the backend is
// reachable (the entry simply doesn't exist), while any other error — most
// commonly a missing D-Bus Secret Service on a headless Linux box — means the
// keyring can't be used and the token must be supplied another way.
func keyringAvailable() bool {
	_, err := keyring.Get(keyringService, keyringProbeAccount)
	return err == nil || err == keyring.ErrNotFound
}

// loadToken retrieves the daemon token from the OS keyring.
func loadToken() (string, error) {
	return keyring.Get(keyringService, keyringAccount)
}

// saveToken stores the daemon token in the OS keyring.
func saveToken(token string) error {
	return keyring.Set(keyringService, keyringAccount, token)
}

// deleteToken removes the daemon token from the OS keyring. It is idempotent:
// a missing entry is not an error. Returns whether a token was actually
// removed, so callers can report "removed" vs "nothing was stored".
func deleteToken() (removed bool, err error) {
	err = keyring.Delete(keyringService, keyringAccount)
	if err == keyring.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
