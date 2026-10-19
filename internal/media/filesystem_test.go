package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Suhare00/mealprep-bot/internal/domain"
)

func TestFilesystemSaveOpenDeleteAndRecreate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media")
	store, err := newFilesystem(root, DefaultMaxBytes, bytes.NewReader(bytes.Repeat([]byte{0x2a}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	contents := encodedPNG(t, 2, 3)
	stored, err := store.Save(t.Context(), domain.MediaTypePNG, bytes.NewReader(contents))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if stored.MIMEType != domain.MediaTypePNG || stored.Width != 2 || stored.Height != 3 || stored.SizeBytes != int64(len(contents)) || !strings.HasSuffix(string(stored.StorageKey), ".png") {
		t.Fatalf("Save() metadata = %#v", stored)
	}
	info, err := os.Stat(filepath.Join(root, string(stored.StorageKey)))
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Fatalf("stored file mode = %v, error = %v", info.Mode().Perm(), err)
	}

	restarted, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := restarted.Open(t.Context(), stored.StorageKey)
	if err != nil {
		t.Fatalf("Open() after recreation error = %v", err)
	}
	got, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(got, contents) {
		t.Fatalf("opened contents mismatch: read=%v close=%v", readErr, closeErr)
	}
	if err := restarted.Delete(t.Context(), stored.StorageKey); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := restarted.Delete(t.Context(), stored.StorageKey); err != nil {
		t.Fatalf("idempotent Delete() error = %v", err)
	}
}

func TestFilesystemAcceptsCompleteJPEG(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media")
	store, err := newFilesystem(root, DefaultMaxBytes, bytes.NewReader(bytes.Repeat([]byte{0x3c}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	contents := encodedJPEG(t, 4, 5)
	stored, err := store.Save(t.Context(), domain.MediaTypeJPEG, bytes.NewReader(contents))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if stored.MIMEType != domain.MediaTypeJPEG || stored.Width != 4 || stored.Height != 5 || !strings.HasSuffix(string(stored.StorageKey), ".jpg") {
		t.Fatalf("Save() metadata = %#v", stored)
	}
	got, err := os.ReadFile(filepath.Join(root, string(stored.StorageKey)))
	if err != nil || !bytes.Equal(got, contents) {
		t.Fatalf("stored JPEG mismatch: error=%v", err)
	}
}

func TestFilesystemRejectsInvalidImagesWithoutLeavingFiles(t *testing.T) {
	tests := []struct {
		name         string
		maxBytes     int64
		expectedMIME domain.MediaType
		contents     func(*testing.T) []byte
		cancel       bool
		wantError    string
	}{
		{name: "unsupported declared MIME", maxBytes: 1024, expectedMIME: "image/gif", contents: func(*testing.T) []byte { return []byte("GIF89a") }, wantError: "unsupported"},
		{name: "content mismatch", maxBytes: 1024, expectedMIME: domain.MediaTypeJPEG, contents: func(t *testing.T) []byte { return encodedPNG(t, 1, 1) }, wantError: "does not match"},
		{name: "malformed PNG payload", maxBytes: 1024, expectedMIME: domain.MediaTypePNG, contents: func(*testing.T) []byte { return append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 20)...) }, wantError: "decode"},
		{name: "truncated header-valid PNG", maxBytes: 1024, expectedMIME: domain.MediaTypePNG, contents: func(t *testing.T) []byte { return headerValidTruncation(t, encodedPNG(t, 2, 3)) }, wantError: "decode image pixels"},
		{name: "truncated header-valid JPEG", maxBytes: 1024, expectedMIME: domain.MediaTypeJPEG, contents: func(t *testing.T) []byte { return headerValidTruncation(t, encodedJPEG(t, 2, 3)) }, wantError: "decode image pixels"},
		{name: "too large", maxBytes: 8, expectedMIME: domain.MediaTypePNG, contents: func(t *testing.T) []byte { return encodedPNG(t, 1, 1) }, wantError: "byte limit"},
		{name: "dimensions", maxBytes: DefaultMaxBytes, expectedMIME: domain.MediaTypePNG, contents: func(t *testing.T) []byte { return encodedPNG(t, domain.MaxImageDimension+1, 1) }, wantError: "dimensions"},
		{name: "cancelled", maxBytes: 1024, expectedMIME: domain.MediaTypePNG, contents: func(t *testing.T) []byte { return encodedPNG(t, 1, 1) }, cancel: true, wantError: "context canceled"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "media")
			store, err := newFilesystem(root, test.maxBytes, bytes.NewReader(make([]byte, 16)))
			if err != nil {
				t.Fatal(err)
			}
			ctx := t.Context()
			if test.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			_, err = store.Save(ctx, test.expectedMIME, bytes.NewReader(test.contents(t)))
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Save() error = %v, want text %q", err, test.wantError)
			}
			entries, readErr := os.ReadDir(root)
			if readErr != nil || len(entries) != 0 {
				t.Fatalf("failed save left files: entries=%v error=%v", entries, readErr)
			}
		})
	}
}

