package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
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

func (c *Config) applyDefaults() {
	if c.Listen == "" {
		c.Listen = "127.0.0.1:3000"
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
