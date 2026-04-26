package database_test

import (
	"testing"

	"mangahub/pkg/database"
)

func TestConnect_CreatesAllTables(t *testing.T) {
	db, err := database.Connect(":memory:")
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer db.Close()

	tables := []string{"users", "manga", "user_progress"}
	for _, table := range tables {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}
