package vault

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"govalite-lightweight-vault-raft-snapshot-agent/pkg/config"

	"github.com/hashicorp/vault/api"
)

type Client struct {
	api      *api.Client
	roleID   string
	secretID string
	token    string
}

func NewClient(cfg *config.Config) (*Client, error) {
	apiConfig := api.DefaultConfig()
	apiConfig.Address = cfg.VaultAddr
	apiConfig.Timeout = 60 * time.Second
	if err := apiConfig.ConfigureTLS(&api.TLSConfig{
		Insecure: true,
	}); err != nil {
		return nil, fmt.Errorf("failed to configure TLS insecure: %w", err)
	}

	client, err := api.NewClient(apiConfig)
	if err != nil {
		return nil, err
	}

	c := &Client{
		api:      client,
		roleID:   cfg.VaultRoleID,
		secretID: cfg.VaultSecretID,
		token:    cfg.VaultToken,
	}

	if err := c.Authenticate(); err != nil {
		return nil, fmt.Errorf("initial auth failed: %w", err)
	}

	return c, nil
}

func (c *Client) Authenticate() error {
	if c.token != "" {
		c.api.SetToken(c.token)
		return nil
	}

	if c.roleID != "" && c.secretID != "" {
		slog.Info("Authenticating with AppRole...")
		data := map[string]interface{}{
			"role_id":   c.roleID,
			"secret_id": c.secretID,
		}
		secret, err := c.api.Logical().Write("auth/approle/login", data)
		if err != nil {
			return fmt.Errorf("approle login failed: %w", err)
		}
		if secret == nil || secret.Auth == nil {
			return fmt.Errorf("approle login returned no auth data")
		}
		c.api.SetToken(secret.Auth.ClientToken)
		slog.Info("AppRole authentication successful")
		return nil
	}

	return fmt.Errorf("no valid authentication method configured")
}

func checkAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "403") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "invalid token")
}

func (c *Client) IsLeader() (bool, error) {
	info, err := c.api.Sys().Leader()
	if checkAuthError(err) {
		slog.Warn("Encountered 403/Auth error during Leader check. Attempting re-authentication...")
		if loginErr := c.Authenticate(); loginErr == nil {
			info, err = c.api.Sys().Leader()
		} else {
			slog.Error("Re-authentication failed during Leader check", "error", loginErr)
		}
	}
	if err != nil {
		return false, fmt.Errorf("failed to get leader status: %w", err)
	}
	return info.IsSelf, nil
}

func (c *Client) TakeSnapshot(ctx context.Context, w io.Writer) error {
	err := c.api.Sys().RaftSnapshotWithContext(ctx, w)
	if err == nil {
		return nil
	}
	if !checkAuthError(err) {
		return err
	}
	slog.Warn("Token expired or permission denied during Snapshot. Re-authenticating...")
	if loginErr := c.Authenticate(); loginErr != nil {
		return fmt.Errorf("re-auth failed: %w (original error: %v)", loginErr, err)
	}
	slog.Info("Retrying snapshot with new token...")
	return c.api.Sys().RaftSnapshotWithContext(ctx, w)
}