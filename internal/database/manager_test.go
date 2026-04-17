package database

import (
	"testing"
)

func TestNewDatabaseManager(t *testing.T) {
	tests := []struct {
		name    string
		dsn     string
		wantErr bool
	}{
		{
			name:    "valid dsn",
			dsn:     "test.db",
			wantErr: false,
		},
		{
			name:    "empty dsn",
			dsn:     "",
			wantErr: false,
		},
		{
			name:    "path dsn",
			dsn:     "/tmp/test.db",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, err := NewDatabaseManager(tt.dsn)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewDatabaseManager(%q) error = %v, wantErr %v", tt.dsn, err, tt.wantErr)
				return
			}

			if mgr == nil {
				t.Errorf("NewDatabaseManager(%q) returned nil manager", tt.dsn)
			}
		})
	}
}

func TestNewDatabaseManager_ReturnsConcreteType(t *testing.T) {
	mgr, err := NewDatabaseManager("test.db")
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	if _, ok := mgr.(*sqliteManager); !ok {
		t.Errorf("NewDatabaseManager() returned %T, want *sqliteManager", mgr)
	}
}
