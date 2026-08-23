package store

import (
	"testing"

	"github.com/tespio/go-rdap-server/internal/config"
)

func TestNewPostgresStoreInvalidDSN(t *testing.T) {
	// An unparseable DSN fails fast at ParseConfig, before any network I/O.
	if _, err := NewPostgresStore(config.StorageConfig{DSN: "://bad"}); err == nil {
		t.Error("expected error for invalid DSN")
	}
	if _, err := NewPostgresStore(config.StorageConfig{DSN: ""}); err == nil {
		t.Error("expected error for empty DSN")
	}
}
