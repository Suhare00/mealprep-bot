// Package media stores recipe image bytes outside PostgreSQL.
package media

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Suhare00/mealprep-bot/internal/domain"
)

const (
	DefaultMaxBytes    int64 = 10 << 20
	maxPublishAttempts       = 4
)

// StoredFile is validated image-file metadata returned after a successful save.
type StoredFile struct {
	StorageKey domain.StorageKey
	MIMEType   domain.MediaType
	Width      int
	Height     int
	SizeBytes  int64
}

// Filesystem stores images beneath one fixed root using generated opaque keys.
type Filesystem struct {
	root     string
	maxBytes int64
	random   io.Reader
}

// NewFilesystem prepares a filesystem-backed image store with conservative limits.
func NewFilesystem(root string) (*Filesystem, error) {
	return newFilesystem(root, DefaultMaxBytes, rand.Reader)
}

func newFilesystem(root string, maxBytes int64, random io.Reader) (*Filesystem, error) {
	if !filepath.IsAbs(root) || maxBytes <= 0 || random == nil {
		return nil, errors.New("media root must be absolute and limits must be positive")
	}
	cleanRoot := filepath.Clean(root)
	if err := os.MkdirAll(cleanRoot, 0o750); err != nil {
		return nil, fmt.Errorf("create media root: %w", err)
	}
	if err := os.Chmod(cleanRoot, 0o750); err != nil {
		return nil, fmt.Errorf("secure media root: %w", err)
	}
	return &Filesystem{root: cleanRoot, maxBytes: maxBytes, random: random}, nil
}

// Save validates and atomically stores one JPEG or PNG image.
func (s *Filesystem) Save(ctx context.Context, expectedMIME domain.MediaType, source io.Reader) (StoredFile, error) {
	if err := ctx.Err(); err != nil {
		return StoredFile{}, err
	}
	if expectedMIME != domain.MediaTypeJPEG && expectedMIME != domain.MediaTypePNG {
		return StoredFile{}, fmt.Errorf("unsupported image MIME type %q", expectedMIME)
	}
	contents, err := io.ReadAll(io.LimitReader(source, s.maxBytes+1))
	if err != nil {
		return StoredFile{}, fmt.Errorf("read image: %w", err)
	}
	if int64(len(contents)) > s.maxBytes {
		return StoredFile{}, fmt.Errorf("image exceeds %d-byte limit", s.maxBytes)
	}
	if err := ctx.Err(); err != nil {
		return StoredFile{}, err
	}
	detected := domain.MediaType(http.DetectContentType(contents))
	if detected != expectedMIME {
		return StoredFile{}, fmt.Errorf("image content MIME %q does not match %q", detected, expectedMIME)
	}
	configuration, format, err := image.DecodeConfig(bytes.NewReader(contents))
	if err != nil {
		return StoredFile{}, fmt.Errorf("decode image metadata: %w", err)
	}
	if (format == "jpeg" && detected != domain.MediaTypeJPEG) || (format == "png" && detected != domain.MediaTypePNG) || (format != "jpeg" && format != "png") {
		return StoredFile{}, errors.New("decoded image format is unsupported")
	}
	if configuration.Width <= 0 || configuration.Height <= 0 ||
		configuration.Width > domain.MaxImageDimension || configuration.Height > domain.MaxImageDimension ||
		int64(configuration.Width)*int64(configuration.Height) > domain.MaxImagePixels {
		return StoredFile{}, errors.New("image dimensions exceed supported limits")
	}
	decoded, decodedFormat, err := image.Decode(bytes.NewReader(contents))
	if err != nil {
		return StoredFile{}, fmt.Errorf("decode image pixels: %w", err)
	}
	decodedBounds := decoded.Bounds()
	if decodedFormat != format || decodedBounds.Dx() != configuration.Width || decodedBounds.Dy() != configuration.Height {
		return StoredFile{}, errors.New("decoded image metadata is inconsistent")
	}

	temporary, err := os.CreateTemp(s.root, ".upload-*")
	if err != nil {
		return StoredFile{}, fmt.Errorf("create temporary image: %w", err)
	}
	temporaryName := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return StoredFile{}, fmt.Errorf("secure temporary image: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		return StoredFile{}, fmt.Errorf("write temporary image: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return StoredFile{}, fmt.Errorf("sync temporary image: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return StoredFile{}, fmt.Errorf("close temporary image: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return StoredFile{}, err
	}
	var key domain.StorageKey
	for attempt := 0; attempt < maxPublishAttempts; attempt++ {
		key, err = s.generatedKey(detected)
		if err != nil {
			return StoredFile{}, err
		}
		destination := s.path(key)
		err = os.Link(temporaryName, destination)
		if err == nil {
			if removeErr := os.Remove(temporaryName); removeErr != nil {
				if cleanupErr := os.Remove(destination); cleanupErr != nil {
					return StoredFile{}, fmt.Errorf("remove published image temporary link: %w (destination cleanup: %v)", removeErr, cleanupErr)
				}
				return StoredFile{}, fmt.Errorf("remove published image temporary link: %w", removeErr)
			}
			committed = true
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return StoredFile{}, fmt.Errorf("publish image: %w", err)
		}
	}
	if !committed {
		return StoredFile{}, fmt.Errorf("publish image: generated key collided after %d attempts", maxPublishAttempts)
	}
	return StoredFile{StorageKey: key, MIMEType: detected, Width: configuration.Width, Height: configuration.Height, SizeBytes: int64(len(contents))}, nil
}

// Open opens a stored image after validating that key is not a path.
func (s *Filesystem) Open(ctx context.Context, key domain.StorageKey) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := key.Validate(); err != nil {
		return nil, err
	}
	file, err := os.Open(s.path(key))
	if err != nil {
		return nil, fmt.Errorf("open image %q: %w", key, err)
	}
	return file, nil
}

// Delete removes a stored image. Missing files are already clean and succeed.
func (s *Filesystem) Delete(ctx context.Context, key domain.StorageKey) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := key.Validate(); err != nil {
		return err
	}
	err := os.Remove(s.path(key))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete image %q: %w", key, err)
	}
	return nil
}

func (s *Filesystem) generatedKey(mime domain.MediaType) (domain.StorageKey, error) {
	identifier := make([]byte, 16)
	if _, err := io.ReadFull(s.random, identifier); err != nil {
		return "", fmt.Errorf("generate media storage key: %w", err)
	}
	extension := ".jpg"
	if mime == domain.MediaTypePNG {
		extension = ".png"
	}
	key := domain.StorageKey(hex.EncodeToString(identifier) + extension)
	if err := key.Validate(); err != nil {
		return "", err
	}
	return key, nil
}

func (s *Filesystem) path(key domain.StorageKey) string {
	return filepath.Join(s.root, string(key))
}
