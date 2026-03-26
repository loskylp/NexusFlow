// Package auth provides authentication middleware, password hashing, and session token management.
// Authentication model: server-side Redis sessions with opaque 256-bit tokens.
// Tokens are issued as HTTP-only cookies (web GUI) and Bearer tokens (API clients).
// See: ADR-006, TASK-003
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/internal/queue"
	"golang.org/x/crypto/bcrypt"
)

// contextKey is an unexported type for context keys set by auth middleware,
// preventing collisions with other packages.
type contextKey int

const (
	// sessionContextKey is the key under which the *models.Session is stored in the request context.
	sessionContextKey contextKey = iota
)

// bcryptCost is the bcrypt cost factor per ADR-006.
const bcryptCost = 12

// sessionCookieName is the HTTP-only cookie name used for web GUI session tokens.
const sessionCookieName = "session"

// HashPassword produces a bcrypt hash of the given plaintext password using cost factor 12.
// See: ADR-006, TASK-003
//
// Args:
//
//	password: The plaintext password. Must not be empty.
//
// Returns:
//
//	The bcrypt hash string stored in the users table as password_hash.
//	An error if bcrypt fails (e.g. cost factor out of range).
//
// Postconditions:
//   - On success: VerifyPassword(password, returned hash) returns nil.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword checks whether plaintext matches the given bcrypt hash.
// See: ADR-006, TASK-003
//
// Args:
//
//	password: The plaintext password provided at login.
//	hash:     The bcrypt hash stored in the users table.
//
// Returns:
//
//	nil if the password matches the hash.
//	ErrInvalidCredentials if they do not match.
//	Any other error indicates a bcrypt internal failure.
func VerifyPassword(password, hash string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}

// GenerateToken produces a cryptographically random 256-bit session token, hex-encoded.
// See: ADR-006, TASK-003
//
// Returns:
//
//	A 64-character hex string suitable for use as a Redis session key suffix.
//	An error if the OS random source is unavailable.
func GenerateToken() (string, error) {
	raw := make([]byte, 32) // 256 bits = 32 bytes
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// Middleware validates the session token on every protected HTTP request.
// It reads the token from the HTTP-only cookie (web GUI) or the Authorization: Bearer header (API).
// On validation success, the *models.Session is stored in the request context
// under sessionContextKey. On failure, it writes 401 Unauthorized and stops the chain.
//
// See: ADR-006, TASK-003
//
// Args:
//
//	sessions: The SessionStore used to look up session tokens in Redis.
//
// Returns:
//
//	An http.Handler middleware that wraps the next handler.
//
// Preconditions:
//   - sessions must be non-nil and connected.
//
// Postconditions:
//   - On valid token: next handler is called with Session in context.
//   - On missing, invalid, or expired token: 401 is written; next handler is not called.
func Middleware(sessions queue.SessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			sess, err := sessions.Get(r.Context(), token)
			if err != nil || sess == nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), sessionContextKey, sess)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns a middleware that enforces that the authenticated user has
// at least the given role. Must be composed after Middleware.
//
// Args:
//
//	role: The minimum required role (e.g. models.RoleAdmin).
//
// Postconditions:
//   - On matching or higher role: next handler is called.
//   - On insufficient role: 403 Forbidden is written; next handler is not called.
func RequireRole(role models.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess := SessionFromContext(r.Context())
			if sess == nil || !hasRole(sess.Role, role) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SessionFromContext retrieves the Session stored by the auth Middleware.
// Returns nil if no session is present (i.e., the request was not authenticated).
// Callers within protected routes can assume a non-nil return; use this in unprotected
// routes only when optional auth is needed.
//
// Args:
//
//	ctx: The request context set by the auth Middleware.
func SessionFromContext(ctx context.Context) *models.Session {
	val := ctx.Value(sessionContextKey)
	if val == nil {
		return nil
	}
	sess, _ := val.(*models.Session)
	return sess
}

// ErrInvalidCredentials is returned by VerifyPassword when the password does not match.
// Handlers should map this to a 401 response without logging the plaintext attempt.
var ErrInvalidCredentials = &authError{"invalid credentials"}

type authError struct{ msg string }

func (e *authError) Error() string { return e.msg }

// extractToken reads the session token from the request.
// It prefers the Authorization: Bearer header; falls back to the session cookie.
// Returns an empty string if no token is found.
func extractToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		return cookie.Value
	}
	return ""
}

// hasRole returns true when userRole satisfies the required minimum role.
// Role hierarchy: admin > user.
func hasRole(userRole, required models.Role) bool {
	if userRole == required {
		return true
	}
	// Admin satisfies any role requirement.
	if userRole == models.RoleAdmin {
		return true
	}
	return false
}
