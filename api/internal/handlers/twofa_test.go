package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// twoFAFixture is an admin account with a live session, backed by a real DB.
type twoFAFixture struct {
	db     *sql.DB
	q      *generated.Queries
	h      *Auth
	cookie *http.Cookie
}

func newTwoFAFixture(t *testing.T) *twoFAFixture {
	t.Helper()
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := NewAuth(q, false)
	return &twoFAFixture{db: db, q: q, h: h, cookie: setupAdmin(t, h)}
}

// call runs hf with the session cookie and a raw JSON body ("" sends none).
func (f *twoFAFixture) call(hf http.HandlerFunc, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/x", bytes.NewReader([]byte(body)))
	r.AddCookie(f.cookie)
	rec := httptest.NewRecorder()
	hf(rec, r)
	return rec
}

func codeBody(t *testing.T, secret string) string {
	t.Helper()
	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)
	return `{"code":"` + code + `"}`
}

// startSetup runs setup and returns the pending secret.
func (f *twoFAFixture) startSetup(t *testing.T) string {
	t.Helper()
	rec := f.call(f.h.TwoFASetup, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp struct {
		Secret string `json:"secret"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.NotEmpty(t, resp.Secret)
	return resp.Secret
}

// enable runs setup and confirms it, returning the active secret.
func (f *twoFAFixture) enable(t *testing.T) string {
	t.Helper()
	secret := f.startSetup(t)
	rec := f.call(f.h.TwoFAEnable, codeBody(t, secret))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return secret
}

func (f *twoFAFixture) user(t *testing.T) generated.User {
	t.Helper()
	u, err := f.q.GetUserByUsername(t.Context(), "admin")
	require.NoError(t, err)
	return u
}

// assertOn checks that 2FA is still on with exactly the given secret.
func (f *twoFAFixture) assertOn(t *testing.T, secret string) {
	t.Helper()
	u := f.user(t)
	assert.Equal(t, int64(1), u.TotpEnabled, "2FA must stay on")
	require.NotNil(t, u.TotpSecret)
	assert.Equal(t, secret, *u.TotpSecret, "the active secret must not change")
}

func (f *twoFAFixture) login(t *testing.T, code string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"username": "admin", "password": "s3cret-pass", "code": code})
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	f.h.Login(rec, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body)))
	return rec
}

func TestTwoFADisableWithValidCode(t *testing.T) {
	f := newTwoFAFixture(t)
	secret := f.enable(t)

	rec := f.call(f.h.TwoFADisable, codeBody(t, secret))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	u := f.user(t)
	assert.Equal(t, int64(0), u.TotpEnabled)
	assert.Nil(t, u.TotpSecret, "disabling must clear the secret")
	assert.Equal(t, http.StatusOK, f.login(t, "").Code, "login needs only the password once 2FA is off")
}

func TestTwoFADisableRejectsWrongCode(t *testing.T) {
	f := newTwoFAFixture(t)
	secret := f.enable(t)

	// A code from some other authenticator is a wrong code, not a missing one.
	other, _, err := auth.GenerateTOTP("someone-else")
	require.NoError(t, err)

	for _, body := range []string{`{"code":"000000"}`, `{"code":"12345"}`, `{"code":"abcdef"}`, codeBody(t, other)} {
		rec := f.call(f.h.TwoFADisable, body)
		assert.Equal(t, http.StatusForbidden, rec.Code, body)
		assert.Contains(t, rec.Body.String(), msg2FABadCode)
	}
	f.assertOn(t, secret)
}

func TestTwoFADisableRejectsMissingCode(t *testing.T) {
	f := newTwoFAFixture(t)
	secret := f.enable(t)

	for _, body := range []string{"", `{}`, `{"code":""}`, `{"code":"   "}`, `not json`} {
		rec := f.call(f.h.TwoFADisable, body)
		assert.Equal(t, http.StatusBadRequest, rec.Code, "body %q", body)
		assert.Contains(t, rec.Body.String(), msg2FACodeNeeded)
	}
	f.assertOn(t, secret)
}

func TestTwoFADisableNeedsSession(t *testing.T) {
	f := newTwoFAFixture(t)
	secret := f.enable(t)

	rec := httptest.NewRecorder()
	f.h.TwoFADisable(rec, httptest.NewRequest(http.MethodPost, "/x", bytes.NewReader([]byte(codeBody(t, secret)))))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	f.assertOn(t, secret)
}

func TestTwoFADisableWhenOffIsConflict(t *testing.T) {
	f := newTwoFAFixture(t)
	rec := f.call(f.h.TwoFADisable, `{"code":"000000"}`)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), msg2FANotOn)

	// A pending (unconfirmed) setup is not "on" either.
	f.startSetup(t)
	rec = f.call(f.h.TwoFADisable, `{"code":"000000"}`)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

// Guessing codes on a live session is limited per account, so a stolen cookie
// cannot brute force its way to turning 2FA off.
func TestTwoFADisableIsRateLimited(t *testing.T) {
	f := newTwoFAFixture(t)
	secret := f.enable(t) // the confirming enable spent one attempt

	for i := 1; i < loginBurst; i++ {
		require.Equal(t, http.StatusForbidden, f.call(f.h.TwoFADisable, `{"code":"000000"}`).Code, "attempt %d", i)
	}
	// Out of attempts: even the right code is refused until the bucket refills.
	rec := f.call(f.h.TwoFADisable, codeBody(t, secret))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	f.assertOn(t, secret)
}

func TestTwoFAEnableIsRateLimited(t *testing.T) {
	f := newTwoFAFixture(t)
	f.startSetup(t)
	for i := 0; i < loginBurst; i++ {
		require.Equal(t, http.StatusBadRequest, f.call(f.h.TwoFAEnable, `{"code":"000000"}`).Code, "attempt %d", i)
	}
	assert.Equal(t, http.StatusTooManyRequests, f.call(f.h.TwoFAEnable, `{"code":"000000"}`).Code)
}

// Re-running setup while 2FA is on must neither turn it off nor swap the secret.
func TestTwoFASetupWhileOnIsRefused(t *testing.T) {
	f := newTwoFAFixture(t)
	secret := f.enable(t)

	rec := f.call(f.h.TwoFASetup, "")
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), msg2FAAlreadyOn)
	assert.NotContains(t, rec.Body.String(), "otpauth", "no new secret may be issued")
	f.assertOn(t, secret)

	// Login still demands a code from the original secret.
	assert.Equal(t, http.StatusUnauthorized, f.login(t, "").Code)
	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, f.login(t, code).Code)
}

func TestTwoFAEnableWhileOnIsRefused(t *testing.T) {
	f := newTwoFAFixture(t)
	secret := f.enable(t)
	rec := f.call(f.h.TwoFAEnable, codeBody(t, secret))
	assert.Equal(t, http.StatusConflict, rec.Code)
	f.assertOn(t, secret)
}

func TestTwoFAEnableMissingCode(t *testing.T) {
	f := newTwoFAFixture(t)
	f.startSetup(t)
	rec := f.call(f.h.TwoFAEnable, `{}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), msg2FACodeNeeded)
	assert.Equal(t, int64(0), f.user(t).TotpEnabled)
}

