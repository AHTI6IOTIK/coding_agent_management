package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	unsetEnv(t, "APP_PORT")
	unsetEnv(t, "DB_PATH")
	unsetEnv(t, "METRICS_PORT")
	unsetEnv(t, "PPROF_PORT")
	t.Setenv("APP_REGION", "x1")

	config, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if config.AppAddress != ":8080" || config.DatabasePath != "gallery.db" || config.MetricsAddress != "127.0.0.1:9090" || config.PprofAddress != "127.0.0.1:6060" || config.Region != "x1" || config.Version != "v1.0.0" {
		t.Errorf("Load() = %+v, want default configuration", config)
	}
}

func TestLoad(t *testing.T) {
	t.Setenv("APP_PORT", "8081")
	t.Setenv("DB_PATH", "test.db")
	t.Setenv("METRICS_PORT", "9091")
	t.Setenv("PPROF_PORT", "6061")
	t.Setenv("APP_REGION", "x1")

	config, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if config.AppAddress != ":8081" {
		t.Errorf("AppAddress = %q, want %q", config.AppAddress, ":8081")
	}
	if config.DatabasePath != "test.db" {
		t.Errorf("DatabasePath = %q, want %q", config.DatabasePath, "test.db")
	}
	if config.MetricsAddress != "127.0.0.1:9091" {
		t.Errorf("MetricsAddress = %q, want %q", config.MetricsAddress, "127.0.0.1:9091")
	}
	if config.PprofAddress != "127.0.0.1:6061" {
		t.Errorf("PprofAddress = %q, want %q", config.PprofAddress, "127.0.0.1:6061")
	}
	if config.Region != "x1" {
		t.Errorf("Region = %q, want %q", config.Region, "x1")
	}
}

func TestLoadRejectsDuplicatePorts(t *testing.T) {
	t.Setenv("APP_PORT", "8080")
	t.Setenv("METRICS_PORT", "8080")
	t.Setenv("PPROF_PORT", "6060")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error for duplicate ports")
	}
}

func TestLoadRequiresAppRegion(t *testing.T) {
	unsetEnv(t, "APP_REGION")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error for missing APP_REGION")
	}
}

func TestLoadRejectsEmptyAppRegion(t *testing.T) {
	t.Setenv("APP_REGION", " \t")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error for empty APP_REGION")
	}
}

func unsetEnv(t *testing.T, name string) {
	t.Helper()

	value, exists := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("Unsetenv(%q) error = %v", name, err)
	}
	t.Cleanup(func() {
		if !exists {
			_ = os.Unsetenv(name)
			return
		}
		_ = os.Setenv(name, value)
	})
}
