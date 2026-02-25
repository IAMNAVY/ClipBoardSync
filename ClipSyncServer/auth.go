package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// ============================================================================
// Configuration
// ============================================================================

var jwtSecret = []byte(getEnv("JWT_SECRET", "clipboard-sync-secret-key-change-me"))

const (
	accessTokenExpiry  = 15 * time.Minute
	refreshTokenExpiry = 7 * 24 * time.Hour // 7 days
)

// ============================================================================
// JWT helpers
// ============================================================================

func generateAccessToken(userID uint, username string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":    userID,
		"username":   username,
		"token_type": "access",
		"exp":        time.Now().Add(accessTokenExpiry).Unix(),
		"iat":        time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func generateRefreshToken(userID uint, username string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":    userID,
		"username":   username,
		"token_type": "refresh",
		"exp":        time.Now().Add(refreshTokenExpiry).Unix(),
		"iat":        time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// generateTokenPair creates an access + refresh token pair.
func generateTokenPair(userID uint, username string) (accessToken, refreshToken string, err error) {
	accessToken, err = generateAccessToken(userID, username)
	if err != nil {
		return "", "", err
	}
	refreshToken, err = generateRefreshToken(userID, username)
	if err != nil {
		return "", "", err
	}
	return accessToken, refreshToken, nil
}

// parseToken validates a token string and returns (userID, username, error).
// It does NOT check token_type — the caller should verify if needed.
func parseToken(tokenStr string) (uint, string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return 0, "", fmt.Errorf("invalid token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, "", fmt.Errorf("invalid claims")
	}
	userIDFloat, ok := claims["user_id"].(float64)
	if !ok {
		return 0, "", fmt.Errorf("invalid user_id")
	}
	username, _ := claims["username"].(string)
	return uint(userIDFloat), username, nil
}

// parseRefreshToken validates a refresh token specifically.
func parseRefreshToken(tokenStr string) (uint, string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return 0, "", fmt.Errorf("invalid refresh token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, "", fmt.Errorf("invalid claims")
	}
	tokenType, _ := claims["token_type"].(string)
	if tokenType != "refresh" {
		return 0, "", fmt.Errorf("not a refresh token")
	}
	userIDFloat, ok := claims["user_id"].(float64)
	if !ok {
		return 0, "", fmt.Errorf("invalid user_id")
	}
	username, _ := claims["username"].(string)
	return uint(userIDFloat), username, nil
}

// isTokenExpiredError checks if a JWT error is specifically an expiration error.
func isTokenExpiredError(err error) bool {
	return strings.Contains(err.Error(), "token is expired")
}

// ============================================================================
// JWT Middleware
// ============================================================================

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid authorization header"})
			c.Abort()
			return
		}
		tokenStr := strings.TrimPrefix(auth, "Bearer ")

		// Try to parse first; if it fails, check if it's an expiration error
		// so the client can distinguish "expired" from "invalid"
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return jwtSecret, nil
		})

		if err != nil {
			if isTokenExpiredError(err) {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "token_expired"})
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			}
			c.Abort()
			return
		}

		if !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		// Reject refresh tokens used as access tokens
		if tokenType, _ := claims["token_type"].(string); tokenType == "refresh" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		userIDFloat, ok := claims["user_id"].(float64)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}
		username, _ := claims["username"].(string)

		c.Set("user_id", uint(userIDFloat))
		c.Set("username", username)
		c.Next()
	}
}
