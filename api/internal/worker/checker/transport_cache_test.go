package checker

import (
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTransportForCachesAndResets(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	ca := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))

	def, err := transportFor(true, false, "")
	require.NoError(t, err)
	require.Same(t, sharedTransportPrivate, def, "the default setup uses the shared transport")

	first, err := transportFor(true, false, ca)
	require.NoError(t, err)
	again, err := transportFor(true, false, ca)
	require.NoError(t, err)
	require.Same(t, first, again, "the same CA reuses its transport")
	require.NotNil(t, first.TLSClientConfig.RootCAs)

	skip, err := transportFor(false, true, "")
	require.NoError(t, err)
	require.True(t, skip.TLSClientConfig.InsecureSkipVerify)
	require.NotSame(t, first, skip)

	// Distinct CA texts fill the cache until it resets.
	for i := 0; i <= maxTLSTransports; i++ {
		_, err := transportFor(true, false, fmt.Sprintf("%s\n# %d\n", ca, i))
		require.NoError(t, err)
	}
	tlsTransports.Lock()
	n := len(tlsTransports.m)
	tlsTransports.Unlock()
	require.Less(t, n, maxTLSTransports)
}
