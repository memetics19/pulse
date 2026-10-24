package handlers

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/generated"
)

type Auth struct {
	q              *generated.Queries
	secure         bool
	limiter        *loginLimiter
	codeLimiter    *loginLimiter
	trustedProxies []*net.IPNet
}

func NewAuth(q *generated.Queries, secure bool, trustedProxies ...*net.IPNet) *Auth {
	return &Auth{q: q, secure: secure, limiter: newLoginLimiter(), codeLimiter: newLoginLimiter(), trustedProxies: trustedProxies}
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
	u, _, ok := a.currentSession(r)
	return u, ok
}

// currentSession is currentUser plus the session token, for handlers that
// must keep this session while signing out the user's others.
func (a *Auth) currentSession(r *http.Request) (generated.User, string, bool) {
	c, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		return generated.User{}, "", false
	}
	sess, err := a.q.GetSession(r.Context(), c.Value)
	if err != nil || !sess.ExpiresAt.After(time.Now()) {
		return generated.User{}, "", false
	}
	u, err := a.q.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		return generated.User{}, "", false
	}
	return u, c.Value, true
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
	if !a.limiter.allow(clientIP(r, a.trustedProxies)) {
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
		if u.TotpSecret == nil {
			http.Error(w, msg2FABadCode, http.StatusUnauthorized)
			return
		}
		if !a.useCode(w, r, u.ID, *u.TotpSecret, strings.TrimSpace(c.Code), http.StatusUnauthorized) {
			return
		}
	}
	if !a.startSession(w, r, u.ID) {
		return
	}
	w.WriteHeader(http.StatusOK)
}

// 2FA error messages. They are part of the API contract: the admin UI shows
// them to the user as they are.
const (
	msg2FAAlreadyOn  = "2FA is already on, disable it first with a code"
	msg2FANotOn      = "2FA is not on"
	msg2FACodeNeeded = "a current 2FA code is required"
	msg2FABadCode    = "invalid 2FA code"
	msg2FATooMany    = "too many attempts"
	msg2FAUsed       = "this 2FA code was already used, wait for the next one"
)

// readCode decodes a {"code": "..."} body. A malformed body yields "", which
// every caller treats as a missing code.
func readCode(r *http.Request) string {
	var body struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	return strings.TrimSpace(body.Code)
}

// allowCode spends one 2FA code attempt for the user. Code checks on a live
// session are limited per account rather than per IP, so a stolen session
// cannot brute force the six digit code from many addresses.
func (a *Auth) allowCode(userID int64) bool {
	return a.codeLimiter.allow("user:" + strconv.FormatInt(userID, 10))
}

// useCode checks code against secret and claims its time step for the user,
// so each code is accepted at most once. The claim is a single conditional
// UPDATE, so two requests racing with the same code cannot both win. On
// failure it writes the response (badStatus for a wrong or reused code) and
// returns false.
func (a *Auth) useCode(w http.ResponseWriter, r *http.Request, userID int64, secret, code string, badStatus int) bool {
	step, ok := auth.MatchTOTP(secret, code, time.Now())
	if !ok {
		http.Error(w, msg2FABadCode, badStatus)
		return false
	}
	n, err := a.q.ClaimTOTPStep(r.Context(), generated.ClaimTOTPStepParams{Step: step, ID: userID})
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return false
	}
	if n == 0 {
		http.Error(w, msg2FAUsed, badStatus)
		return false
	}
	return true
}

// signOutOthers deletes every session of the user except the current one.
// It runs after a 2FA change, so a session opened before the change (for
// example a stolen one) does not outlive it.
func (a *Auth) signOutOthers(w http.ResponseWriter, r *http.Request, userID int64, token string) bool {
	if _, err := a.q.DeleteOtherUserSessions(r.Context(), generated.DeleteOtherUserSessionsParams{UserID: userID, Token: token}); err != nil {
		http.Error(w, "2FA changed, but other sessions could not be signed out", http.StatusInternalServerError)
		return false
	}
	return true
}

// TwoFASetup generates a fresh TOTP secret for the current user and returns
// the provisioning URI and a scannable QR data URL. The secret is stored as
// pending: 2FA stays off, and login ignores the secret, until the user
// confirms a code via TwoFAEnable.
//
// Setup is refused with 409 while 2FA is on. Running it again must never
// replace or drop the active secret, so changing authenticators means turning
// 2FA off with a valid code first and then enrolling again.
func (a *Auth) TwoFASetup(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if u.TotpEnabled == 1 {
		http.Error(w, msg2FAAlreadyOn, http.StatusConflict)
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

// TwoFAEnable confirms the pending TOTP secret with a valid code and enables
// 2FA. Every other session of the user is then signed out.
func (a *Auth) TwoFAEnable(w http.ResponseWriter, r *http.Request) {
	u, token, ok := a.currentSession(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if u.TotpEnabled == 1 {
		http.Error(w, msg2FAAlreadyOn, http.StatusConflict)
		return
	}
	code := readCode(r)
	if u.TotpSecret == nil {
		http.Error(w, "run setup first", http.StatusBadRequest)
		return
	}
	if code == "" {
		http.Error(w, msg2FACodeNeeded, http.StatusBadRequest)
		return
	}
	if !a.allowCode(u.ID) {
		http.Error(w, msg2FATooMany, http.StatusTooManyRequests)
		return
	}
	if !a.useCode(w, r, u.ID, *u.TotpSecret, code, http.StatusBadRequest) {
		return
	}
	if err := a.q.SetUserTOTP(r.Context(), generated.SetUserTOTPParams{TotpSecret: u.TotpSecret, TotpEnabled: 1, ID: u.ID}); err != nil {
		http.Error(w, "could not enable", http.StatusInternalServerError)
		return
	}
	if !a.signOutOthers(w, r, u.ID, token) {
		return
	}
	w.WriteHeader(http.StatusOK)
}

// TwoFADisable turns 2FA off for the current user. It needs a valid code from
// the active secret, so a session cookie alone (for example one taken from an
// unlocked browser) cannot strip the second factor. A missing code is a 400,
// a wrong or reused code is a 403, and in all those cases 2FA stays on. On
// success every other session of the user is signed out.
func (a *Auth) TwoFADisable(w http.ResponseWriter, r *http.Request) {
	u, token, ok := a.currentSession(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if u.TotpEnabled != 1 || u.TotpSecret == nil {
		http.Error(w, msg2FANotOn, http.StatusConflict)
		return
	}
	code := readCode(r)
	if code == "" {
		http.Error(w, msg2FACodeNeeded, http.StatusBadRequest)
		return
	}
	if !a.allowCode(u.ID) {
		http.Error(w, msg2FATooMany, http.StatusTooManyRequests)
		return
	}
	if !a.useCode(w, r, u.ID, *u.TotpSecret, code, http.StatusForbidden) {
		return
	}
	if err := a.q.SetUserTOTP(r.Context(), generated.SetUserTOTPParams{TotpSecret: nil, TotpEnabled: 0, ID: u.ID}); err != nil {
		http.Error(w, "could not disable", http.StatusInternalServerError)
		return
	}
	if !a.signOutOthers(w, r, u.ID, token) {
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
		ExpiresAt: time.Now().UTC().Add(auth.SessionDuration),
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
	encodeJSON(w, v)
}

// encodeJSON writes v as the JSON response body. The status line is already
// sent by then, so a failure (usually the client hanging up) can only be logged.
func encodeJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("handlers: write JSON response: %v", err)
	}
}
