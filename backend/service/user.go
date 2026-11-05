package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"socialai/model"

	"golang.org/x/crypto/bcrypt"
)

// UserStore must create atomically: false means the username already exists.
// Implementations store Password as a bcrypt hash, never as the submitted password.
type UserStore interface {
	GetUser(context.Context, string) (*model.User, error)
	CreateUser(context.Context, *model.User) (bool, error)
}

type UserService struct{ store UserStore }

var (
	ErrUsernameTaken      = errors.New("This username is already registered.")
	ErrInvalidCredentials = errors.New("Invalid username or password.")
	usernamePattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{2,31}$`)
)

type ValidationError string

func (e ValidationError) Error() string { return string(e) }

func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func NewUserService(store UserStore) *UserService { return &UserService{store: store} }

func (s *UserService) Register(ctx context.Context, input model.User) (string, error) {
	username := NormalizeUsername(input.Username)
	if !usernamePattern.MatchString(username) {
		return "", ValidationError("Username must be 3–32 letters, digits or underscores and start with a letter or digit.")
	}
	if !validPassword(input.Password) {
		return "", ValidationError("Password must contain at least 8 characters and no more than 72 UTF-8 bytes.")
	}
	if input.Age < 0 || input.Age > 150 || len(strings.TrimSpace(input.Gender)) > 64 {
		return "", ValidationError("Invalid profile information.")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	record := model.User{Username: username, Password: string(hash), Age: input.Age, Gender: strings.TrimSpace(input.Gender)}
	created, err := s.store.CreateUser(ctx, &record)
	if err != nil {
		return "", err
	}
	if !created {
		return "", ErrUsernameTaken
	}
	return username, nil
}

func validPassword(password string) bool {
	return utf8.ValidString(password) && utf8.RuneCountInString(password) >= 8 && len(password) <= 72 && strings.TrimSpace(password) != ""
}

// A valid dummy hash makes unknown-user and wrong-password checks use bcrypt.
// This value does not belong to an account and cannot authenticate a user.
const dummyPasswordHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

func (s *UserService) Authenticate(ctx context.Context, rawUsername, password string) (string, error) {
	username := NormalizeUsername(rawUsername)
	if !usernamePattern.MatchString(username) || len(password) > 72 || password == "" {
		return "", ErrInvalidCredentials
	}
	user, err := s.store.GetUser(ctx, username)
	if err != nil {
		return "", err
	}
	hash := dummyPasswordHash
	if user != nil {
		hash = user.Password
	}
	mismatch := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if user == nil || mismatch != nil {
		return "", ErrInvalidCredentials
	}
	return username, nil
}
