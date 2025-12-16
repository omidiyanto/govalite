package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"govalite-lightweight-vault-raft-snapshot-agent/pkg/config"
	"govalite-lightweight-vault-raft-snapshot-agent/pkg/storage"
	"govalite-lightweight-vault-raft-snapshot-agent/pkg/vault"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()

	loc, err := time.LoadLocation(cfg.SnapshotTimezone)
	if err != nil {
		logger.Error("Invalid Timezone", "timezone", cfg.SnapshotTimezone, "error", err)
		os.Exit(1)
	}

	logger.Info("Starting Vault Raft Snapshot Agent",
		"frequency", cfg.SnapshotFreq,
		"timezone", cfg.SnapshotTimezone,
		"prefix", cfg.SnapshotPrefix,
		"retain", cfg.SnapshotRetain,
		"enable_s3", cfg.EnableS3,
		"enable_local", cfg.EnableLocal,
	)

	vClient, err := vault.NewClient(cfg)
	if err != nil {
		logger.Error("Failed to create vault client", "error", err)
		os.Exit(1)
	}

	var storages []storage.Provider

	if cfg.EnableLocal {
		storages = append(storages, &storage.LocalStorage{BasePath: cfg.LocalStorage, Prefix: cfg.SnapshotPrefix})
		logger.Info("Storage enabled", "type", "Local", "path", cfg.LocalStorage)
	}

	if cfg.EnableS3 {
		s3Store, err := storage.NewS3Storage(context.Background(), cfg.S3Bucket, cfg.S3Region, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Endpoint, cfg.SnapshotPrefix)
		if err != nil {
			logger.Error("Failed to init S3", "error", err)
			os.Exit(1)
		}
		storages = append(storages, s3Store)
		logger.Info("Storage enabled", "type", "S3", "bucket", cfg.S3Bucket)
	}

	if len(storages) == 0 {
		logger.Error("No storage provider enabled! Please set ENABLE_S3=true or ENABLE_LOCAL=true.")
		os.Exit(1) 
	}

	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("Shutting down agent...")
		cancel()
	}()

	runDynamicScheduler(ctx, vClient, storages, cfg, loc)
}

func runDynamicScheduler(ctx context.Context, v *vault.Client, storages []storage.Provider, cfg *config.Config, loc *time.Location) {
	logger := slog.Default()

	for {
		if ctx.Err() != nil {
			return
		}

		lastSnapshotTime := getLastSnapshotFromAllStorages(ctx, storages)

		nextSchedule := lastSnapshotTime.Add(cfg.SnapshotFreq)
		now := time.Now()
		var waitDuration time.Duration

		if lastSnapshotTime.IsZero() {
			logger.Info("No existing snapshots found. Scheduling immediate execution.")
			waitDuration = 0
		} else if now.After(nextSchedule) {
			logger.Info("Snapshot schedule is OVERDUE.", "last_snapshot", lastSnapshotTime.In(loc), "next_run", nextSchedule.In(loc))
			waitDuration = 0
		} else {
			waitDuration = nextSchedule.Sub(now)
			logger.Info("Schedule synchronized.", "last_snapshot", lastSnapshotTime.In(loc), "next_run", nextSchedule.In(loc), "sleep_for", waitDuration.Round(time.Second))
		}

		if waitDuration > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(waitDuration):
			}
		}

		processSnapshot(ctx, v, storages, cfg, loc)

		time.Sleep(5 * time.Second)
	}
}

func processSnapshot(ctx context.Context, v *vault.Client, storages []storage.Provider, cfg *config.Config, loc *time.Location) {
	logger := slog.Default()

	isLeader, err := v.IsLeader()
	if err != nil {
		logger.Error("Failed to check leader status", "error", err)
		return
	}
	if !isLeader {
		logger.Debug("I am STANDBY. Skipping snapshot task.")
		return
	}

	logger.Info("I am LEADER. Initiating snapshot procedure...")

	latestCheck := getLastSnapshotFromAllStorages(ctx, storages)
	if !latestCheck.IsZero() && time.Since(latestCheck) < (cfg.SnapshotFreq/2) {
		logger.Warn("Snapshot already found in storage recently. Skipping.", "found_at", latestCheck.In(loc))
		return
	}

	tmpFile, err := os.CreateTemp("", "vault-snapshot-*.snap")
	if err != nil {
		logger.Error("Failed to create temp file", "error", err)
		return
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	start := time.Now()
	if err := v.TakeSnapshot(ctx, tmpFile); err != nil {
		logger.Error("Failed to take snapshot from Vault", "error", err)
		return
	}
	duration := time.Since(start)

	info, _ := tmpFile.Stat()
	logger.Info("Snapshot taken successfully", "size_bytes", info.Size(), "duration", duration.String())

	timestampName := time.Now().In(loc).Format("02-01-2006-15-04-05")
	filename := fmt.Sprintf("%s%s.snap", cfg.SnapshotPrefix, timestampName)

	for _, s := range storages {
		if _, err := tmpFile.Seek(0, 0); err != nil {
			logger.Error("Failed to seek temp file", "error", err)
			continue
		}

		logger.Info("Uploading snapshot", "destination", s.Name(), "filename", filename)
		if err := s.Save(ctx, filename, tmpFile); err != nil {
			logger.Error("Failed to save snapshot", "destination", s.Name(), "error", err)
		} else {
			logger.Info("Snapshot uploaded successfully", "destination", s.Name())

			// F. RETENTION POLICY
			if cfg.SnapshotRetain > 0 {
				if err := enforceRetention(ctx, s, cfg.SnapshotRetain); err != nil {
					logger.Warn("Failed to enforce retention policy", "destination", s.Name(), "error", err)
				}
			}
		}
	}
}

func enforceRetention(ctx context.Context, provider storage.Provider, maxRetain int) error {
	files, err := provider.List(ctx)
	if err != nil {
		return err
	}
	if len(files) <= maxRetain {
		return nil
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].LastModified.After(files[j].LastModified)
	})
	toDelete := files[maxRetain:]
	slog.Info("Enforcing retention", "storage", provider.Name(), "keep", maxRetain, "deleting", len(toDelete))
	for _, f := range toDelete {
		if err := provider.Delete(ctx, f.Key); err != nil {
			slog.Warn("Failed to delete old snapshot", "key", f.Key, "error", err)
		} else {
			slog.Debug("Deleted old snapshot", "key", f.Key)
		}
	}
	return nil
}

func getLastSnapshotFromAllStorages(ctx context.Context, storages []storage.Provider) time.Time {
	var latestGlobal time.Time
	for _, s := range storages {
		files, err := s.List(ctx)
		if err != nil {
			slog.Warn("Failed to list storage for timer check", "storage", s.Name(), "error", err)
			continue
		}
		for _, f := range files {
			if f.LastModified.After(latestGlobal) {
				latestGlobal = f.LastModified
			}
		}
	}
	return latestGlobal
}