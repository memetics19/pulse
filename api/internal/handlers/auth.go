package handlers

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/generated"
)

type Auth struct {
	q       *generated.Queries
	secure  bool
	limiter *loginLimiter
}

func NewAuth(q *generated.Queries, secure bool) *Auth {
	return &Auth{q: q, secure: secure, limiter: newLoginLimiter()}
}

// dummyPasswordHash is verified against when the username does not exist, so
// failed logins take the same time for known and unknown usernames (no user
// enumeration via timing).
var dummyPasswordHash = sync.OnceValue(func() string {
	h, err := auth.HashPassword("pulse-dummy-password-for-timing")
	if err != nil {
		return ""
	}
	return h
})

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Code     string `json:"code"`
}

// currentUser resolves the authenticated user from the request's session cookie.
func (a *Auth) currentUser(r *http.Request) (generated.User, bool) {
	c, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		return generated.User{}, false
	}
	sess, err := a.q.GetSession(r.Context(), c.Value)
	if err != nil || !sess.ExpiresAt.After(time.Now()) {
		return generated.User{}, false
	}
	u, err := a.q.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		return generated.User{}, false
	}
	return u, true
}

func (a *Auth) Status(w http.ResponseWriter, r *http.Request) {
	// A DB error must not fall through to needs_setup=true: reporting an
	// already-configured instance as needing setup would invite a takeover
	// attempt via the setup endpoint.
	n, err := a.q.CountUsers(r.Context())
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	authenticated := false
	username := ""
	totpEnabled := false
	if u, ok := a.currentUser(r); ok {
		authenticated = true
		username = u.Username
		totpEnabled = u.TotpEnabled == 1
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"needs_setup":   n == 0,
		"authenticated": authenticated,
		"username":      username,
		"totp_enabled":  totpEnabled,
	})
}

func (a *Auth) Setup(w http.ResponseWriter, r *http.Request) {
	n, err := a.q.CountUsers(r.Context())
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	if n > 0 {
		http.Error(w, "already set up", http.StatusForbidden)
		return
	}
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil || c.Username == "" || len(c.Password) < 8 {
		http.Error(w, "username and password (min 8 chars) required", http.StatusBadRequest)
		return
	}
	hash, err := auth.HashPassword(c.Password)
	if err != nil {
		http.Error(w, "hash error", http.StatusInternalServerError)
		return
	}
	u, err := a.q.CreateUser(r.Context(), generated.CreateUserParams{Username: c.Username, PasswordHash: hash})
	if err != nil {
		http.Error(w, "could not create user", http.StatusInternalServerError)
		return
	}
	if !a.startSession(w, r, u.ID) {
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	if !a.limiter.allow(clientIP(r)) {
		http.Error(w, "too many attempts", http.StatusTooManyRequests)
		return
	}
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	u, err := a.q.GetUserByUsername(r.Context(), c.Username)
	if err != nil {
		// Burn the same hashing cost as a real verification so unknown
		// usernames are not distinguishable by response time.
		_, _ = auth.VerifyPassword(c.Password, dummyPasswordHash())
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	ok, err := auth.VerifyPassword(c.Password, u.PasswordHash)
	if err != nil || !ok {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if u.TotpEnabled == 1 {
		if c.Code == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]bool{"needs_2fa": true})
			return
		}
		if u.TotpSecret == nil || !auth.ValidateTOTP(*u.TotpSecret, c.Code) {
			http.Error(w, "invalid 2FA code", http.StatusUnauthorized)
			return
		}
	}
	if !a.startSession(w, r, u.ID) {
		return
	}
	w.WriteHeader(http.StatusOK)
}

// TwoFASetup generates a fresh TOTP secret for the current user and returns
// the provisioning URI and a scannable QR data URL. The secret is stored but
// 2FA stays disabled until the user confirms a code via TwoFAEnable.
func (a *Auth) TwoFASetup(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	secret, uri, err := auth.GenerateTOTP(u.Username)
	if err != nil {
		http.Error(w, "totp error", http.StatusInternalServerError)
		return
	}
	if err := a.q.SetUserTOTP(r.Context(), generated.SetUserTOTPParams{TotpSecret: &secret, TotpEnabled: 0, ID: u.ID}); err != nil {
		http.Error(w, "could not save", http.StatusInternalServerError)
		return
	}
	qr, _ := auth.TOTPQRDataURL(uri)
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret, "otpauth_url": uri, "qr": qr})
}

// TwoFAEnable confirms the pending TOTP secret with a valid code and enables 2FA.
func (a *Auth) TwoFAEnable(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if u.TotpSecret == nil {
		http.Error(w, "run setup first", http.StatusBadRequest)
		return
	}
	if !auth.ValidateTOTP(*u.TotpSecret, body.Code) {
		http.Error(w, "invalid code", http.StatusBadRequest)
		return
	}
	if err := a.q.SetUserTOTP(r.Context(), generated.SetUserTOTPParams{TotpSecret: u.TotpSecret, TotpEnabled: 1, ID: u.ID}); err != nil {
		http.Error(w, "could not enable", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// TwoFADisable clears the TOTP secret and disables 2FA for the current user.
func (a *Auth) TwoFADisable(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := a.q.SetUserTOTP(r.Context(), generated.SetUserTOTPParams{TotpSecret: nil, TotpEnabled: 0, ID: u.ID}); err != nil {
		http.Error(w, "could not disable", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.SessionCookieName); err == nil {
		_ = a.q.DeleteSession(r.Context(), c.Value)
	}
	auth.ClearSessionCookie(w)
	w.WriteHeader(http.StatusOK)
}

// startSession creates a session row and sets the session cookie. It returns
// false (after writing an error response) when the session could not be
// persisted, so callers must not write a success status in that case.
func (a *Auth) startSession(w http.ResponseWriter, r *http.Request, userID int64) bool {
	token, err := auth.NewSessionToken()
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return false
	}
	if err := a.q.CreateSession(r.Context(), generated.CreateSessionParams{
		Token:     token,
		UserID:    userID,
		ExpiresAt: time.Now().Add(auth.SessionDuration),
	}); err != nil {
		http.Error(w, "could not create session", http.StatusInternalServerError)
		return false
	}
	auth.SetSessionCookie(w, token, a.secure)
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
