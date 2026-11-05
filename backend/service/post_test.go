package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"

	"socialai/model"
)

type failedPostStore struct{}

func (failedPostStore) SavePost(context.Context, *model.Post) error {
	return errors.New("index unavailable")
}
func (failedPostStore) GetPost(context.Context, string) (*model.Post, error) { return nil, nil }
func (failedPostStore) SearchPosts(context.Context, string, string, int) ([]model.Post, error) {
	return nil, nil
}
func (failedPostStore) DeletePost(context.Context, string) error { return nil }

type trackingMedia struct {
	saved, deleted      string
	cleanupContextError error
	cancel              context.CancelFunc
}

func (m *trackingMedia) SaveMedia(ctx context.Context, id, kind string, r io.Reader) error {
	m.saved = id
	if m.cancel != nil {
		m.cancel()
	}
	return nil
}
func (m *trackingMedia) OpenMedia(context.Context, string) (io.ReadCloser, string, error) {
	return nil, "", errors.New("not used")
}
func (m *trackingMedia) DeleteMedia(ctx context.Context, id string) error {
	m.deleted = id
	m.cleanupContextError = ctx.Err()
	return nil
}

func fixturePNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestFailedIndexWriteCompensatesMediaEvenAfterRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	media := &trackingMedia{cancel: cancel}
	posts := NewPostService(failedPostStore{}, media)
	if _, err := posts.Create(ctx, "alice", "a photo", fixturePNG(t)); err == nil {
		t.Fatal("index failure accepted")
	}
	if media.saved == "" || media.saved != media.deleted || media.cleanupContextError != nil {
		t.Fatal("media was not cleaned with an independent context")
	}
}

func TestValidateMediaRejectsDisguisedAndInvalidFiles(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("<html><script>bad</script></html>"), []byte("not a png")} {
		if _, _, err := ValidateMedia(data); err == nil {
			t.Fatal("invalid media accepted")
		}
	}
	data := fixturePNG(t)
	if _, _, err := ValidateMedia(data[:24]); err == nil {
		t.Fatal("truncated image accepted")
	}
	kind, mime, err := ValidateMedia(data)
	if err != nil || kind != "image" || mime != "image/png" {
		t.Fatal("valid PNG rejected", err)
	}
}
