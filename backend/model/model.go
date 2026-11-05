package model

import "time"

type Post struct {
	Id        string    `json:"id"`
	User      string    `json:"user"`
	Message   string    `json:"message"`
	Url       string    `json:"url"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
}

// Password is a bcrypt hash in storage. HTTP responses use separate safe DTOs.
type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Age      int64  `json:"age"`
	Gender   string `json:"gender"`
}
