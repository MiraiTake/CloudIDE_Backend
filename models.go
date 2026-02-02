package main

import (
	"time"
	"github.com/golang-jwt/jwt/v4"
)

// JWT-ключ
var jwtKey = []byte("")

// Claims для JWT (Access Token)
type Claims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

// RefreshToken модель
type RefreshToken struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Token       string    `json:"token"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
	LastUsedAt  time.Time `json:"last_used_at"`
	DeviceInfo  string    `json:"device_info,omitempty"`
	IPAddress   string    `json:"ip_address,omitempty"`
	Revoked     bool      `json:"revoked"`
}

// User-модель
type User struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	Email       string  `json:"email"`
	Password    string  `json:"password"`
	GitHubLogin *string `json:"github_login"`
	GitHubToken string  `json:"github_token"`
}

// Project-модель
type Project struct {
	ID        string `json:"id"`         // CHAR(36)
	UserID    string `json:"user_id"`    // CHAR(36)
	Name      string `json:"name"`       // VARCHAR(255)
	DockerID  string `json:"docker_id"`  // VARCHAR(255)
	UpdatedAt string `json:"updated_at"` // время последнего редактирования
}

// Структуры для статистики проектов
type projectStat struct {
	Name string `json:"name"`
	At   string `json:"at"`
}

type StatsResponse struct {
	Total   int         `json:"total"`
	First   projectStat `json:"first"`
	Last    projectStat `json:"last"`
	Updated projectStat `json:"updated"`
}

// Payload для "изменённых файлов"
type ChangedFilesPayload struct {
	Files []string `json:"files"`
}

// TokenPair - пара токенов для ответа
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // секунды до истечения access token
}