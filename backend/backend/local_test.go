package backend

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"socialai/model"
)

func TestLocalPersistenceAndAtomicUserCreation(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	var count atomic.Int32
	var group sync.WaitGroup
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			created, err := store.CreateUser(context.Background(), &model.User{Username: "alice", Password: "stored-bcrypt-hash"})
			if err != nil {
				t.Error(err)
			}
			if created {
				count.Add(1)
			}
		}()
	}
	group.Wait()
	if count.Load() != 1 {
		t.Fatalf("created %d users", count.Load())
	}
	post := model.Post{Id: uuid.NewString(), User: "alice", Message: "A beach travel photo", CreatedAt: time.Now().UTC(), Type: "image"}
	if err := store.SavePost(context.Background(), &post); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	user, err := reloaded.GetUser(context.Background(), "alice")
	if err != nil || user == nil || user.Password != "stored-bcrypt-hash" {
		t.Fatalf("user did not persist: %v", err)
	}
	posts, err := reloaded.SearchPosts(context.Background(), "alice", "travel beach", 100)
	if err != nil || len(posts) != 1 {
		t.Fatalf("posts did not persist/search: %v", err)
	}
	key, err := LoadOrCreateSigningKey(dir)
	if err != nil || len(key) < 32 {
		t.Fatal("signing key not created", err)
	}
	second, err := LoadOrCreateSigningKey(dir)
	if err != nil || key != second {
		t.Fatal("signing key changed across restart")
	}
	mode, err := os.Stat(filepath.Join(dir, ".jwt-key"))
	if err != nil || mode.Mode().Perm() != 0600 {
		t.Fatal("signing key permissions")
	}
}

func TestLocalMetadataFailureDoesNotCommitInMemory(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "data")
	store, err := NewLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(parent, "offline")
	if err := os.Rename(dir, backup); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(backup, dir)
	if created, err := store.CreateUser(context.Background(), &model.User{Username: "alice"}); err == nil || created {
		t.Fatal("unavailable disk should fail registration")
	}
	user, _ := store.GetUser(context.Background(), "alice")
	if user != nil {
		t.Fatal("failed user remained in memory")
	}
	id := uuid.NewString()
	if err := store.SavePost(context.Background(), &model.Post{Id: id}); err == nil {
		t.Fatal("unavailable disk should fail posting")
	}
	post, _ := store.GetPost(context.Background(), id)
	if post != nil {
		t.Fatal("failed post remained in memory")
	}
}

type failingMediaReader struct{}

func (failingMediaReader) Read(p []byte) (int, error) { copy(p, "bad"); return 3, io.ErrUnexpectedEOF }
func TestLocalMediaRejectsPathsAndCleansPartialWrites(t *testing.T) {
	store, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMedia(context.Background(), "../outside", "image/png", strings.NewReader("bad")); err == nil {
		t.Fatal("unsafe path accepted")
	}
	id := uuid.NewString()
	if err := store.SaveMedia(context.Background(), id, "image/png", failingMediaReader{}); err == nil {
		t.Fatal("read failure accepted")
	}
	reader, _, err := store.OpenMedia(context.Background(), id)
	if reader != nil {
		reader.Close()
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial file remained: %v", err)
	}
}
