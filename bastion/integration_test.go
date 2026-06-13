//go:build integration

// Integration tests drive the real handler stack (routes + middleware +
// Postgres + daemon WebSocket protocol) end to end. They need a Postgres
// reachable via TEST_DATABASE_URL (default: local blackhaul_test database,
// created automatically if the server is up).
//
// Run: go test -tags=integration ./bastion/
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"blackhaul/pkg"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp/totp"
)

const testJWTSecret = "integration-test-secret-not-for-production"

// --- harness ---------------------------------------------------------------

func testDBURL() string {
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u
	}
	return "postgres://postgres:postgres@localhost:5432/blackhaul_test?sslmode=disable"
}

// openTestDB connects to the test database, creating it first when it does
// not exist yet (so a plain local Postgres works out of the box).
func openTestDB(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dbURL := testDBURL()
	pool, err := OpenDB(ctx, dbURL)
	if err == nil {
		return pool
	}
	u, perr := url.Parse(dbURL)
	if perr != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", perr)
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	u.Path = "/postgres"
	admin, aerr := OpenDB(ctx, u.String())
	if aerr != nil {
		t.Fatalf("postgres unreachable (start one, e.g. `docker compose up -d postgres`): %v", err)
	}
	defer admin.Close()
	if _, cerr := admin.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %q`, dbName)); cerr != nil {
		t.Fatalf("create database %s: %v", dbName, cerr)
	}
	pool, err = OpenDB(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to %s after create: %v", dbName, err)
	}
	return pool
}

// newTestServer builds a Server exactly like main() does (fresh rate
// limiters, migrated + truncated database) and exposes the real routes()
// handler via httptest.
func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	ctx := context.Background()
	pool := openTestDB(t, ctx)
	if err := RunMigrations(ctx, pool); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE TABLE daemons, users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	srv := &Server{
		pool:             pool,
		cfg:              Config{JWTSecret: testJWTSecret},
		hub:              NewHub(),
		totpCache:        NewTotpSetupCache(),
		authLimiter:      NewRateLimiter(10, time.Minute),
		loginFailLimiter: NewRateLimiter(5, 15*time.Minute),
		totpFailLimiter:  NewRateLimiter(5, 15*time.Minute),
		transferSem:      make(chan struct{}, maxConcurrentTransfers),
	}
	ts := httptest.NewServer(srv.routes())
	t.Cleanup(func() {
		ts.Close()
		pool.Close()
	})
	return srv, ts
}

// client returns an http.Client with a cookie jar (the console's session
// cookie flow).
func client(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	return &http.Client{Jar: jar}
}

func postJSON(t *testing.T, c *http.Client, url string, body any) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := c.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func decodeJSON[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return v
}

func wantStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("status = %d, want %d", resp.StatusCode, want)
	}
}

// registerUser runs the full TOTP-mandatory registration flow and returns
// the TOTP secret for later logins.
func registerUser(t *testing.T, c *http.Client, baseURL, username, password string) (totpSecret string) {
	t.Helper()
	resp := postJSON(t, c, baseURL+"/api/register/totp-setup", map[string]string{})
	wantStatus(t, resp, http.StatusOK)
	setup := decodeJSON[map[string]string](t, resp)
	code, err := totp.GenerateCode(setup["secret"], time.Now())
	if err != nil {
		t.Fatalf("generate totp: %v", err)
	}
	resp = postJSON(t, c, baseURL+"/api/register", map[string]string{
		"username": username, "password": password,
		"totp_code": code, "setup_id": setup["setup_id"],
	})
	wantStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	return setup["secret"]
}

// login performs password + TOTP login; the session cookie lands in the
// client's jar. Returns the raw session cookie for tests that need to replay
// it after logout.
func login(t *testing.T, c *http.Client, baseURL, username, password, totpSecret string) *http.Cookie {
	t.Helper()
	resp := postJSON(t, c, baseURL+"/api/login", map[string]string{
		"username": username, "password": password,
	})
	wantStatus(t, resp, http.StatusOK)
	challenge := decodeJSON[map[string]any](t, resp)
	if challenge["requires_totp"] != true {
		t.Fatalf("expected requires_totp challenge, got %v", challenge)
	}
	code, err := totp.GenerateCode(totpSecret, time.Now())
	if err != nil {
		t.Fatalf("generate totp: %v", err)
	}
	resp = postJSON(t, c, baseURL+"/api/login/totp", map[string]string{
		"login_token": challenge["login_token"].(string), "code": code,
	})
	wantStatus(t, resp, http.StatusOK)
	defer resp.Body.Close()
	for _, ck := range resp.Cookies() {
		if ck.Name == "session" && ck.Value != "" {
			return ck
		}
	}
	t.Fatal("no session cookie in TOTP login response")
	return nil
}

// wrongTOTPCode returns a code guaranteed to differ from the current one.
func wrongTOTPCode(t *testing.T, secret string) string {
	t.Helper()
	valid, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate totp: %v", err)
	}
	if valid == "000000" {
		return "111111"
	}
	return "000000"
}

// --- fake daemon -----------------------------------------------------------

// fakeDaemon speaks the daemon side of the pkg protocol over a real
// WebSocket, backed by an in-memory file map.
type fakeDaemon struct {
	t    *testing.T
	conn *websocket.Conn

	mu     sync.Mutex
	files  map[string][]byte
	chunks map[string][][]byte // upload_id → chunk slices
}

// dialDaemonWS connects and authenticates; returns the first auth reply.
func dialDaemonWS(t *testing.T, baseURL, token string) (*websocket.Conn, map[string]string) {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/ws/daemon"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	if err := conn.WriteJSON(pkg.Auth{Type: pkg.TypeAuth, Token: token}); err != nil {
		t.Fatalf("ws auth write: %v", err)
	}
	var reply map[string]string
	if err := conn.ReadJSON(&reply); err != nil {
		t.Fatalf("ws auth read: %v", err)
	}
	return conn, reply
}

func startFakeDaemon(t *testing.T, baseURL, token string, files map[string][]byte) *fakeDaemon {
	t.Helper()
	conn, reply := dialDaemonWS(t, baseURL, token)
	if reply["type"] != pkg.TypeAuthOK {
		t.Fatalf("daemon auth failed: %v", reply)
	}
	if files == nil {
		files = map[string][]byte{}
	}
	fd := &fakeDaemon{t: t, conn: conn, files: files, chunks: map[string][][]byte{}}
	go fd.serve()
	t.Cleanup(func() { conn.Close() })
	return fd
}

func (fd *fakeDaemon) get(path string) ([]byte, bool) {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	data, ok := fd.files[path]
	return data, ok
}

func (fd *fakeDaemon) serve() {
	for {
		msgType, data, err := fd.conn.ReadMessage()
		if err != nil {
			return
		}
		if msgType != websocket.TextMessage {
			continue
		}
		var req struct {
			Type        string `json:"type"`
			RequestID   string `json:"request_id"`
			Path        string `json:"path"`
			Data        string `json:"data"`
			Offset      int64  `json:"offset"`
			Size        int    `json:"size"`
			UploadID    string `json:"upload_id"`
			ChunkIndex  int    `json:"chunk_index"`
			TotalChunks int    `json:"total_chunks"`
			ChunkSize   int    `json:"chunk_size"`
		}
		if json.Unmarshal(data, &req) != nil {
			continue
		}
		switch req.Type {
		case pkg.TypeGetDisk:
			fd.write(pkg.GetDiskResponse{Type: req.Type, RequestID: req.RequestID, FreeBytes: 1 << 30, TotalBytes: 2 << 30})
		case pkg.TypeListDir:
			fd.mu.Lock()
			entries := make([]pkg.FileEntry, 0, len(fd.files))
			for name, content := range fd.files {
				entries = append(entries, pkg.FileEntry{Name: name, Size: int64(len(content)), Mtime: "2026-01-01T00:00:00Z"})
			}
			fd.mu.Unlock()
			fd.write(pkg.ListDirResponse{Type: req.Type, RequestID: req.RequestID, Entries: entries})
		case pkg.TypeGetMeta:
			if req.Path == "." {
				fd.write(pkg.GetMetaResponse{Type: req.Type, RequestID: req.RequestID, IsDir: true})
				break
			}
			if content, ok := fd.get(req.Path); ok {
				fd.write(pkg.GetMetaResponse{Type: req.Type, RequestID: req.RequestID, Size: int64(len(content)), Mtime: "2026-01-01T00:00:00Z"})
			} else {
				fd.write(pkg.GetMetaResponse{Type: req.Type, RequestID: req.RequestID, Error: "no such file"})
			}
		case pkg.TypeReadFile:
			if content, ok := fd.get(req.Path); ok {
				fd.write(pkg.ReadFileResponse{Type: req.Type, RequestID: req.RequestID, Data: base64.StdEncoding.EncodeToString(content)})
			} else {
				fd.write(pkg.ReadFileResponse{Type: req.Type, RequestID: req.RequestID, Error: "no such file"})
			}
		case pkg.TypeReadChunk:
			content, ok := fd.get(req.Path)
			if !ok || req.Offset >= int64(len(content)) {
				fd.write(pkg.ReadChunkResponse{Type: req.Type, RequestID: req.RequestID, Error: "bad chunk"})
				break
			}
			end := req.Offset + int64(req.Size)
			if end > int64(len(content)) {
				end = int64(len(content))
			}
			chunk := content[req.Offset:end]
			fd.write(pkg.ReadChunkResponse{Type: req.Type, RequestID: req.RequestID, ChunkSize: len(chunk)})
			fd.writeBinary(chunk)
		case pkg.TypeWriteFile:
			decoded, err := base64.StdEncoding.DecodeString(req.Data)
			if err != nil {
				fd.write(pkg.WriteFileResponse{Type: req.Type, RequestID: req.RequestID, Error: "bad data"})
				break
			}
			fd.mu.Lock()
			fd.files[req.Path] = decoded
			fd.mu.Unlock()
			fd.write(pkg.WriteFileResponse{Type: req.Type, RequestID: req.RequestID})
		case pkg.TypeWriteChunk:
			// Control frame is followed by one binary frame with the chunk.
			binType, chunk, err := fd.conn.ReadMessage()
			if err != nil || binType != websocket.BinaryMessage {
				fd.write(pkg.WriteChunkResponse{Type: req.Type, RequestID: req.RequestID, UploadID: req.UploadID, ChunkIndex: req.ChunkIndex, Error: "missing binary frame"})
				break
			}
			fd.mu.Lock()
			if fd.chunks[req.UploadID] == nil {
				fd.chunks[req.UploadID] = make([][]byte, req.TotalChunks)
			}
			fd.chunks[req.UploadID][req.ChunkIndex] = chunk
			if req.ChunkIndex == req.TotalChunks-1 {
				fd.files[req.Path] = bytes.Join(fd.chunks[req.UploadID], nil)
				delete(fd.chunks, req.UploadID)
			}
			fd.mu.Unlock()
			fd.write(pkg.WriteChunkResponse{Type: req.Type, RequestID: req.RequestID, UploadID: req.UploadID, ChunkIndex: req.ChunkIndex})
		case pkg.TypeDeleteFile:
			fd.mu.Lock()
			_, ok := fd.files[req.Path]
			delete(fd.files, req.Path)
			fd.mu.Unlock()
			if ok {
				fd.write(pkg.DeleteFileResponse{Type: req.Type, RequestID: req.RequestID})
			} else {
				fd.write(pkg.DeleteFileResponse{Type: req.Type, RequestID: req.RequestID, Error: "no such file"})
			}
		}
	}
}

func (fd *fakeDaemon) write(v any) {
	if err := fd.conn.WriteJSON(v); err != nil {
		fd.t.Logf("fake daemon write: %v", err)
	}
}

func (fd *fakeDaemon) writeBinary(data []byte) {
	if err := fd.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		fd.t.Logf("fake daemon write binary: %v", err)
	}
}

// createDaemon provisions a daemon via the API and returns (id, token).
func createDaemon(t *testing.T, c *http.Client, baseURL, label string) (string, string) {
	t.Helper()
	resp := postJSON(t, c, baseURL+"/api/daemons", map[string]string{"label": label})
	wantStatus(t, resp, http.StatusCreated)
	created := decodeJSON[map[string]string](t, resp)
	if created["id"] == "" || created["token"] == "" {
		t.Fatalf("create daemon response missing id/token: %v", created)
	}
	return created["id"], created["token"]
}

// --- auth flow -------------------------------------------------------------

func TestIntegrationRegistrationFlow(t *testing.T) {
	_, ts := newTestServer(t)
	c := client(t)

	resp, err := c.Get(ts.URL + "/api/setup")
	if err != nil {
		t.Fatal(err)
	}
	if open := decodeJSON[map[string]bool](t, resp); !open["registration_open"] {
		t.Fatal("fresh instance should have registration open")
	}

	registerUser(t, c, ts.URL, "zach", "correct horse battery staple")

	resp, err = c.Get(ts.URL + "/api/setup")
	if err != nil {
		t.Fatal(err)
	}
	if open := decodeJSON[map[string]bool](t, resp); open["registration_open"] {
		t.Fatal("registration must close after the first user")
	}

	// Single-user system: second registration is rejected.
	resp = postJSON(t, c, ts.URL+"/api/register/totp-setup", map[string]string{})
	wantStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()
}

func TestIntegrationLoginFlow(t *testing.T) {
	_, ts := newTestServer(t)
	c := client(t)
	secret := registerUser(t, c, ts.URL, "zach", "correct horse battery staple")

	// Wrong password → generic 401.
	resp := postJSON(t, c, ts.URL+"/api/login", map[string]string{"username": "zach", "password": "wrong"})
	wantStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()

	// Protected endpoint without a session → 401.
	resp, err := c.Get(ts.URL + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()

	// Full password + TOTP login → session cookie works on /api/me.
	login(t, c, ts.URL, "zach", "correct horse battery staple", secret)
	resp, err = c.Get(ts.URL + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusOK)
	me := decodeJSON[map[string]string](t, resp)
	if me["username"] != "zach" {
		t.Fatalf("me.username = %q, want zach", me["username"])
	}
}

func TestIntegrationLoginWrongTOTP(t *testing.T) {
	_, ts := newTestServer(t)
	c := client(t)
	secret := registerUser(t, c, ts.URL, "zach", "pw12345678")

	resp := postJSON(t, c, ts.URL+"/api/login", map[string]string{"username": "zach", "password": "pw12345678"})
	wantStatus(t, resp, http.StatusOK)
	challenge := decodeJSON[map[string]any](t, resp)

	resp = postJSON(t, c, ts.URL+"/api/login/totp", map[string]string{
		"login_token": challenge["login_token"].(string), "code": wrongTOTPCode(t, secret),
	})
	wantStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()

	// The TOTP challenge token must not work as a session token.
	req, _ := http.NewRequest("GET", ts.URL+"/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+challenge["login_token"].(string))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
}

func TestIntegrationLogoutRevokesAllSessions(t *testing.T) {
	_, ts := newTestServer(t)
	c := client(t)
	secret := registerUser(t, c, ts.URL, "zach", "pw12345678")
	cookie := login(t, c, ts.URL, "zach", "pw12345678", secret)

	resp := postJSON(t, c, ts.URL+"/api/logout", map[string]string{})
	wantStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	// Replaying the pre-logout cookie must fail: logout bumps token_version,
	// revoking every outstanding session, not just clearing the cookie.
	req, _ := http.NewRequest("GET", ts.URL+"/api/me", nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
}

func TestIntegrationAuthRateLimit(t *testing.T) {
	_, ts := newTestServer(t)
	c := client(t)
	// The per-account limiter locks a username after 5 failed password attempts
	// (stricter than, and independent of, the per-IP limiter).
	for i := 0; i < 5; i++ {
		resp := postJSON(t, c, ts.URL+"/api/login", map[string]string{"username": "ghost", "password": "nope"})
		wantStatus(t, resp, http.StatusUnauthorized)
		resp.Body.Close()
	}
	resp := postJSON(t, c, ts.URL+"/api/login", map[string]string{"username": "ghost", "password": "nope"})
	wantStatus(t, resp, http.StatusTooManyRequests)
	resp.Body.Close()
}

func TestIntegrationTOTPLockout(t *testing.T) {
	_, ts := newTestServer(t)
	c := client(t)
	secret := registerUser(t, c, ts.URL, "zach", "pw12345678")

	resp := postJSON(t, c, ts.URL+"/api/login", map[string]string{"username": "zach", "password": "pw12345678"})
	wantStatus(t, resp, http.StatusOK)
	challenge := decodeJSON[map[string]any](t, resp)
	loginToken := challenge["login_token"].(string)

	bad := wrongTOTPCode(t, secret)
	for i := 0; i < 5; i++ {
		resp = postJSON(t, c, ts.URL+"/api/login/totp", map[string]string{"login_token": loginToken, "code": bad})
		wantStatus(t, resp, http.StatusUnauthorized)
		resp.Body.Close()
	}
	// Sixth attempt hits the per-account lockout — even with the right code.
	good, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	resp = postJSON(t, c, ts.URL+"/api/login/totp", map[string]string{"login_token": loginToken, "code": good})
	wantStatus(t, resp, http.StatusTooManyRequests)
	resp.Body.Close()
}

// --- daemon WebSocket auth ---------------------------------------------------

func TestIntegrationDaemonWSAuth(t *testing.T) {
	_, ts := newTestServer(t)
	c := client(t)
	secret := registerUser(t, c, ts.URL, "zach", "pw12345678")
	login(t, c, ts.URL, "zach", "pw12345678", secret)
	_, token := createDaemon(t, c, ts.URL, "test-host")

	// Wrong token → auth_error.
	conn, reply := dialDaemonWS(t, ts.URL, "wrong-token")
	conn.Close()
	if reply["type"] != pkg.TypeAuthError {
		t.Fatalf("expected auth_error for bad token, got %v", reply)
	}

	// Right token → auth_ok and the daemon shows connected with disk stats.
	startFakeDaemon(t, ts.URL, token, nil)
	resp, err := c.Get(ts.URL + "/api/daemons")
	if err != nil {
		t.Fatal(err)
	}
	daemons := decodeJSON[[]map[string]any](t, resp)
	if len(daemons) != 1 || daemons[0]["connected"] != true {
		t.Fatalf("expected one connected daemon, got %v", daemons)
	}
	if daemons[0]["disk_free"] != float64(1<<30) || daemons[0]["disk_total"] != float64(2<<30) {
		t.Fatalf("disk stats missing or wrong: %v", daemons[0])
	}
}

func TestIntegrationDaemonAPIRequiresSession(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/daemons")
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
}

// --- file proxy --------------------------------------------------------------

func TestIntegrationFileProxyRoundTrip(t *testing.T) {
	_, ts := newTestServer(t)
	c := client(t)
	secret := registerUser(t, c, ts.URL, "zach", "pw12345678")
	login(t, c, ts.URL, "zach", "pw12345678", secret)
	id, token := createDaemon(t, c, ts.URL, "files-host")

	content := []byte("hello from the daemon side\n")
	fd := startFakeDaemon(t, ts.URL, token, map[string][]byte{"notes.txt": content})
	filesURL := ts.URL + "/api/daemons/" + id + "/files"

	// List.
	resp, err := c.Get(filesURL + "?path=.")
	if err != nil {
		t.Fatal(err)
	}
	entries := decodeJSON[[]pkg.FileEntry](t, resp)
	if len(entries) != 1 || entries[0].Name != "notes.txt" || entries[0].Size != int64(len(content)) {
		t.Fatalf("unexpected listing: %v", entries)
	}

	// Download (small-file path).
	resp, err = c.Get(filesURL + "?path=notes.txt&download=1")
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusOK)
	got := readAll(t, resp)
	if !bytes.Equal(got, content) {
		t.Fatalf("downloaded %q, want %q", got, content)
	}

	// Upload (single-shot path) and read it back through the daemon map.
	uploaded := []byte("uploaded body")
	req, _ := http.NewRequest("PUT", filesURL+"?path=upload.txt", bytes.NewReader(uploaded))
	resp, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
	if stored, ok := fd.get("upload.txt"); !ok || !bytes.Equal(stored, uploaded) {
		t.Fatalf("daemon stored %q, want %q", stored, uploaded)
	}

	// Delete.
	req, _ = http.NewRequest("DELETE", filesURL+"?path=upload.txt", nil)
	resp, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
	if _, ok := fd.get("upload.txt"); ok {
		t.Fatal("file still present after delete")
	}

	// Daemon-side error surfaces as a 400, not a 5xx.
	resp, err = c.Get(filesURL + "?path=missing.txt&download=1")
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestIntegrationFileProxyDaemonOffline(t *testing.T) {
	_, ts := newTestServer(t)
	c := client(t)
	secret := registerUser(t, c, ts.URL, "zach", "pw12345678")
	login(t, c, ts.URL, "zach", "pw12345678", secret)
	id, _ := createDaemon(t, c, ts.URL, "offline-host")

	resp, err := c.Get(ts.URL + "/api/daemons/" + id + "/files?path=.")
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusServiceUnavailable)
	resp.Body.Close()
}

func TestIntegrationFileProxyRejectsUnownedDaemon(t *testing.T) {
	srv, ts := newTestServer(t)
	c := client(t)
	secret := registerUser(t, c, ts.URL, "zach", "pw12345678")
	login(t, c, ts.URL, "zach", "pw12345678", secret)

	// A daemon owned by a *different* user must be invisible (404, not 503),
	// even though it exists in the table — the ownership gate, not connectivity.
	var otherUser string
	if err := srv.pool.QueryRow(context.Background(),
		`INSERT INTO users (username, password_hash) VALUES ('intruder', 'x') RETURNING id::text`).Scan(&otherUser); err != nil {
		t.Fatal(err)
	}
	var otherDaemon string
	if err := srv.pool.QueryRow(context.Background(),
		`INSERT INTO daemons (label, token_hash, hosted_path, owner_id) VALUES ('theirs', 'h', '.', $1) RETURNING id::text`,
		otherUser).Scan(&otherDaemon); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/files?path=.", "/meta?path=."} {
		resp, err := c.Get(ts.URL + "/api/daemons/" + otherDaemon + path)
		if err != nil {
			t.Fatal(err)
		}
		wantStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	}
	// The owner's own list must not include the other user's daemon.
	resp, err := c.Get(ts.URL + "/api/daemons")
	if err != nil {
		t.Fatal(err)
	}
	list := decodeJSON[[]map[string]any](t, resp)
	for _, d := range list {
		if d["id"] == otherDaemon {
			t.Fatal("ListDaemons leaked another user's daemon")
		}
	}
}

func TestIntegrationChunkedUpload(t *testing.T) {
	_, ts := newTestServer(t)
	c := client(t)
	secret := registerUser(t, c, ts.URL, "zach", "pw12345678")
	login(t, c, ts.URL, "zach", "pw12345678", secret)
	id, token := createDaemon(t, c, ts.URL, "chunk-host")
	fd := startFakeDaemon(t, ts.URL, token, nil)

	chunk0 := bytes.Repeat([]byte("A"), 1024)
	chunk1 := bytes.Repeat([]byte("B"), 512)
	base := ts.URL + "/api/daemons/" + id + "/files?path=big.bin&upload_id=u1&total_chunks=2&chunk_index="

	for i, chunk := range [][]byte{chunk0, chunk1} {
		req, _ := http.NewRequest("PUT", fmt.Sprintf("%s%d", base, i), bytes.NewReader(chunk))
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		wantStatus(t, resp, http.StatusOK)
		ack := decodeJSON[map[string]any](t, resp)
		if int(ack["chunk_index"].(float64)) != i {
			t.Fatalf("ack chunk_index = %v, want %d", ack["chunk_index"], i)
		}
	}
	want := append(append([]byte{}, chunk0...), chunk1...)
	if stored, ok := fd.get("big.bin"); !ok || !bytes.Equal(stored, want) {
		t.Fatalf("assembled file wrong: got %d bytes, want %d", len(stored), len(want))
	}
}

func TestIntegrationLargeFileChunkedDownload(t *testing.T) {
	_, ts := newTestServer(t)
	c := client(t)
	secret := registerUser(t, c, ts.URL, "zach", "pw12345678")
	login(t, c, ts.URL, "zach", "pw12345678", secret)
	id, token := createDaemon(t, c, ts.URL, "large-host")

	// Over downloadChunkSize (5 MB) so the bastion takes the streaming
	// read_chunk path: two chunks (5 MB + 512 KB) over binary frames.
	large := bytes.Repeat([]byte("0123456789abcdef"), (5<<20+512<<10)/16)
	startFakeDaemon(t, ts.URL, token, map[string][]byte{"large.bin": large})

	resp, err := c.Get(ts.URL + "/api/daemons/" + id + "/files?path=large.bin&download=1")
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusOK)
	if got := resp.Header.Get("Content-Length"); got != fmt.Sprint(len(large)) {
		t.Fatalf("Content-Length = %s, want %d", got, len(large))
	}
	got := readAll(t, resp)
	if !bytes.Equal(got, large) {
		t.Fatalf("downloaded %d bytes, want %d (content mismatch)", len(got), len(large))
	}
}

func readAll(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer resp.Body.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	return buf.Bytes()
}
