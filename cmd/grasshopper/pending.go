package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// pendingConnection holds only a short-lived device pairing secret. It is not a
// memory store and is removed after approval, denial, or expiry.
type pendingConnection struct {
	Address     string    `json:"address"`
	TokenPath   string    `json:"token_path"`
	Device      string    `json:"device"`
	Agents      string    `json:"agents"`
	CursorCLI   bool      `json:"cursor_cli"`
	CursorDir   string    `json:"cursor_dir"`
	Update      bool      `json:"update"`
	Secret      string    `json:"secret"`
	RequestID   string    `json:"request_id"`
	Code        string    `json:"code"`
	ApprovalURL string    `json:"approval_url"`
	ExpiresAt   time.Time `json:"expires_at"`
	ConfigHash  string    `json:"config_hash"`
}

func pendingPath(configPath string) string { return configPath + ".pairing" }

func connectionConfigHash(configPath string) (string, error) {
	data, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func loadPendingConnection(configPath string) (*pendingConnection, error) {
	path := pendingPath(configPath)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 4096 {
		return nil, errors.New("pairing state is not a private regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pending pendingConnection
	if err := json.Unmarshal(data, &pending); err != nil {
		return nil, err
	}
	if pending.Address == "" || pending.Secret == "" || len(pending.RequestID) != 64 || len(pending.Code) != 8 ||
		!filepath.IsAbs(pending.TokenPath) || pending.ExpiresAt.IsZero() {
		return nil, errors.New("pairing state is incomplete")
	}
	return &pending, nil
}

func savePendingConnection(configPath string, pending pendingConnection) error {
	path := pendingPath(configPath)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("pairing state already exists or cannot be saved: %w", err)
	}
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}
