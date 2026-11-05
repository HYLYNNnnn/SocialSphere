package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"golang.org/x/time/rate"
	"socialai/service"
)

type PostHandlers struct {
	posts *service.PostService
	media service.MediaStore
}

func NewPostHandlers(posts *service.PostService, media service.MediaStore) *PostHandlers {
	return &PostHandlers{posts: posts, media: media}
}

func (h *PostHandlers) Upload(w http.ResponseWriter, r *http.Request) {
	username, ok := AuthenticatedUsername(r)
	if !ok {
		writeAuthError(w, 401, "Please sign in to continue.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, service.MaxMediaBytes+(64<<10))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var max *http.MaxBytesError
		status := http.StatusBadRequest
		if errors.As(err, &max) {
			status = http.StatusRequestEntityTooLarge
		}
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
		writeAuthError(w, status, "Upload one supported media file no larger than 10 MiB.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["media_file"]) != 1 {
		writeAuthError(w, 400, "Select exactly one media file.")
		return
	}
	file, header, err := r.FormFile("media_file")
	if err != nil {
		writeAuthError(w, 400, "A media file is required.")
		return
	}
	defer file.Close()
	if header.Size > service.MaxMediaBytes {
		writeAuthError(w, 413, "The media file exceeds 10 MiB.")
		return
	}
	media, err := io.ReadAll(io.LimitReader(file, service.MaxMediaBytes+1))
	if err != nil {
		writeAuthError(w, 400, "The media file could not be read.")
		return
	}
	if int64(len(media)) > service.MaxMediaBytes {
		writeAuthError(w, 413, "The media file exceeds 10 MiB.")
		return
	}
	post, err := h.posts.Create(r.Context(), username, r.FormValue("message"), media)
	if err != nil {
		postError(w, err, "Unable to save the post. Please try again.")
		return
	}
	writeAuthJSON(w, http.StatusCreated, post)
}

func (h *PostHandlers) Search(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		number, err := strconv.Atoi(raw)
		if err != nil {
			writeAuthError(w, 400, "Invalid search limit.")
			return
		}
		limit = number
	}
	posts, err := h.posts.Search(r.Context(), r.URL.Query().Get("user"), r.URL.Query().Get("keywords"), limit)
	if err != nil {
		postError(w, err, "Unable to load posts. Please try again.")
		return
	}
	writeAuthJSON(w, http.StatusOK, posts)
}

func (h *PostHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	username, ok := AuthenticatedUsername(r)
	if !ok {
		writeAuthError(w, 401, "Please sign in to continue.")
		return
	}
	id := mux.Vars(r)["id"]
	if _, err := uuid.Parse(id); err != nil {
		writeAuthError(w, 404, "Post not found.")
		return
	}
	if err := h.posts.Delete(r.Context(), username, id); err != nil {
		postError(w, err, "Unable to delete the post. Please try again.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Posts contain public social media. GCS objects can stay private and are served by
// opaque IDs here; no bucket ACL changes or cloud credentials reach the browser.
func (h *PostHandlers) Media(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		writeAuthError(w, 404, "Media not found.")
		return
	}
	reader, contentType, err := h.media.OpenMedia(r.Context(), id)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeAuthError(w, 404, "Media not found.")
		} else {
			writeAuthError(w, 500, "Media is temporarily unavailable.")
		}
		return
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, service.MaxMediaBytes+1))
	if err != nil || int64(len(data)) > service.MaxMediaBytes {
		writeAuthError(w, 500, "Media is temporarily unavailable.")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, id, time.Time{}, bytes.NewReader(data))
}

func postError(w http.ResponseWriter, err error, fallback string) {
	var validation service.ValidationError
	if errors.As(err, &validation) {
		writeAuthError(w, 400, validation.Error())
		return
	}
	if errors.Is(err, service.ErrPostNotFound) {
		writeAuthError(w, 404, "Post not found.")
		return
	}
	writeAuthError(w, 500, fallback)
}

type ImageHandler struct {
	generator service.ImageGenerator
	limiter   *rate.Limiter
}

func NewImageHandler(generator service.ImageGenerator) *ImageHandler {
	return &ImageHandler{generator: generator, limiter: rate.NewLimiter(rate.Every(30*time.Second), 2)}
}
func (h *ImageHandler) Generate(w http.ResponseWriter, r *http.Request) {
	username, ok := AuthenticatedUsername(r)
	if !ok {
		writeAuthError(w, 401, "Please sign in to continue.")
		return
	}
	var input struct {
		Prompt string `json:"prompt"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		var tooLarge *http.MaxBytesError
		status := 400
		if errors.As(err, &tooLarge) {
			status = 413
		}
		writeAuthError(w, status, "Send an image description using 1–2000 characters.")
		return
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		writeAuthError(w, 400, "Send one JSON object.")
		return
	}
	input.Prompt = strings.TrimSpace(input.Prompt)
	if input.Prompt == "" || !utf8.ValidString(input.Prompt) || utf8.RuneCountInString(input.Prompt) > 2000 {
		writeAuthError(w, 400, "Describe an image using 1–2000 characters.")
		return
	}
	if h.generator == nil || !h.generator.Enabled() {
		writeAuthError(w, 503, "AI image generation is not configured on the server.")
		return
	}
	if !h.limiter.Allow() {
		w.Header().Set("Retry-After", "30")
		writeAuthError(w, 429, "Please wait before generating another image.")
		return
	}
	url, err := h.generator.Generate(r.Context(), username, input.Prompt)
	if err != nil {
		var validation service.ValidationError
		var imageError service.ImageError
		if errors.As(err, &validation) {
			writeAuthError(w, 400, validation.Error())
		} else if errors.As(err, &imageError) {
			writeAuthError(w, imageError.Status, imageError.Message)
		} else {
			writeAuthError(w, 502, "Image generation failed. Please try again.")
		}
		return
	}
	writeAuthJSON(w, 200, map[string]string{"image_url": url})
}
