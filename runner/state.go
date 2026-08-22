package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// forwardState persists macOS userland forwarder rules in the host-managed
// plugin store, separate from replaceable plugin code.

type ForwardRule struct {
	ID            string `json:"id"`
	ListenAddress string `json:"listenAddress"`
	ListenPort    int    `json:"listenPort"`
	Protocol      string `json:"protocol"`
	TargetIP      string `json:"targetIP"`
	TargetPort    int    `json:"targetPort"`
	PID           int    `json:"pid,omitempty"`
	CreatedAt     string `json:"createdAt"`
}

type forwardStateFile struct {
	Forwards []ForwardRule `json:"forwards"`
}

// userConfigDir is a seam for tests; production code uses os.UserConfigDir.
var userConfigDir = os.UserConfigDir

func legacyNetworkStateDir() (string, error) {
	config, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(config, "alx-network"), nil
}

func networkStateDir() (string, error) {
	legacy, err := legacyNetworkStateDir()
	if err != nil {
		return "", err
	}
	return pluginStoreDir(legacy)
}

func forwardStatePath() (string, error) {
	dir, err := networkStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "forwards.json"), nil
}

func loadForwardState() ([]ForwardRule, error) {
	path, err := forwardStatePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []ForwardRule{}, nil
		}
		return nil, err
	}
	var state forwardStateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.Forwards == nil {
		state.Forwards = []ForwardRule{}
	}
	return state.Forwards, nil
}

func saveForwardState(entries []ForwardRule) error {
	path, err := forwardStatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(forwardStateFile{Forwards: entries}, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".new"
	if err := os.WriteFile(temporary, data, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
