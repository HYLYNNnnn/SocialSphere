package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"socialai/model"
)

const MaxMediaBytes int64 = 10 << 20

var ErrPostNotFound = errors.New("Post not found.")

type PostStore interface {
	SavePost(context.Context, *model.Post) error
	GetPost(context.Context, string) (*model.Post, error)
	SearchPosts(context.Context, string, string, int) ([]model.Post, error)
	DeletePost(context.Context, string) error
}

type MediaStore interface {
	SaveMedia(context.Context, string, string, io.Reader) error
	OpenMedia(context.Context, string) (io.ReadCloser, string, error)
	DeleteMedia(context.Context, string) error
}

type PostService struct {
	posts PostStore
	media MediaStore
}

func NewPostService(posts PostStore, media MediaStore) *PostService {
	return &PostService{posts: posts, media: media}
}

func ValidateMedia(data []byte) (string, string, error) {
	if len(data) == 0 || int64(len(data)) > MaxMediaBytes {
		return "", "", ValidationError("Select a media file no larger than 10 MiB.")
	}
	contentType := http.DetectContentType(data)
	switch contentType {
	case "image/jpeg", "image/png", "image/gif":
		config, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 8192 || config.Height > 8192 || int64(config.Width)*int64(config.Height) > 16000000 {
			return "", "", ValidationError("Invalid image or image dimensions are too large.")
		}
		if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
			return "", "", ValidationError("Invalid image file.")
		}
		return "image", contentType, nil
	case "video/mp4", "video/webm":
		if len(data) < 32 {
			return "", "", ValidationError("Invalid video file.")
		}
		return "video", contentType, nil
	default:
		return "", "", ValidationError("Supported files: JPEG, PNG, GIF, MP4 and WebM.")
	}
}

func (s *PostService) Create(ctx context.Context, username, message string, media []byte) (*model.Post, error) {
	message = strings.TrimSpace(message)
	if message == "" || !utf8.ValidString(message) || utf8.RuneCountInString(message) > 2000 {
		return nil, ValidationError("Message must contain 1–2000 characters.")
	}
	kind, contentType, err := ValidateMedia(media)
	if err != nil {
		return nil, err
	}
	post := &model.Post{Id: uuid.NewString(), User: username, Message: message, Type: kind, CreatedAt: time.Now().UTC()}
	post.Url = "/api/media/" + post.Id
	if err := s.media.SaveMedia(ctx, post.Id, contentType, bytes.NewReader(media)); err != nil {
		return nil, err
	}
	if err := s.posts.SavePost(ctx, post); err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if cleanupErr := s.media.DeleteMedia(cleanup, post.Id); cleanupErr != nil {
			return nil, errors.Join(err, cleanupErr)
		}
		return nil, err
	}
	return post, nil
}

func (s *PostService) Search(ctx context.Context, username, keywords string, limit int) ([]model.Post, error) {
	if utf8.RuneCountInString(keywords) > 200 || !utf8.ValidString(keywords) {
		return nil, ValidationError("Search keywords must be at most 200 characters.")
	}
	if limit < 1 || limit > 100 {
		return nil, ValidationError("Search limit must be between 1 and 100.")
	}
	posts, err := s.posts.SearchPosts(ctx, NormalizeUsername(username), strings.TrimSpace(keywords), limit)
	if posts == nil && err == nil {
		posts = []model.Post{}
	}
	return posts, err
}

func (s *PostService) Delete(ctx context.Context, username, id string) error {
	post, err := s.posts.GetPost(ctx, id)
	if err != nil {
		return err
	}
	if post == nil || post.User != username {
		return ErrPostNotFound
	}
	// Keep metadata on media errors so the owner can retry. Across cloud stores,
	// deleting media and index metadata is not a distributed transaction.
	if err := s.media.DeleteMedia(ctx, id); err != nil {
		return err
	}
	return s.posts.DeletePost(ctx, id)
}
