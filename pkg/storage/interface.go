package storage

import (
	"context"
	"io"
	"time"
)

type FileInfo struct {
	Key          string
	LastModified time.Time
}

type Provider interface {
	Name() string
	Save(ctx context.Context, filename string, data io.Reader) error
	List(ctx context.Context) ([]FileInfo, error)
	Delete(ctx context.Context, key string) error
}