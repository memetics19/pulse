package push

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken()
	require.NoError(t, err)
	assert.Len(t, token, 32)
	assert.Regexp(t, regexp.MustCompile(`^[A-Za-z0-9_-]{32}$`), token)
	assert.True(t, ValidToken(token))
}

func TestValidToken(t *testing.T) {
	assert.True(t, ValidToken("abcdefghij"))
	assert.True(t, ValidToken("abcdefghij-_0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"))
	assert.False(t, ValidToken("abcdefghi"))
	assert.False(t, ValidToken("abcdefghij!"))
	assert.False(t, ValidToken(strings.Repeat("a", 129)))
}

func TestHashToken(t *testing.T) {
	hash := HashToken("abcdefghij")
	assert.Len(t, hash, 64)
	assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{64}$`), hash)
}

func TestPrefix(t *testing.T) {
	assert.Equal(t, "abcdefgh", Prefix("abcdefghij"))
	assert.Equal(t, "short", Prefix("short"))
	assert.Empty(t, Prefix(""))
}
