package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRejectsMalformedDatasetBeforeConfiguration(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-dir", t.TempDir()}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run() error = nil")
	}
	if !strings.Contains(err.Error(), "recipes.csv") {
		t.Fatalf("run() error = %q, want recipes.csv context", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() stdout = %q, want empty", stdout.String())
	}
}
