package backend

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"socialai/model"
)

type localState struct {
	Users map[string]model.User `json:"users"`
	Posts map[string]model.Post `json:"posts"`
}

// LocalStore is a persistent, single-process development store.
// Cloud deployments can use the Elasticsearch and GCS adapters instead.
type LocalStore struct {
	mu    sync.RWMutex
	dir   string
	state localState
}

func NewLocalStore(dir string) (*LocalStore, error) {
	if err := os.MkdirAll(filepath.Join(dir, "media"), 0700); err != nil {
		return nil, err
	}
	store := &LocalStore{dir: dir, state: localState{Users: map[string]model.User{}, Posts: map[string]model.Post{}}}
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(data, &store.state); err != nil {
			return nil, fmt.Errorf("read local state: %w", err)
		}
		if store.state.Users == nil {
			store.state.Users = map[string]model.User{}
		}
		if store.state.Posts == nil {
			store.state.Posts = map[string]model.Post{}
		}
	}
	return store, nil
}

func (s *LocalStore) persistLocked() error {
	data, err := json.Marshal(s.state)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(s.dir, ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(s.dir, "state.json"))
}

func (s *LocalStore) GetUser(ctx context.Context, username string) (*model.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.state.Users[username]
	if !ok {
		return nil, nil
	}
	return &user, nil
}

func (s *LocalStore) CreateUser(ctx context.Context, user *model.User) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.state.Users[user.Username]; exists {
		return false, nil
	}
	s.state.Users[user.Username] = *user
	if err := s.persistLocked(); err != nil {
		delete(s.state.Users, user.Username)
		return false, err
	}
	return true, nil
}

func (s *LocalStore) SavePost(ctx context.Context, post *model.Post) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.state.Posts[post.Id]; exists {
		return errors.New("post already exists")
	}
	s.state.Posts[post.Id] = *post
	if err := s.persistLocked(); err != nil {
		delete(s.state.Posts, post.Id)
		return err
	}
	return nil
}

func (s *LocalStore) GetPost(ctx context.Context, id string) (*model.Post, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	post, ok := s.state.Posts[id]
	if !ok {
		return nil, nil
	}
	return &post, nil
}

func (s *LocalStore) SearchPosts(ctx context.Context, user, keywords string, limit int) ([]model.Post, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	posts := make([]model.Post, 0)
	terms := strings.Fields(strings.ToLower(keywords))
	for _, post := range s.state.Posts {
		if user != "" && post.User != user {
			continue
		}
		matches := true
		for _, term := range terms {
			if !strings.Contains(strings.ToLower(post.Message), term) {
				matches = false
				break
			}
		}
		if matches {
			posts = append(posts, post)
		}
	}
	sort.Slice(posts, func(i, j int) bool {
		if posts[i].CreatedAt.Equal(posts[j].CreatedAt) {
			return posts[i].Id > posts[j].Id
		}
		return posts[i].CreatedAt.After(posts[j].CreatedAt)
	})
	if limit > 0 && len(posts) > limit {
		posts = posts[:limit]
	}
	return posts, nil
}

func (s *LocalStore) DeletePost(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	post, exists := s.state.Posts[id]
	if !exists {
		return nil
	}
	delete(s.state.Posts, id)
	if err := s.persistLocked(); err != nil {
		s.state.Posts[id] = post
		return err
	}
	return nil
}

func (s *LocalStore) mediaPath(name string) (string, error) {
	id, err := uuid.Parse(name)
	if err != nil || id.String() != name {
		return "", os.ErrNotExist
	}
	return filepath.Join(s.dir, "media", name), nil
}

func (s *LocalStore) SaveMedia(ctx context.Context, name, contentType string, reader io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.mediaPath(name)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return errors.Join(copyErr, closeErr)
	}
	return nil
}

func (s *LocalStore) OpenMedia(ctx context.Context, name string) (io.ReadCloser, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	path, err := s.mediaPath(name)
	if err != nil {
		return nil, "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	header := make([]byte, 512)
	n, readErr := file.Read(header)
	if readErr != nil && readErr != io.EOF {
		file.Close()
		return nil, "", readErr
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return nil, "", err
	}
	return file, http.DetectContentType(header[:n]), nil
}

func (s *LocalStore) DeleteMedia(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.mediaPath(name)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func LoadOrCreateSigningKey(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, ".jwt-key")
	if data, err := os.ReadFile(path); err == nil {
		return strings.TrimSpace(string(data)), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	bytes := make([]byte, 48)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	key := base64.RawURLEncoding.EncodeToString(bytes)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	if _, err := io.WriteString(file, key); err != nil {
		file.Close()
		os.Remove(path)
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return key, nil
}