// A pending secret is never a login factor: it is neither demanded nor accepted
// until it has been confirmed.
func TestTwoFAPendingSecretNotUsedForLogin(t *testing.T) {
	f := newTwoFAFixture(t)
	f.startSetup(t)

	rec := f.login(t, "")
	assert.Equal(t, http.StatusOK, rec.Code, "a pending secret must not make login ask for a code")
	assert.NotContains(t, rec.Body.String(), "needs_2fa")

	// Once 2FA is on with secret A, a code from a second, never-confirmed
	// secret does not log in.
	f2 := newTwoFAFixture(t)
	active := f2.enable(t)
	stray, _, err := auth.GenerateTOTP("admin")
	require.NoError(t, err)
	strayCode, err := totp.GenerateCode(stray, time.Now())
	require.NoError(t, err)
	activeCode, err := totp.GenerateCode(active, time.Now())
	require.NoError(t, err)
	if strayCode != activeCode {
		assert.Equal(t, http.StatusUnauthorized, f2.login(t, strayCode).Code)
	}
	assert.Equal(t, http.StatusOK, f2.login(t, activeCode).Code)
}

func TestTwoFADisableSaveError(t *testing.T) {
	f := newTwoFAFixture(t)
	secret := f.enable(t)

	// currentUser makes two queries. The third, the write, fails.
	broken := NewAuth(generated.New(testutil.FailAfter(f.db, 2)), false)
	rec := f.call(broken.TwoFADisable, codeBody(t, secret))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	f.assertOn(t, secret)
}
