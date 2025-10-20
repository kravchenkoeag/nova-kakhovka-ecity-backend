// pkg/auth/jwt.go

package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTManager struct {
	secretKey     string
	tokenDuration time.Duration
}

// Claims представляє JWT payload
type Claims struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	Role        string `json:"role"`         // ✅ ДОДАНО
	IsModerator bool   `json:"is_moderator"` // Legacy support
	jwt.RegisteredClaims
}

func NewJWTManager(secretKey string, tokenDuration time.Duration) *JWTManager {
	return &JWTManager{
		secretKey:     secretKey,
		tokenDuration: tokenDuration,
	}
}

// ✅ ОНОВЛЕНО: Додано параметр role
func (m *JWTManager) GenerateToken(userID string, email string, role string, isModerator bool) (string, error) {
	// Створюємо claims з усіма полями
	claims := Claims{
		UserID:      userID,
		Email:       email,
		Role:        role,
		IsModerator: isModerator,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.tokenDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	// Створюємо токен
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Підписуємо токен
	tokenString, err := token.SignedString([]byte(m.secretKey))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// ValidateToken перевіряє та розшифровує JWT токен
func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	// Парсимо токен
	token, err := jwt.ParseWithClaims(
		tokenString,
		&Claims{},
		func(token *jwt.Token) (interface{}, error) {
			// Перевіряємо метод підпису
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(m.secretKey), nil
		},
	)

	if err != nil {
		return nil, err
	}

	// Отримуємо claims
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}

// RefreshToken оновлює токен
func (m *JWTManager) RefreshToken(oldToken string) (string, error) {
	// Валідуємо старий токен
	claims, err := m.ValidateToken(oldToken)
	if err != nil {
		return "", err
	}

	// Генеруємо новий токен з тими ж claims
	return m.GenerateToken(
		claims.UserID,
		claims.Email,
		claims.Role,
		claims.IsModerator,
	)
}
