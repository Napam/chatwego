package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Napam/chatwego/db/generated/sqlc"
	"github.com/Napam/chatwego/lib/identity"
)

const (
	cookieName   = "session_token"
	cookieMaxAge = 30 * 24 * time.Hour // 30 days
	tokenByteLen = 32
)

type contextKey struct{}

type SessionData struct {
	IdentityID  string
	SessionHash []byte
}

func IdentityFromContext(ctx context.Context) string {
	if s, ok := ctx.Value(contextKey{}).(SessionData); ok {
		return s.IdentityID
	}
	// WARN: Just have it like this during early dev
	return identity.SystemIdentityID
}

func SessionDataFromContext(ctx context.Context) (SessionData, bool) {
	s, ok := ctx.Value(contextKey{}).(SessionData)
	return s, ok
}

func Middleware(logger *slog.Logger, queries *sqlc.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/static/") {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			now := time.Now().UTC().Format(time.RFC3339)

			// Try to read existing cookie
			cookie, err := r.Cookie(cookieName)
			if err == nil && cookie.Value != "" {
				tokenHash := hashToken(cookie.Value)
				token, err := queries.GetSessionTokenByHash(ctx, tokenHash)
				if err == nil {
					// Valid session found
					if token.ExpiresAt > now {
						_ = queries.UpdateSessionTokenLastUsed(
							ctx,
							sqlc.UpdateSessionTokenLastUsedParams{
								LastUsedAt: sql.NullString{String: now, Valid: true},
								Hash:       tokenHash,
							},
						)
						sd := SessionData{
							IdentityID:  token.IdentityID,
							SessionHash: tokenHash,
						}
						next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, contextKey{}, sd)))
						return
					}
					// Expired — revoke it
					_ = queries.RevokeSessionToken(ctx, sqlc.RevokeSessionTokenParams{
						RevokedAt: sql.NullString{String: now, Valid: true},
						Hash:      tokenHash,
					})
				} else if !errors.Is(err, sql.ErrNoRows) {
					logger.Error("Session token lookup failed", slog.Any("error", err))
				}
			}

			// No valid session — create one for the system identity
			token, tokenHash, err := createSystemSession(ctx, queries, now)
			if err != nil {
				logger.Error("Failed to create system session", slog.Any("error", err))
				next.ServeHTTP(w, r)
				return
			}

			setCookie(w, token)

			sd := SessionData{
				IdentityID:  identity.SystemIdentityID,
				SessionHash: tokenHash,
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, contextKey{}, sd)))
		})
	}
}

func createSystemSession(
	ctx context.Context,
	queries *sqlc.Queries,
	now string,
) (string, []byte, error) {
	// Generate a random token
	tokenBytes := make([]byte, tokenByteLen)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", nil, err
	}
	token := hex.EncodeToString(tokenBytes)
	tokenHash := hashToken(token)

	expiresAt := time.Now().UTC().Add(cookieMaxAge).Format(time.RFC3339)

	_, err := queries.CreateSessionToken(ctx, sqlc.CreateSessionTokenParams{
		Hash:       tokenHash,
		LoginID:    identity.SystemLoginID,
		IdentityID: identity.SystemIdentityID,
		LastUsedAt: sql.NullString{String: now, Valid: true},
		ExpiresAt:  expiresAt,
		CreatedAt:  now,
	})
	if err != nil {
		return "", nil, err
	}

	return token, tokenHash, nil
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func setCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(cookieMaxAge.Seconds()),
	})
}
