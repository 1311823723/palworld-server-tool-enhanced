package tool

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zaigie/palworld-server-tool/internal/config"
)

func withTestConfig(t *testing.T) {
	t.Helper()
	store, err := config.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("open config store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	config.SetCurrent(store)
}

func setRestAddress(t *testing.T, address string, timeout int) {
	t.Helper()
	settings := config.Current()
	settings.Rest.Address = address
	settings.Rest.Timeout = timeout
	if err := config.CurrentStore().Update(settings, ""); err != nil {
		t.Fatalf("update rest settings: %v", err)
	}
}

func TestCallApiConcurrentCallsAreIndependent(t *testing.T) {
	withTestConfig(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	setRestAddress(t, server.URL, 5)

	const workers = 16
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body, err := callApi(http.MethodGet, "/v1/api/info", nil)
			if err != nil {
				errs <- err
				return
			}
			if string(body) != `{"ok":true}` {
				errs <- fmt.Errorf("unexpected body: %s", body)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestCallApiHonorsConfiguredTimeout(t *testing.T) {
	withTestConfig(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
	}))
	defer server.Close()
	setRestAddress(t, server.URL, 1)

	started := time.Now()
	_, err := callApi(http.MethodGet, "/v1/api/info", nil)
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	elapsed := time.Since(started)
	if elapsed > 2500*time.Millisecond {
		t.Fatalf("timeout took %s, want less than 2.5s", elapsed)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "deadline") && !strings.Contains(strings.ToLower(err.Error()), "timeout") {
		t.Fatalf("error %q does not describe a timeout", err)
	}
}
