package backend

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestGCSRequiresBucketBeforeLookingForCredentials(t *testing.T) {
	if _, err := NewGCSBackend(context.Background(), "  "); err == nil {
		t.Fatal("expected missing bucket configuration to be rejected")
	}
}

func TestGCSMediaReaderReleasesItsContextWhenClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := &gcsMediaReader{ReadCloser: io.NopCloser(strings.NewReader("image")), cancel: cancel}
	if ctx.Err() != nil {
		t.Fatal("context was cancelled before streaming")
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != context.Canceled {
		t.Fatal("closing media must release its context")
	}
}
