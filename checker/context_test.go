package checker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// These requests deliberately have long timeouts. Shutdown must interrupt both
// a pending response and a blocked response-body read through the lifecycle
// context instead of waiting for the configured probe timeout.
func TestLifecycleCancellationInterruptsHTTPChecks(t *testing.T) {
	for _, tc := range []struct {
		name         string
		flushHeaders bool
		run          func(*ProxyChecker, *http.Client) error
	}{
		{
			name: "current IP response body", flushHeaders: true,
			run: func(pc *ProxyChecker, client *http.Client) error {
				pc.httpClient = client
				_, err := pc.GetCurrentIP()
				return err
			},
		},
		{
			name: "proxy IP response body", flushHeaders: true,
			run: func(pc *ProxyChecker, client *http.Client) error {
				_, _, _, err := pc.checkByIP(client)
				return err
			},
		},
		{
			name: "status response headers",
			run: func(pc *ProxyChecker, client *http.Client) error {
				_, _, _, err := pc.checkByGen(client)
				return err
			},
		},
		{
			name: "download response body", flushHeaders: true,
			run: func(pc *ProxyChecker, client *http.Client) error {
				_, _, _, err := pc.checkByDownload(client)
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entered := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.flushHeaders {
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
				}
				close(entered)
				<-r.Context().Done()
			}))
			t.Cleanup(func() {
				server.CloseClientConnections()
				server.Close()
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			pc := NewProxyChecker(nil, 10000, server.URL, 60, server.URL, server.URL, 60, 1, "status", 0)
			pc.SetContext(ctx)
			finished := make(chan error, 1)
			go func() { finished <- tc.run(pc, server.Client()) }()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("HTTP check did not reach the local server")
			}
			cancel()
			select {
			case err := <-finished:
				if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
					t.Fatalf("error = %v, want lifecycle cancellation", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("HTTP check ignored lifecycle cancellation")
			}
		})
	}
}
