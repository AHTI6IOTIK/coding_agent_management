package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const (
	defaultAppPort      = "8080"
	defaultDatabasePath = "gallery.db"
	defaultMetricsPort  = "9090"
	defaultPprofPort    = "6060"
	applicationVersion  = "v1.0.0"
)

type Config struct {
	AppAddress     string
	DatabasePath   string
	MetricsAddress string
	PprofAddress   string
	Region         string
	Version        string
}

func Load() (Config, error) {
	appPort, err := port("APP_PORT", defaultAppPort)
	if err != nil {
		return Config{}, err
	}

	metricsPort, err := port("METRICS_PORT", defaultMetricsPort)
	if err != nil {
		return Config{}, err
	}

	pprofPort, err := port("PPROF_PORT", defaultPprofPort)
	if err != nil {
		return Config{}, err
	}

	if appPort == metricsPort || appPort == pprofPort || metricsPort == pprofPort {
		return Config{}, fmt.Errorf("service ports must be distinct")
	}

	databasePath, err := value("DB_PATH", defaultDatabasePath)
	if err != nil {
		return Config{}, err
	}

	region, err := value("APP_REGION", "")
	if err != nil {
		return Config{}, err
	}
	if region == "" {
		return Config{}, fmt.Errorf("APP_REGION must not be empty")
	}

	return Config{
		AppAddress:     net.JoinHostPort("", appPort),
		DatabasePath:   databasePath,
		MetricsAddress: net.JoinHostPort("127.0.0.1", metricsPort),
		PprofAddress:   net.JoinHostPort("127.0.0.1", pprofPort),
		Region:         region,
		Version:        applicationVersion,
	}, nil
}

func port(name, defaultValue string) (string, error) {
	value, err := value(name, defaultValue)
	if err != nil {
		return "", err
	}

	parsed, err := strconv.ParseUint(value, 10, 16)
	if err != nil || parsed == 0 {
		return "", fmt.Errorf("%s must be a port number between 1 and 65535", name)
	}

	return value, nil
}

func value(name, defaultValue string) (string, error) {
	value, exists := os.LookupEnv(name)
	if !exists {
		return defaultValue, nil
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s must not be empty", name)
	}

	return value, nil
}
