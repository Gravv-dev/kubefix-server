package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"math/big"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"kubefix-labs/backend/internal/db"
)

// Session claims issued by KubeFix Labs.
type sessionClaims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

type Service struct {
	store           *db.Store
	oauthConf       *oauth2.Config
	jwtSecret       []byte
	cookieName      string
	cookieDomain    string
	secureCookie    bool
	idTokenVerifier *IDTokenVerifier
}

// NewService builds the auth service.
func NewService(store *db.Store) *Service {
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	redirectURL := os.Getenv("GOOGLE_REDIRECT_URL")
	if redirectURL == "" {
		redirectURL = "http://localhost:8080/api/auth/google/callback"
	}

	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}

	secret := []byte(os.Getenv("JWT_SECRET"))
	if len(secret) == 0 {
		log.Println("warning: JWT_SECRET not set; using insecure default (set a real secret in production)")
		secret = []byte("insecure-development-secret-change-me")
	}

	secure := os.Getenv("COOKIE_SECURE") != "false" // secure by default
	domain := os.Getenv("COOKIE_DOMAIN")

	idTokenVerifier := NewIDTokenVerifier()

	return &Service{
		store:           store,
		oauthConf:       conf,
		jwtSecret:       secret,
		cookieName:      "kubefix_session",
		cookieDomain:    domain,
		secureCookie:    secure,
		idTokenVerifier: idTokenVerifier,
	}
}

// HandleGoogleLogin exchanges the Google authorization code for tokens, validates the ID token,
// creates/updates the user record, and issues a secure session cookie.
// Note: the frontend posts the authorization code; we never trust a frontend-supplied user ID.
func (s *Service) HandleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" {
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		return
	}

	token, err := s.oauthConf.Exchange(context.Background(), body.Code)
	if err != nil {
		log.Printf("oauth exchange failed: %v", err)
		http.Error(w, "failed to exchange authorization code", http.StatusBadRequest)
		return
	}

	if !token.Valid() {
		http.Error(w, "invalid token from Google", http.StatusBadRequest)
		return
	}

	// Verify the ID token signature and audience using Google's public keys (JWKS).
	idToken, ok := token.Extra("id_token").(string)
	if !ok || idToken == "" {
		http.Error(w, "no id_token in token response", http.StatusBadRequest)
		return
	}

	payload, err := s.idTokenVerifier.Verify(r.Context(), idToken)
	if err != nil {
		log.Printf("id token verification failed: %v", err)
		http.Error(w, "invalid Google id token", http.StatusUnauthorized)
		return
	}

	claims := s.idTokenVerifier.Payload(payload)
	if claims == nil {
		http.Error(w, "invalid token claims", http.StatusUnauthorized)
		return
	}

	// The Google "sub" is the stable provider subject identifier. Trust only this.
	sub, _ := claims["sub"].(string)
	if sub == "" {
		http.Error(w, "google id token missing subject", http.StatusBadRequest)
		return
	}

	emailStr, _ := claims["email"].(string)
	nameStr, _ := claims["name"].(string)
	pictureStr, _ := claims["picture"].(string)

	user, err := s.store.GetOrCreateUser(r.Context(), sub, emailStr, nameStr, pictureStr)
	if err != nil {
		log.Printf("get or create user failed: %v", err)
		http.Error(w, "failed to create user record", http.StatusInternalServerError)
		return
	}

	if err := s.setSessionCookie(w, user.ID); err != nil {
		log.Printf("set session cookie failed: %v", err)
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"user":  user,
		"token": token.AccessToken,
	})
}

// HandleLogout clears the session cookie.
func (s *Service) HandleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Value:    "",
		Path:     "/",
		Domain:   s.cookieDomain,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "logged out"})
}

// HandleMe returns the current authenticated user.
func (s *Service) HandleMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(ctxUserIDKey{}).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	user, err := s.store.GetUserByID(r.Context(), userID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"user": user})
}

