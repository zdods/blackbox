package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// loadConfig reads a key = value config file at path.
// Missing keys are returned as empty strings (not an error).
// A missing file is also not an error.
func loadConfig(path string) (bastionURL, token, hostedPath string, err error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", "", nil
		}
		return "", "", "", err
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
		case "token":
			token = val
		case "hosted_path":
			hostedPath = val
		}
	}
	return bastionURL, token, hostedPath, scanner.Err()
}

// saveConfig writes the daemon config to path with 0600 permissions (owner-read-only).
func saveConfig(path, bastionURL, token, hostedPath string) error {
	content := fmt.Sprintf("bastion_url = %s\ntoken = %s\nhosted_path = %s\n", bastionURL, token, hostedPath)
	return os.WriteFile(path, []byte(content), 0600)
}
