// Package testhelpers provides shared utilities for tests and the testserver
// fixture-backed binary.
package testhelpers

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// LoadFixture reads a JSON file and extracts the .data field from the
// Synology/UniFi response envelope ({"data": T, ...}).
func LoadFixture[T any](t *testing.T, path string) T {
	t.Helper()
	v, err := readFixture[T](path)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return v
}

// MustLoadFixture is the panic-on-error variant of LoadFixture, intended for
// non-test binaries such as cmd/testserver.
func MustLoadFixture[T any](path string) T {
	v, err := readFixture[T](path)
	if err != nil {
		panic(err)
	}
	return v
}

func readFixture[T any](path string) (T, error) {
	var zero T
	data, err := os.ReadFile(path)
	if err != nil {
		return zero, fmt.Errorf("read fixture %s: %w", path, err)
	}
	var envelope struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return zero, fmt.Errorf("parse fixture %s: %w", path, err)
	}
	return envelope.Data, nil
}
