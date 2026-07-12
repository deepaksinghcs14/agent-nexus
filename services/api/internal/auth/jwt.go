package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID      string `json:"sub"`
	WorkspaceID string `json:"workspace_id"`
	Email       string `json:"email"`
	IsAdmin     bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

func SignAccessToken(secret, userID, workspaceID, email string, isAdmin bool) (string, error) {
	claims := Claims{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Email:       email,
		IsAdmin:     isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}
