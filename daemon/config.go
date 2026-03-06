package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "blackbox-daemon"
	keyringAccount = "token"
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

// loadToken retrieves the daemon token from the OS keyring.
func loadToken() (string, error) {
	return keyring.Get(keyringService, keyringAccount)
}

// saveToken stores the daemon token in the OS keyring.
func saveToken(token string) error {
	return keyring.Set(keyringService, keyringAccount, token)
}