// setSessionCookie issues a signed JWT session cookie bound to the user ID.
func (s *Service) setSessionCookie(w http.ResponseWriter, userID uuid.UUID) error {
	claims := &sessionClaims{
		UserID: userID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   userID.String(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Value:    signed,
		Path:     "/",
		Domain:   s.cookieDomain,
		Expires:  claims.ExpiresAt.Time,
		MaxAge:   int(time.Until(claims.ExpiresAt.Time).Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// ExtractSession validates the session cookie and returns the user ID.
func (s *Service) ExtractSession(r *http.Request) (uuid.UUID, error) {
	cookie, err := r.Cookie(s.cookieName)
	if err != nil {
		return uuid.Nil, errors.New("no session cookie")
	}
	token, err := jwt.ParseWithClaims(cookie.Value, &sessionClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return uuid.Nil, errors.New("invalid session")
	}
	claims, ok := token.Claims.(*sessionClaims)
	if !ok {
		return uuid.Nil, errors.New("invalid session claims")
	}
	return uuid.Parse(claims.UserID)
}

// Admin check: users with role "admin" can manage challenges.
func IsAdmin(role string) bool {
	return role == db.RoleAdmin
}

type ctxUserIDKey struct{}

// UserIDCtxKey is the context key for the authenticated user id.
var UserIDCtxKey = ctxUserIDKey{}

func WithUserID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, UserIDCtxKey, id)
}

// UserIDFromContext returns the authenticated user id from context, if present.
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	v := ctx.Value(UserIDCtxKey)
	id, ok := v.(uuid.UUID)
	return id, ok
}

// Store returns the underlying data store (used by admin authorization).
func (s *Service) Store() *db.Store {
	return s.store
}

// IDTokenVerifier validates Google OAuth2 ID tokens by verifying their signature
// against Google's public JWKS endpoint. It caches the JWKS and refreshes it periodically.
type IDTokenVerifier struct {
	mu       sync.RWMutex
	keys     map[string]any // kid -> *rsa.PublicKey
	updated  time.Time
	clientID string
}

const googleJWKSURL = "https://www.googleapis.com/oauth2/v3/certs"

// NewIDTokenVerifier creates a verifier that fetches Google's public keys.
func NewIDTokenVerifier() *IDTokenVerifier {
	return &IDTokenVerifier{
		keys:     make(map[string]any),
		clientID: os.Getenv("GOOGLE_CLIENT_ID"),
	}
}

// Verify parses and validates a Google ID token.
func (v *IDTokenVerifier) Verify(ctx context.Context, idToken string) (*jwt.Token, error) {
	token, err := jwt.Parse(idToken, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errors.New("unexpected signing method")
		}
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("missing kid in token header")
		}
		v.mu.RLock()
		key, exists := v.keys[kid]
		v.mu.RUnlock()
		if !exists {
			if err := v.refreshKeys(ctx); err != nil {
				return nil, err
			}
			v.mu.RLock()
			key, exists = v.keys[kid]
			v.mu.RUnlock()
			if !exists {
				return nil, errors.New("unknown key id")
			}
		}
		return key, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("token is not valid")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid token claims")
	}

	// Verify issuer.
	iss, _ := claims["iss"].(string)
	if iss != "accounts.google.com" && iss != "https://accounts.google.com" {
		return nil, errors.New("invalid issuer")
	}

	// Verify audience if a client ID is configured.
	if v.clientID != "" {
		aud := v.audience(claims)
		if !containsStr(aud, v.clientID) {
			return nil, errors.New("invalid audience")
		}
	}

	// Verify expiration.
	exp, _ := claims["exp"].(float64)
	if exp != 0 && time.Now().Unix() > int64(exp) {
		return nil, errors.New("token expired")
	}

	return token, nil
}

// Payload returns the verified claims as a map.
func (v *IDTokenVerifier) Payload(token *jwt.Token) map[string]any {
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		return claims
	}
	return nil
}

func (v *IDTokenVerifier) audience(claims jwt.MapClaims) []string {
	switch a := claims["aud"].(type) {
	case string:
		return []string{a}
	case []any:
		out := make([]string, 0, len(a))
		for _, x := range a {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (v *IDTokenVerifier) refreshKeys(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Avoid refreshing too frequently.
	if time.Since(v.updated) < 10*time.Minute && len(v.keys) > 0 {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleJWKSURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return err
	}

	keys := make(map[string]any, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pk, err := decodeJWKRSAPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = pk
	}
	if len(keys) == 0 {
		return errors.New("no usable keys in JWKS")
	}
	v.keys = keys
	v.updated = time.Now()
	return nil
}

// decodeJWKRSAPublicKey decodes a JWK RSA public key from base64url n and e parameters.
func decodeJWKRSAPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64URLDecode(nB64)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64URLDecode(eB64)
	if err != nil {
		return nil, err
	}
	n := new(big.Int).SetBytes(nBytes)
	e := 0
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}
	return &rsa.PublicKey{N: n, E: e}, nil
}

// base64URLDecode decodes a base64url-encoded string (with or without padding).
func base64URLDecode(s string) ([]byte, error) {
	if len(s)%4 == 0 {
		return base64.URLEncoding.DecodeString(s)
	}
	return base64.RawURLEncoding.DecodeString(s)
}
