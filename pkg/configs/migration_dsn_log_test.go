package configs

import (
	"strings"
	"testing"
)

func TestSafeMigrationDSNForLog(t *testing.T) {
	dsn := "postgres://user:secret@localhost:5432/mydb?sslmode=disable"
	safe := SafeMigrationDSNForLog(dsn)
	if safe == dsn {
		t.Fatalf("expected redacted dsn, got %q", safe)
	}
	if strings.Contains(safe, "secret") {
		t.Fatalf("password leaked in log dsn: %q", safe)
	}
}