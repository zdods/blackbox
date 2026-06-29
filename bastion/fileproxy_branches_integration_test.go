//go:build integration

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// loginAndDaemon registers + logs in a user, creates a daemon, and connects a
// fake daemon backing the given files. It returns the server, the authenticated
// client, the base URL, and the daemon id.
func loginAndDaemon(t *testing.T, label string, files map[string][]byte) (*Server, *http.Client, string, string) {
	t.Helper()
	srv, ts := newTestServer(t)
	c := client(t)
	secret := registerUser(t, c, ts.URL, "zach", "pw12345678")
	login(t, c, ts.URL, "zach", "pw12345678", secret)
	id, token := createDaemon(t, c, ts.URL, label)
	startFakeDaemon(t, ts.URL, token, files)
	return srv, c, ts.URL, id
}

func TestIntegrationUploadTooLargeSingleShot(t *testing.T) {
	defer swap(&maxSingleShotUpload, int64(16))()
	_, c, base, id := loginAndDaemon(t, "big-upload", nil)

	req, _ := http.NewRequest("PUT", base+"/api/daemons/"+id+"/files?path=big.bin", bytes.NewReader(bytes.Repeat([]byte("x"), 17)))
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusRequestEntityTooLarge)
	resp.Body.Close()
}

func TestIntegrationUploadChunkTooLarge(t *testing.T) {
	defer swap(&maxChunkUpload, int64(16))()
	_, c, base, id := loginAndDaemon(t, "big-chunk", nil)

	url := base + "/api/daemons/" + id + "/files?path=big.bin&upload_id=u1&total_chunks=1&chunk_index=0"
	req, _ := http.NewRequest("PUT", url, bytes.NewReader(bytes.Repeat([]byte("x"), 17)))
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusRequestEntityTooLarge)
	resp.Body.Close()
}

func TestIntegrationInvalidChunkParams(t *testing.T) {
	_, c, base, id := loginAndDaemon(t, "bad-chunk", nil)

	cases := []string{
		"path=f&upload_id=u1&total_chunks=1&chunk_index=5",   // index >= total
		"path=f&upload_id=u1&total_chunks=1&chunk_index=-1",  // negative index
		"path=f&upload_id=u1&total_chunks=abc&chunk_index=0", // non-numeric total
		"path=f&upload_id=u1&total_chunks=0&chunk_index=0",   // zero total
	}
	for _, q := range cases {
		req, _ := http.NewRequest("PUT", base+"/api/daemons/"+id+"/files?"+q, bytes.NewReader([]byte("x")))
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("params %q → status %d, want 400", q, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestIntegrationDownloadDirectoryRejected(t *testing.T) {
	_, c, base, id := loginAndDaemon(t, "dir-dl", nil)

	// The fake daemon reports path "." as a directory; downloading it must 400.
	resp, err := c.Get(base + "/api/daemons/" + id + "/files?path=.&download=1")
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestIntegrationTransferSemaphoreBusy(t *testing.T) {
	defer swap(&proxyTimeout, 50*time.Millisecond)()
	srv, c, base, id := loginAndDaemon(t, "busy", map[string][]byte{"f.txt": []byte("hi")})

	// Saturate every transfer slot so the next download can't acquire one.
	for i := 0; i < cap(srv.transferSem); i++ {
		srv.transferSem <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(srv.transferSem); i++ {
			<-srv.transferSem
		}
	}()

	resp, err := c.Get(base + "/api/daemons/" + id + "/files?path=f.txt&download=1")
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusServiceUnavailable)
	resp.Body.Close()
}

func TestIntegrationUpdateAndDeleteDaemon(t *testing.T) {
	_, ts := newTestServer(t)
	c := client(t)
	secret := registerUser(t, c, ts.URL, "zach", "pw12345678")
	login(t, c, ts.URL, "zach", "pw12345678", secret)
	id, _ := createDaemon(t, c, ts.URL, "before")

	// Update with empty label → 400.
	resp := patchJSON(t, c, ts.URL+"/api/daemons/"+id, map[string]any{"label": ""})
	wantStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()

	// Valid update → 204, and the new label shows up in the list.
	resp = patchJSON(t, c, ts.URL+"/api/daemons/"+id, map[string]any{"label": "after"})
	wantStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
	resp, err := c.Get(ts.URL + "/api/daemons")
	if err != nil {
		t.Fatal(err)
	}
	list := decodeJSON[[]map[string]any](t, resp)
	found := false
	for _, d := range list {
		if d["id"] == id && d["label"] == "after" {
			found = true
		}
	}
	if !found {
		t.Fatalf("updated label not reflected in listing: %v", list)
	}

	// Update a non-existent daemon → 404.
	resp = patchJSON(t, c, ts.URL+"/api/daemons/00000000-0000-0000-0000-000000000000", map[string]any{"label": "x"})
	wantStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	// Delete it → 204, then a second delete → 404.
	for i, want := range []int{http.StatusNoContent, http.StatusNotFound} {
		req, _ := http.NewRequest("DELETE", ts.URL+"/api/daemons/"+id, nil)
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != want {
			t.Errorf("delete #%d → status %d, want %d", i+1, resp.StatusCode, want)
		}
		resp.Body.Close()
	}
}

func patchJSON(t *testing.T, c *http.Client, url string, body map[string]any) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, _ := http.NewRequest("PATCH", url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("PATCH %s: %v", url, err)
	}
	return resp
}
