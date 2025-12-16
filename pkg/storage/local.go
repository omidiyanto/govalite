package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	// "time"  <-- Dihapus karena tidak digunakan langsung
)

type LocalStorage struct {
	BasePath string
	Prefix   string
}

func (l *LocalStorage) Name() string { return "LocalDisk" }

func (l *LocalStorage) Save(ctx context.Context, filename string, data io.Reader) error {
	fullPath := filepath.Join(l.BasePath, filename)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0750); err != nil {
		return err
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, data)
	return err
}

func (l *LocalStorage) List(ctx context.Context) ([]FileInfo, error) {
	var files []FileInfo

	err := filepath.WalkDir(l.BasePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Hitung relative path agar sesuai dengan format Key (filename)
		relPath, err := filepath.Rel(l.BasePath, path)
		if err != nil {
			return nil
		}
		// Fix separator windows jika perlu
		relPath = filepath.ToSlash(relPath)

		// Cek apakah file ini match dengan prefix yang diinginkan
		if strings.HasPrefix(relPath, l.Prefix) {
			info, err := d.Info()
			if err != nil {
				return nil
			}
			files = append(files, FileInfo{
				Key:          relPath,
				LastModified: info.ModTime(),
			})
		}
		return nil
	})

	return files, err
}

func (l *LocalStorage) Delete(ctx context.Context, key string) error {
	fullPath := filepath.Join(l.BasePath, key)
	return os.Remove(fullPath)
}