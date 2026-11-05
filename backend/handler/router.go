package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"socialai/service"
)

type RouterOptions struct {
	Users          *UserHandlers
	Auth           *Authenticator
	Posts          *service.PostService
	Media          service.MediaStore
	Images         service.ImageGenerator
	AllowedOrigins []string
	FrontendDir    string
}

func NewRouter(options RouterOptions) http.Handler {
	posts := NewPostHandlers(options.Posts, options.Media)
	images := NewImageHandler(options.Images)
	register := func(router *mux.Router) {
		router.HandleFunc("/signup", options.Users.Signup).Methods("POST")
		router.HandleFunc("/signin", options.Users.Signin).Methods("POST")
		router.Handle("/me", options.Auth.Middleware(http.HandlerFunc(options.Users.Me))).Methods("GET")
		router.Handle("/upload", options.Auth.Middleware(http.HandlerFunc(posts.Upload))).Methods("POST")
		router.Handle("/search", options.Auth.Middleware(http.HandlerFunc(posts.Search))).Methods("GET")
		router.Handle("/post/{id}", options.Auth.Middleware(http.HandlerFunc(posts.Delete))).Methods("DELETE")
		router.Handle("/generate-image", options.Auth.Middleware(http.HandlerFunc(images.Generate))).Methods("POST")
		router.HandleFunc("/media/{id}", posts.Media).Methods("GET", "HEAD")
		router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { writeAuthJSON(w, 200, map[string]string{"status": "ok"}) }).Methods("GET")
		router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeAuthError(w, 404, "Endpoint not found.") })
		router.MethodNotAllowedHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeAuthError(w, 405, "Method not allowed.") })
	}
	api := mux.NewRouter()
	register(api)
	root := mux.NewRouter()
	root.PathPrefix("/api/").Handler(http.StripPrefix("/api", api))
	register(root) // Keep the original root endpoint paths compatible.
	root.PathPrefix("/").Handler(spaHandler(options.FrontendDir)).Methods("GET", "HEAD")
	cors := handlers.CORS(handlers.AllowedOrigins(options.AllowedOrigins),
		handlers.AllowedHeaders([]string{"Authorization", "Content-Type"}),
		handlers.AllowedMethods([]string{"GET", "POST", "DELETE", "HEAD", "OPTIONS"}))(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		cors.ServeHTTP(w, r)
	})
}

func spaHandler(dir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Clean("/" + r.URL.Path)
		file := filepath.Join(dir, strings.TrimPrefix(path, "/"))
		stat, err := os.Stat(file)
		if err == nil && !stat.IsDir() {
			http.ServeFile(w, r, file)
			return
		}
		if filepath.Ext(path) != "" {
			http.NotFound(w, r)
			return
		}
		index := filepath.Join(dir, "index.html")
		if _, err := os.Stat(index); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Frontend build is not installed. Use the React development server or build the Docker image."})
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, index)
	})
}
