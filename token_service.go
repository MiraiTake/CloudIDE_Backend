package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

const (
	AccessTokenDuration  = 1 * time.Hour
	RefreshTokenDuration = 30 * 24 * time.Hour // 30 дней
)

// Генерация Access Token (JWT)
func generateAccessToken(userID string) (string, error) {
	expirationTime := time.Now().Add(AccessTokenDuration)
	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

// Генерация Refresh Token (случайная строка)
func generateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// Создание пары токенов (Access + Refresh)
func createTokenPair(userID, deviceInfo, ipAddress string) (*TokenPair, error) {
	// Генерируем Access Token
	accessToken, err := generateAccessToken(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	// Генерируем Refresh Token
	refreshToken, err := generateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Сохраняем Refresh Token в БД
	refreshID := uuid.NewString()
	expiresAt := time.Now().Add(RefreshTokenDuration)

	_, err = db.Exec(`
		INSERT INTO refresh_tokens 
		(id, user_id, token, expires_at, device_info, ip_address, revoked) 
		VALUES (?, ?, ?, ?, ?, ?, FALSE)`,
		refreshID, userID, refreshToken, expiresAt, deviceInfo, ipAddress,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	log.Printf("Создана пара токенов для пользователя %s", userID)

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(AccessTokenDuration.Seconds()),
	}, nil
}

// Валидация Refresh Token
func validateRefreshToken(refreshToken string) (*RefreshToken, error) {
	var rt RefreshToken
	err := db.QueryRow(`
		SELECT id, user_id, token, expires_at, created_at, last_used_at, 
		       device_info, ip_address, revoked
		FROM refresh_tokens 
		WHERE token = ? AND revoked = FALSE`,
		refreshToken,
	).Scan(
		&rt.ID, &rt.UserID, &rt.Token, &rt.ExpiresAt, &rt.CreatedAt,
		&rt.LastUsedAt, &rt.DeviceInfo, &rt.IPAddress, &rt.Revoked,
	)

	if err != nil {
		return nil, fmt.Errorf("refresh token not found or revoked: %w", err)
	}

	// Проверяем, не истёк ли токен
	if time.Now().After(rt.ExpiresAt) {
		return nil, fmt.Errorf("refresh token expired")
	}

	// Обновляем время последнего использования
	_, err = db.Exec(
		"UPDATE refresh_tokens SET last_used_at = ? WHERE id = ?",
		time.Now(), rt.ID,
	)
	if err != nil {
		log.Printf("Failed to update last_used_at for token %s: %v", rt.ID, err)
	}

	return &rt, nil
}

// Отзыв Refresh Token
func revokeRefreshToken(refreshToken string) error {
	result, err := db.Exec(
		"UPDATE refresh_tokens SET revoked = TRUE WHERE token = ?",
		refreshToken,
	)
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("token not found")
	}

	return nil
}

// Отзыв всех токенов пользователя
func revokeAllUserTokens(userID string) error {
	_, err := db.Exec(
		"UPDATE refresh_tokens SET revoked = TRUE WHERE user_id = ? AND revoked = FALSE",
		userID,
	)
	if err != nil {
		return fmt.Errorf("failed to revoke user tokens: %w", err)
	}

	log.Printf("Все токены пользователя %s отозваны", userID)
	return nil
}

// Очистка истёкших токенов (вызывать периодически)
func cleanupExpiredTokens() error {
	result, err := db.Exec(
		"DELETE FROM refresh_tokens WHERE expires_at < ? OR revoked = TRUE",
		time.Now().Add(-7*24*time.Hour), // удаляем токены, истёкшие более 7 дней назад
	)
	if err != nil {
		return fmt.Errorf("failed to cleanup tokens: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		log.Printf("Очищено %d истёкших/отозванных токенов", rows)
	}

	return nil
}

// Получение всех активных токенов пользователя (для отображения в профиле)
func getUserActiveSessions(userID string) ([]RefreshToken, error) {
	rows, err := db.Query(`
		SELECT id, user_id, token, expires_at, created_at, last_used_at, 
		       device_info, ip_address, revoked
		FROM refresh_tokens 
		WHERE user_id = ? AND revoked = FALSE AND expires_at > ?
		ORDER BY last_used_at DESC`,
		userID, time.Now(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sessions: %w", err)
	}
	defer rows.Close()

	var sessions []RefreshToken
	for rows.Next() {
		var rt RefreshToken
		if err := rows.Scan(
			&rt.ID, &rt.UserID, &rt.Token, &rt.ExpiresAt, &rt.CreatedAt,
			&rt.LastUsedAt, &rt.DeviceInfo, &rt.IPAddress, &rt.Revoked,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, rt)
	}

	return sessions, nil
}