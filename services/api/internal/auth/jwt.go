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
	// Role is the caller's role in WorkspaceID (owner/admin/member/viewer),
	// snapshotted at issuance like IsAdmin — a role change via UpdateMember
	// takes effect on that user's next login/refresh, not mid-token.
	Role string `json:"role"`
	jwt.RegisteredClaims
}

func SignAccessToken(secret, userID, workspaceID, email string, isAdmin bool, role string) (string, error) {
	claims := Claims{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Email:       email,
		IsAdmin:     isAdmin,
		Role:        role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}
