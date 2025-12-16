package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	VaultAddr     string
	VaultToken    string
	VaultRoleID   string
	VaultSecretID string

	SnapshotFreq     time.Duration
	SnapshotPrefix   string
	SnapshotRetain   int
	SnapshotTimezone string 

	EnableS3    bool 
	EnableLocal bool 

	LocalStorage string

	S3Bucket    string
	S3Region    string
	S3AccessKey string
	S3SecretKey string
	S3Endpoint  string
}

func Load() *Config {
	return &Config{
		VaultAddr:     getEnv("VAULT_ADDR", "http://127.0.0.1:8200"),
		VaultToken:    getEnv("VAULT_TOKEN", ""),
		VaultRoleID:   getEnv("VAULT_ROLE_ID", ""),
		VaultSecretID: getEnv("VAULT_SECRET_ID", ""),

		SnapshotFreq:     getEnvDuration("SNAPSHOT_FREQUENCY", 1*time.Hour),
		SnapshotPrefix:   getEnv("SNAPSHOT_PREFIX", "raft-snapshot-"),
		SnapshotRetain:   getEnvInt("SNAPSHOT_RETAIN", 5),
		SnapshotTimezone: getEnv("SNAPSHOT_TIMEZONE", "UTC"), 

		EnableS3:    getEnvBool("ENABLE_S3", false),
		EnableLocal: getEnvBool("ENABLE_LOCAL", false),

		LocalStorage: getEnv("STORAGE_LOCAL_PATH", "./snapshots"),

		S3Bucket:    getEnv("STORAGE_S3_BUCKET", ""),
		S3Region:    getEnv("STORAGE_S3_REGION", "us-east-1"),
		S3AccessKey: getEnv("AWS_ACCESS_KEY_ID", ""),
		S3SecretKey: getEnv("AWS_SECRET_ACCESS_KEY", ""),
		S3Endpoint:  getEnv("AWS_ENDPOINT_URL", ""),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if val, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		val = strings.ToLower(strings.TrimSpace(val))
		if val == "true" || val == "1" || val == "yes" || val == "on" {
			return true
		}
		if val == "false" || val == "0" || val == "no" || val == "off" {
			return false
		}
	}
	return fallback
}