package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"socialai/model"
	"socialai/service"
)

type UserHandlers struct {
	users *service.UserService
	auth  *Authenticator
}

func NewUserHandlers(users *service.UserService, auth *Authenticator) *UserHandlers {
	return &UserHandlers{users: users, auth: auth}
}

func decodeUser(w http.ResponseWriter, r *http.Request) (model.User, bool) {
	var user model.User
	var input *model.User
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		status := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeAuthError(w, status, "Invalid registration or login data.")
		return user, false
	}
	if input == nil {
		writeAuthError(w, http.StatusBadRequest, "Send one JSON object.")
		return user, false
	}
	user = *input
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeAuthError(w, http.StatusBadRequest, "Send one JSON object.")
		return user, false
	}
	return user, true
}

func (h *UserHandlers) Signup(w http.ResponseWriter, r *http.Request) {
	user, ok := decodeUser(w, r)
	if !ok {
		return
	}
	username, err := h.users.Register(r.Context(), user)
	if err != nil {
		var validation service.ValidationError
		switch {
		case errors.As(err, &validation):
			writeAuthError(w, http.StatusBadRequest, validation.Error())
		case errors.Is(err, service.ErrUsernameTaken):
			writeAuthError(w, http.StatusConflict, err.Error())
		default:
			writeAuthError(w, http.StatusInternalServerError, "Registration is temporarily unavailable.")
		}
		return
	}
	writeAuthJSON(w, http.StatusCreated, map[string]string{"username": username})
}

func (h *UserHandlers) Signin(w http.ResponseWriter, r *http.Request) {
	user, ok := decodeUser(w, r)
	if !ok {
		return
	}
	username, err := h.users.Authenticate(r.Context(), user.Username, user.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			writeAuthError(w, http.StatusUnauthorized, "Invalid username or password.")
		} else {
			writeAuthError(w, http.StatusInternalServerError, "Sign-in is temporarily unavailable.")
		}
		return
	}
	token, err := h.auth.Issue(username)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "Sign-in is temporarily unavailable.")
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]string{"token": token, "username": username})
}

func (h *UserHandlers) Me(w http.ResponseWriter, r *http.Request) {
	username, ok := AuthenticatedUsername(r)
	if !ok {
		writeAuthError(w, http.StatusUnauthorized, "Please sign in to continue.")
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]string{"username": username})
}
