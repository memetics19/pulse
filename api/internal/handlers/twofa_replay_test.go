package handlers

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A login code works once. An older step is refused too, and the next step
// still works.
func TestTwoFALoginCodeWorksOnce(t *testing.T) {
	f := newTwoFAFixture(t)
	secret := f.enable(t) // spends the current step

	next := codeAt(t, secret, 1)
	require.Equal(t, http.StatusOK, f.login(t, next).Code)

	rec := f.login(t, next)
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "the same code must not log in twice")
	assert.Contains(t, rec.Body.String(), msg2FAUsed)

	rec = f.login(t, codeAt(t, secret, 0))
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "a code older than the last accepted one is refused")
	assert.Contains(t, rec.Body.String(), msg2FAUsed)

	rec = f.login(t, "000000")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), msg2FABadCode)
}

// The code that turned 2FA on cannot be replayed to turn it off. The next
// code can.
func TestTwoFAEnableCodeCannotDisable(t *testing.T) {
	f := newTwoFAFixture(t)
	secret := f.startSetup(t)
	enableCode := codeBody(t, secret)
	require.Equal(t, http.StatusOK, f.call(f.h.TwoFAEnable, enableCode).Code)

	rec := f.call(f.h.TwoFADisable, enableCode)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), msg2FAUsed)
	f.assertOn(t, secret)

	assert.Equal(t, http.StatusOK, f.call(f.h.TwoFADisable, nextCodeBody(t, secret)).Code)
}

// Enable claims the step as well: after a confirmed setup, a fresh setup and
// enable cannot reuse a step that was already accepted.
func TestTwoFAEnableCodeWorksOnce(t *testing.T) {
	f := newTwoFAFixture(t)
	secret := f.enable(t)
	require.Equal(t, http.StatusOK, f.call(f.h.TwoFADisable, nextCodeBody(t, secret)).Code)

	// Steps up to now+1 are spent, so even a new secret's code for those
	// steps is refused until a newer step comes round.
	fresh := f.startSetup(t)
	rec := f.call(f.h.TwoFAEnable, nextCodeBody(t, fresh))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), msg2FAUsed)
	assert.Equal(t, int64(0), f.user(t).TotpEnabled)
}

// Requests racing with one code: exactly one wins, because the step is
// claimed with a single conditional UPDATE.
func TestTwoFAConcurrentUseOfOneCode(t *testing.T) {
	const n = 8 // under both the per-IP and per-account burst of 10

	t.Run("login", func(t *testing.T) {
		f := newTwoFAFixture(t)
		secret := f.enable(t)
		code := codeAt(t, secret, 1)
		assert.Equal(t, 1, countOK(n, func() int { return f.login(t, code).Code }))
	})

	t.Run("disable", func(t *testing.T) {
		f := newTwoFAFixture(t)
		secret := f.enable(t)
		body := nextCodeBody(t, secret)
		assert.Equal(t, 1, countOK(n, func() int { return f.call(f.h.TwoFADisable, body).Code }))
	})

	t.Run("enable", func(t *testing.T) {
		f := newTwoFAFixture(t)
		secret := f.startSetup(t)
		body := codeBody(t, secret)
		assert.Equal(t, 1, countOK(n, func() int { return f.call(f.h.TwoFAEnable, body).Code }))
		f.assertOn(t, secret)
	})
}

// countOK runs do n times at once and counts the 200 responses.
func countOK(n int, do func() int) int {
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		ok    int
		start = make(chan struct{})
	)
	for range n {
		wg.Go(func() {
			<-start
			if do() == http.StatusOK {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		})
	}
	close(start)
	wg.Wait()
	return ok
}

// sessionAlive reports whether token is still a valid session.
func (f *twoFAFixture) sessionAlive(t *testing.T, token string) bool {
	t.Helper()
	_, err := f.q.GetSession(t.Context(), token)
	return err == nil
}

// addSession opens another session for userID, as a second browser would.
func (f *twoFAFixture) addSession(t *testing.T, userID int64, token string) {
	t.Helper()
	require.NoError(t, f.q.CreateSession(t.Context(), generated.CreateSessionParams{
		Token: token, UserID: userID, ExpiresAt: time.Now().Add(time.Hour),
	}))
}

// A 2FA change signs out every other session of the user and keeps the one
// that made the change. Other users' sessions are untouched.
func TestTwoFAChangeSignsOutOtherSessions(t *testing.T) {
	setupOthers := func(t *testing.T, f *twoFAFixture) generated.User {
		t.Helper()
		admin := f.user(t)
		f.addSession(t, admin.ID, "other-browser")
		f.addSession(t, admin.ID, "stolen-cookie")
		other, err := f.q.CreateUser(t.Context(), generated.CreateUserParams{Username: "someone", PasswordHash: "x"})
		require.NoError(t, err)
		f.addSession(t, other.ID, "someone-else")
		return admin
	}
	check := func(t *testing.T, f *twoFAFixture) {
		t.Helper()
		assert.True(t, f.sessionAlive(t, f.cookie.Value), "the session that made the change stays")
		assert.False(t, f.sessionAlive(t, "other-browser"))
		assert.False(t, f.sessionAlive(t, "stolen-cookie"))
		assert.True(t, f.sessionAlive(t, "someone-else"), "other users keep their sessions")
	}

	t.Run("enable", func(t *testing.T) {
		f := newTwoFAFixture(t)
		secret := f.startSetup(t)
		setupOthers(t, f)
		require.Equal(t, http.StatusOK, f.call(f.h.TwoFAEnable, codeBody(t, secret)).Code)
		check(t, f)
	})

	t.Run("disable", func(t *testing.T) {
		f := newTwoFAFixture(t)
		secret := f.enable(t)
		setupOthers(t, f)
		require.Equal(t, http.StatusOK, f.call(f.h.TwoFADisable, nextCodeBody(t, secret)).Code)
		check(t, f)
	})

	t.Run("refused change keeps sessions", func(t *testing.T) {
		f := newTwoFAFixture(t)
		f.enable(t)
		setupOthers(t, f)
		require.Equal(t, http.StatusForbidden, f.call(f.h.TwoFADisable, `{"code":"000000"}`).Code)
		assert.True(t, f.sessionAlive(t, "other-browser"))
	})
}