func TestFilesystemRetriesGeneratedKeyCollisionWithoutOverwrite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media")
	firstRandom := bytes.Repeat([]byte{0x2a}, 16)
	secondRandom := bytes.Repeat([]byte{0x2b}, 16)
	store, err := newFilesystem(root, DefaultMaxBytes, bytes.NewReader(append(firstRandom, secondRandom...)))
	if err != nil {
		t.Fatal(err)
	}
	firstKey := strings.Repeat("2a", 16) + ".png"
	secondKey := strings.Repeat("2b", 16) + ".png"
	original := []byte("original image bytes")
	if err := os.WriteFile(filepath.Join(root, firstKey), original, 0o600); err != nil {
		t.Fatal(err)
	}
	upload := encodedPNG(t, 2, 2)

	stored, err := store.Save(t.Context(), domain.MediaTypePNG, bytes.NewReader(upload))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if string(stored.StorageKey) != secondKey {
		t.Fatalf("StorageKey = %q, want retry key %q", stored.StorageKey, secondKey)
	}
	assertFileContents(t, filepath.Join(root, firstKey), original)
	assertFileContents(t, filepath.Join(root, secondKey), upload)
	assertDirectoryEntries(t, root, 2)
}

func TestFilesystemStopsAfterBoundedKeyCollisions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media")
	collisionRandom := bytes.Repeat([]byte{0x4d}, 16)
	store, err := newFilesystem(root, DefaultMaxBytes, bytes.NewReader(bytes.Repeat(collisionRandom, maxPublishAttempts)))
	if err != nil {
		t.Fatal(err)
	}
	key := strings.Repeat("4d", 16) + ".png"
	original := []byte("must remain unchanged")
	if err := os.WriteFile(filepath.Join(root, key), original, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = store.Save(t.Context(), domain.MediaTypePNG, bytes.NewReader(encodedPNG(t, 1, 1)))
	if err == nil || !strings.Contains(err.Error(), "collided after") {
		t.Fatalf("Save() error = %v, want bounded collision error", err)
	}
	assertFileContents(t, filepath.Join(root, key), original)
	assertDirectoryEntries(t, root, 1)
}

func TestFilesystemRejectsTraversalAndPropagatesContext(t *testing.T) {
	store, err := NewFilesystem(filepath.Join(t.TempDir(), "media"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []domain.StorageKey{"../secret", "folder/image.png", "..\\secret"} {
		if _, err := store.Open(t.Context(), key); err == nil {
			t.Fatalf("Open(%q) error = nil", key)
		}
		if err := store.Delete(t.Context(), key); err == nil {
			t.Fatalf("Delete(%q) error = nil", key)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.Open(ctx, "safe.png"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Open(cancelled) error = %v", err)
	}
}

func encodedPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewNRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func encodedJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, image.NewNRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func headerValidTruncation(t *testing.T, complete []byte) []byte {
	t.Helper()
	for length := len(complete) - 1; length > 0; length-- {
		candidate := complete[:length]
		if _, _, err := image.DecodeConfig(bytes.NewReader(candidate)); err != nil {
			continue
		}
		if _, _, err := image.Decode(bytes.NewReader(candidate)); err != nil {
			return append([]byte(nil), candidate...)
		}
	}
	t.Fatal("could not produce a header-valid truncated image")
	return nil
}

func assertFileContents(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("file %s contents mismatch: error=%v", path, err)
	}
}

func assertDirectoryEntries(t *testing.T, root string, want int) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != want {
		t.Fatalf("directory entries = %v, error=%v; want %d", entries, err, want)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".upload-") {
			t.Fatalf("temporary upload was not cleaned up: %s", entry.Name())
		}
	}
}
