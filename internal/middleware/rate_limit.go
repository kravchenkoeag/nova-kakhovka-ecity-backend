// internal/middleware/rate_limit.go
package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// RateLimiter структура для управління rate limiting
type RateLimiter struct {
	requests map[primitive.ObjectID]time.Time
	mu       sync.RWMutex
}

var (
	// Глобальний instance rate limiter для poll створення
	pollRateLimiter = &RateLimiter{
		requests: make(map[primitive.ObjectID]time.Time),
	}
)

// RateLimitMiddleware middleware для обмеження частоти запитів від одного користувача
// Використовується для захисту від спаму при створенні опитувань
//
// Правила:
// - 5 хвилин між створенням опитувань одним користувачем
// - Автоматичне очищення старих записів (>1 години)
// - Безпечна робота з concurrent requests
func RateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Отримуємо ID користувача з контексту
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"details": "User authentication required",
			})
			c.Abort()
			return
		}

		// Перевіряємо тип user_id
		userIDObj, ok := userID.(primitive.ObjectID)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Internal error",
				"details": "Invalid user ID format",
			})
			c.Abort()
			return
		}

		// Блокуємо для безпечного доступу до map
		pollRateLimiter.mu.Lock()
		defer pollRateLimiter.mu.Unlock()

		// Перевіряємо останній запит від цього користувача
		if lastRequest, ok := pollRateLimiter.requests[userIDObj]; ok {
			timeSinceLastRequest := time.Since(lastRequest)

			// Якщо пройшло менше 5 хвилин - блокуємо запит
			if timeSinceLastRequest < 5*time.Minute {
				remaining := 5*time.Minute - timeSinceLastRequest

				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":               "Rate limit exceeded",
					"details":             "You can create a poll only once every 5 minutes",
					"retry_after_seconds": int(remaining.Seconds()),
					"retry_after":         remaining.Round(time.Second).String(),
				})
				c.Abort()
				return
			}
		}

		// Оновлюємо час останнього запиту
		pollRateLimiter.requests[userIDObj] = time.Now()

		// Запускаємо очищення старих записів в фоні
		go cleanupOldEntries(pollRateLimiter)

		// Продовжуємо обробку запиту
		c.Next()
	}
}

// cleanupOldEntries видаляє записи старші 1 години для економії пам'яті
func cleanupOldEntries(limiter *RateLimiter) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	cutoff := time.Now().Add(-1 * time.Hour)

	for userID, timestamp := range limiter.requests {
		if timestamp.Before(cutoff) {
			delete(limiter.requests, userID)
		}
	}
}

// GetRateLimitStatus повертає інформацію про rate limit для користувача
// Корисно для UI щоб показати коли користувач зможе створити наступний poll
func GetRateLimitStatus(userID primitive.ObjectID) (canCreate bool, waitTime time.Duration) {
	pollRateLimiter.mu.RLock()
	defer pollRateLimiter.mu.RUnlock()

	if lastRequest, ok := pollRateLimiter.requests[userID]; ok {
		timeSince := time.Since(lastRequest)
		if timeSince < 5*time.Minute {
			return false, 5*time.Minute - timeSince
		}
	}

	return true, 0
}

// ResetRateLimitForUser скидає rate limit для конкретного користувача
// Використовується тільки адміністраторами в особливих випадках
func ResetRateLimitForUser(userID primitive.ObjectID) {
	pollRateLimiter.mu.Lock()
	defer pollRateLimiter.mu.Unlock()

	delete(pollRateLimiter.requests, userID)
}

// ========================================
// АЛЬТЕРНАТИВНИЙ ЗАГАЛЬНИЙ RATE LIMITER
// ========================================

// GeneralRateLimiter загальний rate limiter для будь-яких endpoints
type GeneralRateLimiter struct {
	limit    int                                // Максимальна кількість запитів
	window   time.Duration                      // Часове вікно
	requests map[primitive.ObjectID][]time.Time // Історія запитів користувача
	mu       sync.RWMutex
}

// NewGeneralRateLimiter створює новий загальний rate limiter
func NewGeneralRateLimiter(limit int, window time.Duration) *GeneralRateLimiter {
	limiter := &GeneralRateLimiter{
		limit:    limit,
		window:   window,
		requests: make(map[primitive.ObjectID][]time.Time),
	}

	// Запускаємо фонове очищення
	go limiter.startCleanup()

	return limiter
}

// Middleware повертає Gin middleware для rate limiting
func (rl *GeneralRateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Отримуємо ID користувача
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized",
			})
			c.Abort()
			return
		}

		userIDObj, ok := userID.(primitive.ObjectID)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Invalid user ID",
			})
			c.Abort()
			return
		}

		rl.mu.Lock()
		defer rl.mu.Unlock()

		now := time.Now()
		cutoff := now.Add(-rl.window)

		// Отримуємо історію запитів користувача
		timestamps := rl.requests[userIDObj]

		// Фільтруємо тільки запити в межах вікна
		var validTimestamps []time.Time
		for _, ts := range timestamps {
			if ts.After(cutoff) {
				validTimestamps = append(validTimestamps, ts)
			}
		}

		// Перевіряємо ліміт
		if len(validTimestamps) >= rl.limit {
			oldestRequest := validTimestamps[0]
			retryAfter := rl.window - now.Sub(oldestRequest)

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":               "Rate limit exceeded",
				"limit":               rl.limit,
				"window":              rl.window.String(),
				"retry_after_seconds": int(retryAfter.Seconds()),
			})
			c.Abort()
			return
		}

		// Додаємо поточний запит
		validTimestamps = append(validTimestamps, now)
		rl.requests[userIDObj] = validTimestamps

		c.Next()
	}
}

// startCleanup запускає фонове очищення старих записів
func (rl *GeneralRateLimiter) startCleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()

		cutoff := time.Now().Add(-rl.window * 2)

		for userID, timestamps := range rl.requests {
			// Видаляємо користувачів без активності
			if len(timestamps) == 0 || timestamps[len(timestamps)-1].Before(cutoff) {
				delete(rl.requests, userID)
			}
		}

		rl.mu.Unlock()
	}
}
