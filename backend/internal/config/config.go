package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Driver is the SQL engine kind for a target connection registered in the
// metadata store. Chatdb's own metadata always lives in SQLite.
type Driver string

const (
	DriverPostgres Driver = "postgres"
	DriverMySQL    Driver = "mysql"
)

// Config is the full chatdb runtime configuration loaded from a JSON file.
type Config struct {
	Listen    string   `json:"listen"`
	JWTSecret string   `json:"jwt_secret"`
	AppKey    string   `json:"app_key"`
	Metadata Metadata `json:"metadata"`
	// JSONKeyAuthDisabledRemoved: legacy "auth_disabled" key is still accepted in JSON
	// (DisallowUnknownFields) but ignored; the feature was removed.
	JSONKeyAuthDisabledRemoved bool `json:"auth_disabled,omitempty"`
}

// Metadata controls where chatdb stores its own users and connection registry.
// Always a local SQLite file so the binary stays self-contained.
type Metadata struct {
	Path string `json:"path"`
}

// Load reads and validates the config at path, applying defaults.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var c Config
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

const defaultListen = "127.0.0.1:6366"

// DefaultConfigPath returns the path to chatdb.config.json under the OS user
// config directory (e.g. %APPDATA%\chatdb on Windows, ~/.config/chatdb on Linux).
func DefaultConfigPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(base, "chatdb", "chatdb.config.json"), nil
}

// LoadOrCreate loads an existing config file, or creates it with random secrets
// and a co-located metadata SQLite path, then loads it.
func LoadOrCreate(path string) (*Config, error) {
	if _, err := os.Stat(path); err == nil {
		return Load(path)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat config %s: %w", path, err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	metaPath := filepath.Join(dir, "chatdb.meta.sqlite")
	jwtSecret, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	appKey, err := randomHex(16)
	if err != nil {
		return nil, err
	}

	initial := Config{
		Listen:    defaultListen,
		JWTSecret: jwtSecret,
		AppKey:    appKey,
		Metadata:  Metadata{Path: metaPath},
	}
	raw, err := json.MarshalIndent(initial, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal new config: %w", err)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return Load(path)
		}
		return nil, fmt.Errorf("create config %s: %w", path, err)
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write config %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("close config %s: %w", path, err)
	}

	return Load(path)
}

func randomHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (c *Config) applyDefaults() {
	if c.Listen == "" {
		c.Listen = defaultListen
	}
	if c.Metadata.Path == "" {
		c.Metadata.Path = "chatdb.meta.sqlite"
	}
}

func (c *Config) validate() error {
	if c.JWTSecret == "" {
		return errors.New("jwt_secret is required")
	}
	if len(c.AppKey) != 32 {
		return errors.New("app_key must be exactly 32 bytes (used as AES-256 key)")
	}
	if strings.TrimSpace(c.Metadata.Path) == "" {
		return errors.New("metadata.path is required (path to chatdb's SQLite metadata file)")
	}
	return nil
}
