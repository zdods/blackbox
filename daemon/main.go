package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"blackhaul/pkg"
	"blackhaul/pkg/version"

	"github.com/gorilla/websocket"
	"github.com/zalando/go-keyring"
	"golang.org/x/term"
)

const defaultBastionURL = "ws://localhost:8080/ws/daemon"

var errAuthFailed = errors.New("auth failed")
var errDialFailed = errors.New("dial failed")

func main() {
	bastionURL := flag.String("bastion-url", "", "blackhaul-server WebSocket URL")
	token := flag.String("token", "", "blackhaul daemon token (from blackhaul-console)")
	hostedPath := flag.String("hosted-path", "", "Root directory to expose (e.g. /path/to/dir or C:\\Users\\you\\files)")
	configPath := flag.String("config", "", "Path to config file (default: ~/.blackhaul-daemon)")
	showVersion := flag.Bool("version", false, "Print version and exit")
	logout := flag.Bool("logout", false, "Remove the stored daemon token from the OS keyring and exit")
	reset := flag.Bool("reset", false, "Remove the stored token (keyring) and the config file, then exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("blackhaul-daemon " + version.Version)
		return
	}

	// Resolve config file path (expand ~ for both default and user-supplied paths)
	cfgPath := *configPath
	if cfgPath == "" {
		cfgPath = "~/.blackhaul-daemon"
	}
	var err error
	cfgPath, err = resolveDir(cfgPath)
	if err != nil {
		log.Fatalf("config path: %v", err)
	}

	// Credential cleanup commands: clear locally stored credentials and exit.
	// --logout removes the keyring token; --reset also removes the config file.
	if *logout || *reset {
		runCleanup(cfgPath, *reset)
		return
	}

	// Load config file (missing file is not an error)
	cfgURL, cfgHosted, err := loadConfig(cfgPath)
	if err != nil {
		log.Printf("warning: could not read config %s: %v", cfgPath, err)
	}

	// Priority: flags > env vars > keyring > config file.
	// Only consult the keyring when the token wasn't supplied directly — this
	// avoids a spurious "keyring unavailable" warning on headless hosts that
	// pass the token via flag or environment.
	url := firstNonEmpty(*bastionURL, os.Getenv("BLACKHAUL_BASTION_URL"), cfgURL)
	tok := firstNonEmpty(*token, os.Getenv("BLACKHAUL_TOKEN"))
	if tok == "" {
		keyringTok, err := loadToken()
		if err != nil && err != keyring.ErrNotFound {
			log.Printf("warning: could not read token from keyring: %v", err)
		}
		tok = keyringTok
	}
	path := firstNonEmpty(*hostedPath, os.Getenv("BLACKHAUL_HOSTED_PATH"), cfgHosted)

	// Fall back to interactive setup if required values are still missing
	fromSetup := false
	if tok == "" || path == "" {
		url, tok, path = runSetup(url, tok, path)
		fromSetup = true
	}
	if url == "" {
		url = defaultBastionURL
	}

	// Offer to save config after interactive setup. Only offer to store the
	// token when this host actually has a usable keyring — on headless Linux
	// with no Secret Service there's nowhere secure to put it, so we save the
	// config (URL + path) and explain how to supply the token on each start.
	if fromSetup {
		keyringOK := keyringAvailable()
		tokenDest := "OS keyring"
		if !keyringOK {
			tokenDest = "not stored — no keyring on this host"
		}
		fmt.Printf("\n  save settings? (config: %s, token: %s) [y/N]: ", cfgPath, tokenDest)
		reader := bufio.NewReader(os.Stdin)
		ans, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(ans)) == "y" {
			if keyringOK {
				if err := saveToken(tok); err != nil {
					log.Printf("warning: could not save token to keyring: %v", err)
				} else {
					log.Printf("token saved to OS keyring")
				}
			}
			if err := saveConfig(cfgPath, url, path); err != nil {
				log.Printf("warning: could not save config: %v", err)
			} else {
				log.Printf("config saved to %s", cfgPath)
			}
		}
		if !keyringOK {
			printNoKeyringHelp()
		}
	}

	root, err := resolveDir(path)
	if err != nil {
		log.Fatalf("hosted-path: %v", err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		log.Fatalf("hosted-path must be an existing directory: %v", err)
	}

	authFailures := 0
	backoff := 2 * time.Second
	for {
		err := runDaemon(url, tok, root)
		switch err {
		case errAuthFailed:
			authFailures++
			if authFailures >= 3 {
				log.Fatalf("auth failed repeatedly — the server rejected this token (it may have been deleted in the console). Verify the host in blackhaul-console, or run 'blackhaul-daemon --logout' to clear the stored token, then restart.")
			}
			log.Printf("auth failed (%d/3); reconnecting in %v...", authFailures, backoff)
		case errDialFailed:
			authFailures = 0
			log.Printf("could not reach server; reconnecting in %v...", backoff)
		default: // nil — was connected, session ended normally
			authFailures = 0
			backoff = 2 * time.Second // reset after a successful session
			log.Printf("disconnected; reconnecting in %v...", backoff)
		}
		time.Sleep(backoff)
		if backoff < 60*time.Second {
			backoff *= 2
			if backoff > 60*time.Second {
				backoff = 60 * time.Second
			}
		}
	}
}

// dialFailureMessage explains a failed websocket dial, using the HTTP
// response the server sent instead of the upgrade to point at common
// reverse-proxy misconfigurations.
func dialFailureMessage(bastionURL string, resp *http.Response, err error) string {
	if resp == nil {
		return fmt.Sprintf("dial %s: %v", bastionURL, err)
	}
	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		loc := resp.Header.Get("Location")
		hint := ""
		if strings.HasPrefix(bastionURL, "ws://") && strings.HasPrefix(loc, "https://") {
			hint = " — the server redirects to HTTPS; use a wss:// URL"
		}
		return fmt.Sprintf("dial %s: %v (HTTP %d redirect to %s)%s", bastionURL, err, resp.StatusCode, loc, hint)
	case resp.StatusCode == http.StatusBadRequest:
		return fmt.Sprintf("dial %s: %v (HTTP 400 — if the server is behind a reverse proxy, the proxy must forward the Upgrade and Connection headers for /ws/; see docs/deployment.md)", bastionURL, err)
	default:
		return fmt.Sprintf("dial %s: %v (HTTP %d)", bastionURL, err, resp.StatusCode)
	}
}

// runCleanup removes locally stored daemon credentials and prints what it did.
// It always clears the OS keyring token; when full is true (--reset) it also
// removes the config file. Both operations are idempotent. Server-side
// revocation is done by deleting the host in the console.
func runCleanup(cfgPath string, full bool) {
	removed, err := deleteToken()
	switch {
	case err != nil:
		log.Printf("warning: could not remove token from the OS keyring: %v", err)
	case removed:
		fmt.Println("  removed the daemon token from the OS keyring")
	default:
		fmt.Println("  no daemon token was stored in the OS keyring")
	}

	if full {
		switch err := os.Remove(cfgPath); {
		case err == nil:
			fmt.Printf("  removed config file %s\n", cfgPath)
		case os.IsNotExist(err):
			fmt.Printf("  no config file at %s\n", cfgPath)
		default:
			log.Printf("warning: could not remove config %s: %v", cfgPath, err)
		}
	}

	fmt.Println()
	fmt.Println("  local credentials cleared. To fully revoke access, also delete")
	fmt.Println("  this host in the blackhaul console.")
}

// firstNonEmpty returns the first non-empty string from vals.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// printNoKeyringHelp explains how to supply the token on hosts without a usable
// OS keyring (typically headless Linux with no D-Bus Secret Service), where the
// token can't be stored and must be provided on each start.
func printNoKeyringHelp() {
	fmt.Println()
	fmt.Println("  note: no OS keyring is available on this host (common on headless")
	fmt.Println("        Linux with no desktop session). The token can't be stored, so")
	fmt.Println("        provide it on each start via an environment variable:")
	fmt.Println()
	fmt.Println("          BLACKHAUL_TOKEN=<token> blackhaul-daemon")
	fmt.Println()
	fmt.Println("        or a 0600 EnvironmentFile in your systemd unit. See")
	fmt.Println("        docs/deployment.md for a headless service example.")
}

// runSetup prompts for host, directory, and token when not provided. Returns (url, token, hostedPath).
func runSetup(url, token, hostedPath string) (string, string, string) {
	fmt.Println()
	fmt.Println("  [▪‿▪]  blackhaul-daemon setup")
	fmt.Println()
	scan := bufio.NewScanner(os.Stdin)

	if url == "" {
		fmt.Printf("  host [%s]: ", defaultBastionURL)
		if scan.Scan() {
			s := strings.TrimSpace(scan.Text())
			if s != "" {
				url = s
			} else {
				url = defaultBastionURL
			}
		}
		if url == "" {
			url = defaultBastionURL
		}
	}
	if hostedPath == "" {
		fmt.Print("  directory to serve (absolute path, e.g. ~/files): ")
		if scan.Scan() {
			hostedPath = strings.TrimSpace(scan.Text())
		}
		for hostedPath == "" {
			fmt.Print("  directory to serve: ")
			if scan.Scan() {
				hostedPath = strings.TrimSpace(scan.Text())
			}
		}
	}
	if token == "" {
		token = readTokenLine(scan)
		for token == "" {
			token = readTokenLine(scan)
		}
	}
	fmt.Println()
	fmt.Println("  [▪‿▪]  connecting...")
	fmt.Println()
	return url, token, hostedPath
}

// readTokenLine reads the token with masking when stdin is a TTY.
func readTokenLine(scan *bufio.Scanner) string {
	fmt.Print("  token (from console, paste then enter): ")
	if term.IsTerminal(int(os.Stdin.Fd())) {
		line, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(line))
	}
	if scan.Scan() {
		return strings.TrimSpace(scan.Text())
	}
	return ""
}

// resolveDir expands ~ to home and returns an absolute path. Path is not relative to cwd.
func resolveDir(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else if strings.HasPrefix(path, "~/") {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
		// ~user not supported; treated as a literal path
	}
	return filepath.Abs(path)
}

// uploadState tracks an in-progress chunked upload.
type uploadState struct {
	destPath    string // resolved once at creation; later chunks can't change it
	totalChunks int
	received    map[int]bool
	tmpDir      string
	createdAt   time.Time
	lastActive  time.Time
	mu          sync.Mutex
}

// activeUploads tracks in-progress chunked uploads by upload_id.
var activeUploads = struct {
	sync.Mutex
	m map[string]*uploadState
}{m: make(map[string]*uploadState)}

const tmpDirPrefix = ".blackhaul-tmp"
const uploadTimeout = 10 * time.Minute

// Resource bounds. Every path/size/count below is supplied by the bastion and
// must be treated as untrusted, so each is capped to keep a malicious or
// compromised bastion from escaping the hosted root or exhausting the host.
const (
	maxReadBytes     = 8 << 20   // 8 MB cap on read_chunk / read_file responses
	maxChunkBytes    = 8 << 20   // 8 MB cap on an accepted upload chunk
	maxTotalChunks   = 1_000_000 // bounds the assembly loop and state map
	maxActiveUploads = 128       // concurrent chunked uploads
)

// uploadIDRe constrains upload_id to a safe charset before it is ever used in a
// filesystem path (it names a temp directory under the hosted root).
var uploadIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func init() {
	// Background goroutine to clean up stale uploads
	go func() {
		for {
			time.Sleep(2 * time.Minute)
			activeUploads.Lock()
			for id, u := range activeUploads.m {
				u.mu.Lock()
				// Idle timeout, plus an absolute age cap so an attacker can't
				// keep an upload alive indefinitely by dribbling chunks.
				if time.Since(u.lastActive) > uploadTimeout || time.Since(u.createdAt) > 2*uploadTimeout {
					os.RemoveAll(u.tmpDir)
					delete(activeUploads.m, id)
					log.Printf("cleaned up stale upload %s", logSafe(id))
				}
				u.mu.Unlock()
			}
			activeUploads.Unlock()
		}
	}()
}

func handleWriteChunk(root string, req *pkg.WriteChunkRequest, chunkData []byte) pkg.WriteChunkResponse {
	errResp := func(msg string) pkg.WriteChunkResponse {
		return pkg.WriteChunkResponse{
			Type:       pkg.TypeWriteChunk,
			RequestID:  req.RequestID,
			UploadID:   req.UploadID,
			ChunkIndex: req.ChunkIndex,
			Error:      msg,
		}
	}

	// Validate every bastion-supplied field before it touches the filesystem.
	if !uploadIDRe.MatchString(req.UploadID) {
		return errResp("invalid upload id")
	}
	if req.TotalChunks <= 0 || req.TotalChunks > maxTotalChunks {
		return errResp("invalid total chunks")
	}
	if req.ChunkIndex < 0 || req.ChunkIndex >= req.TotalChunks {
		return errResp("invalid chunk index")
	}
	if len(chunkData) > maxChunkBytes {
		return errResp("chunk too large")
	}

	destPath := safePath(root, req.Path)
	if destPath == "" {
		return errResp("invalid path")
	}

	// Get or create upload state. destPath/totalChunks are bound once at
	// creation; later chunks reusing this upload_id cannot retarget the write.
	activeUploads.Lock()
	u, exists := activeUploads.m[req.UploadID]
	if !exists {
		if len(activeUploads.m) >= maxActiveUploads {
			activeUploads.Unlock()
			return errResp("too many active uploads")
		}
		tmpDir := filepath.Join(root, tmpDirPrefix, req.UploadID)
		if err := os.MkdirAll(tmpDir, 0700); err != nil {
			activeUploads.Unlock()
			return errResp("failed to create temp dir")
		}
		now := time.Now()
		u = &uploadState{
			destPath:    destPath,
			totalChunks: req.TotalChunks,
			received:    make(map[int]bool),
			tmpDir:      tmpDir,
			createdAt:   now,
			lastActive:  now,
		}
		activeUploads.m[req.UploadID] = u
	}
	activeUploads.Unlock()

	u.mu.Lock()
	defer u.mu.Unlock()
	if req.ChunkIndex >= u.totalChunks {
		return errResp("invalid chunk index")
	}
	u.lastActive = time.Now()

	// Write chunk to temp file
	chunkFile := filepath.Join(u.tmpDir, fmt.Sprintf("chunk_%d", req.ChunkIndex))
	if err := os.WriteFile(chunkFile, chunkData, 0600); err != nil {
		return errResp("failed to write chunk")
	}
	u.received[req.ChunkIndex] = true

	// If all chunks received, assemble into a temp file and atomically rename
	// onto destPath, so a failed/partial upload never truncates an existing file.
	if len(u.received) == u.totalChunks {
		if dir := filepath.Dir(u.destPath); dir != u.destPath {
			if err := os.MkdirAll(dir, 0700); err != nil {
				return errResp("failed to create directory")
			}
		}
		assembled := filepath.Join(u.tmpDir, "assembled")
		outFile, err := os.OpenFile(assembled, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return errResp("failed to create output file")
		}
		for i := 0; i < u.totalChunks; i++ {
			cf := filepath.Join(u.tmpDir, fmt.Sprintf("chunk_%d", i))
			data, err := os.ReadFile(cf)
			if err != nil {
				outFile.Close()
				return errResp("missing chunk during assembly")
			}
			if _, err := outFile.Write(data); err != nil {
				outFile.Close()
				return errResp("failed to write assembled file")
			}
		}
		if err := outFile.Sync(); err != nil {
			outFile.Close()
			return errResp("failed to flush assembled file")
		}
		outFile.Close()
		if err := os.Rename(assembled, u.destPath); err != nil {
			return errResp("failed to finalize upload")
		}

		// Clean up temp dir and upload state
		os.RemoveAll(u.tmpDir)
		activeUploads.Lock()
		delete(activeUploads.m, req.UploadID)
		activeUploads.Unlock()
		log.Printf("assembled chunked upload: %s (%d chunks)", logSafe(u.destPath), u.totalChunks)
	}

	return pkg.WriteChunkResponse{
		Type:       pkg.TypeWriteChunk,
		RequestID:  req.RequestID,
		UploadID:   req.UploadID,
		ChunkIndex: req.ChunkIndex,
	}
}

func handleReadChunk(root string, req *pkg.ReadChunkRequest) (*pkg.ReadChunkResponse, []byte) {
	path := safePath(root, req.Path)
	if path == "" {
		return &pkg.ReadChunkResponse{Type: pkg.TypeReadChunk, RequestID: req.RequestID, Error: "invalid path"}, nil
	}
	if req.Size <= 0 || req.Size > maxReadBytes {
		return &pkg.ReadChunkResponse{Type: pkg.TypeReadChunk, RequestID: req.RequestID, Error: "invalid chunk size"}, nil
	}
	if req.Offset < 0 {
		return &pkg.ReadChunkResponse{Type: pkg.TypeReadChunk, RequestID: req.RequestID, Error: "invalid offset"}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return &pkg.ReadChunkResponse{Type: pkg.TypeReadChunk, RequestID: req.RequestID, Error: err.Error()}, nil
	}
	defer f.Close()
	buf := make([]byte, req.Size)
	n, err := f.ReadAt(buf, req.Offset)
	if err != nil && n == 0 {
		return &pkg.ReadChunkResponse{Type: pkg.TypeReadChunk, RequestID: req.RequestID, Error: err.Error()}, nil
	}
	return &pkg.ReadChunkResponse{
		Type:      pkg.TypeReadChunk,
		RequestID: req.RequestID,
		ChunkSize: n,
	}, buf[:n]
}

func runDaemon(bastionURL, token, root string) error {
	header := http.Header{}
	conn, resp, err := websocket.DefaultDialer.Dial(bastionURL, header)
	if err != nil {
		log.Print(dialFailureMessage(bastionURL, resp, err))
		if resp != nil {
			resp.Body.Close()
		}
		return errDialFailed
	}
	defer conn.Close()
	// Send auth
	if err := conn.WriteJSON(pkg.Auth{Type: pkg.TypeAuth, Token: token}); err != nil {
		log.Printf("auth send: %v", err)
		return nil
	}
	// Read auth response
	_, data, err := conn.ReadMessage()
	if err != nil {
		log.Printf("auth read: %v", err)
		return nil
	}
	var authResp struct {
		Type     string `json:"type"`
		DaemonID string `json:"daemon_id"`
		Error    string `json:"error"`
	}
	if err := json.Unmarshal(data, &authResp); err != nil {
		log.Printf("auth parse: %v", err)
		return nil
	}
	if authResp.Type == pkg.TypeAuthError {
		log.Printf("auth failed: %s", logSafe(authResp.Error))
		return errAuthFailed
	}
	if authResp.Type != pkg.TypeAuthOK {
		log.Printf("unexpected auth response: %s", logSafe(authResp.Type))
		return nil
	}
	log.Printf("blackhaul daemon connected (id %s)", logSafe(authResp.DaemonID))
	// Message loop
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("read: %v", err)
			return nil
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			continue
		}
		switch envelope.Type {
		case pkg.TypeListDir:
			var req pkg.ListDirRequest
			if json.Unmarshal(data, &req) == nil {
				resp := handleListDir(root, &req)
				if err := conn.WriteJSON(resp); err != nil {
					log.Printf("write: %v", err)
					return nil
				}
			}
		case pkg.TypeReadFile:
			var req pkg.ReadFileRequest
			if json.Unmarshal(data, &req) == nil {
				resp := handleReadFile(root, &req)
				if err := conn.WriteJSON(resp); err != nil {
					log.Printf("write: %v", err)
					return nil
				}
			}
		case pkg.TypeWriteFile:
			var req pkg.WriteFileRequest
			if json.Unmarshal(data, &req) == nil {
				resp := handleWriteFile(root, &req)
				if err := conn.WriteJSON(resp); err != nil {
					log.Printf("write: %v", err)
					return nil
				}
			}
		case pkg.TypeGetMeta:
			var req pkg.GetMetaRequest
			if json.Unmarshal(data, &req) == nil {
				resp := handleGetMeta(root, &req)
				if err := conn.WriteJSON(resp); err != nil {
					log.Printf("write: %v", err)
					return nil
				}
			}
		case pkg.TypeDeleteFile:
			var req pkg.DeleteFileRequest
			if json.Unmarshal(data, &req) == nil {
				resp := handleDeleteFile(root, &req)
				if err := conn.WriteJSON(resp); err != nil {
					log.Printf("write: %v", err)
					return nil
				}
			}
		case pkg.TypeGetDisk:
			var req pkg.GetDiskRequest
			if json.Unmarshal(data, &req) == nil {
				resp := handleGetDisk(root, &req)
				if err := conn.WriteJSON(resp); err != nil {
					log.Printf("write: %v", err)
					return nil
				}
			}
		case pkg.TypeReadChunk:
			var req pkg.ReadChunkRequest
			if json.Unmarshal(data, &req) == nil {
				resp, chunkData := handleReadChunk(root, &req)
				if resp.Error != "" {
					// Error — send JSON only, no binary follow-up
					if err := conn.WriteJSON(resp); err != nil {
						log.Printf("write: %v", err)
						return nil
					}
				} else {
					// Send JSON control + binary data atomically
					if err := conn.WriteJSON(resp); err != nil {
						log.Printf("write: %v", err)
						return nil
					}
					if err := conn.WriteMessage(websocket.BinaryMessage, chunkData); err != nil {
						log.Printf("write chunk: %v", err)
						return nil
					}
				}
			}
		case pkg.TypeWriteChunk:
			var req pkg.WriteChunkRequest
			if json.Unmarshal(data, &req) == nil {
				// Read the next message which should be the binary chunk data
				msgType, chunkData, err := conn.ReadMessage()
				if err != nil {
					log.Printf("read chunk data: %v", err)
					return nil
				}
				if msgType != websocket.BinaryMessage {
					resp := pkg.WriteChunkResponse{
						Type:       pkg.TypeWriteChunk,
						RequestID:  req.RequestID,
						UploadID:   req.UploadID,
						ChunkIndex: req.ChunkIndex,
						Error:      "expected binary frame",
					}
					if err := conn.WriteJSON(resp); err != nil {
						log.Printf("write: %v", err)
						return nil
					}
					continue
				}
				resp := handleWriteChunk(root, &req, chunkData)
				if err := conn.WriteJSON(resp); err != nil {
					log.Printf("write: %v", err)
					return nil
				}
			}
		}
	}
}

// safePath returns an absolute path under root, or "" if the request escapes
// the hosted root. It applies a lexical check (clean + reject "..") and then,
// when the path exists on disk, a symlink check: the deepest existing ancestor
// is resolved with EvalSymlinks and re-verified to be contained, so a symlink
// inside the root that points outside it cannot be used to read/write/delete
// beyond the hosted directory. The symlink check is skipped only when nothing
// on the path exists yet (a fresh write target, or a synthetic test root) —
// non-existent components cannot be symlinks.
func safePath(root, rel string) string {
	rel = filepath.Clean(rel)
	// filepath.IsLocal rejects absolute paths, "..", and (on Windows) reserved
	// names lexically — a stronger, standard guard than a manual ".." check, and
	// one static analysis recognizes as a path-traversal sanitizer.
	if !filepath.IsLocal(rel) {
		return ""
	}
	rootClean := filepath.Clean(root)
	abs := filepath.Clean(filepath.Join(rootClean, rel))
	if abs != rootClean && !strings.HasPrefix(abs, rootClean+string(filepath.Separator)) {
		return ""
	}
	if resolvedRoot, err := filepath.EvalSymlinks(rootClean); err == nil {
		if real := resolveExisting(abs); real != "" {
			if real != resolvedRoot && !strings.HasPrefix(real, resolvedRoot+string(filepath.Separator)) {
				return ""
			}
		}
	}
	return abs
}

// logSafe strips control characters (notably CR/LF) from an attacker-influenced
// value before it is logged, so a crafted path, filename, or remote message
// cannot forge or inject log lines.
// logSafe neutralizes a remote- or user-controlled string for safe logging: it
// maps every control character (< 0x20, which includes newlines, tab, and ESC)
// and DEL (0x7f) to a space, so the value can't forge log lines or inject
// terminal escape sequences.
//
// The trailing strings.ReplaceAll calls for "\n" and "\r" are redundant at
// runtime (strings.Map has already removed them), but they are the exact
// pattern CodeQL's go/log-injection query recognizes as a sanitizer
// (ReplaceSanitizer). Without them the analyzer can't see through strings.Map
// and reports false-positive log-injection alerts at every logSafe call site.
// Keep them.
func logSafe(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

// resolveExisting resolves the deepest existing ancestor of p with EvalSymlinks
// (following any symlinks) and re-appends the non-existent tail components. It
// returns "" if no ancestor up to the filesystem root resolves. The re-appended
// tail components don't exist yet, so they cannot themselves be symlinks.
func resolveExisting(p string) string {
	cur := p
	var tail []string
	for {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		tail = append(tail, filepath.Base(cur))
		cur = parent
	}
}

func handleListDir(root string, req *pkg.ListDirRequest) pkg.ListDirResponse {
	path := safePath(root, req.Path)
	if path == "" {
		return pkg.ListDirResponse{Type: pkg.TypeListDir, RequestID: req.RequestID, Error: "invalid path"}
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return pkg.ListDirResponse{Type: pkg.TypeListDir, RequestID: req.RequestID, Error: err.Error()}
	}
	var out []pkg.FileEntry
	for _, e := range entries {
		if e.Name() == tmpDirPrefix {
			continue
		}
		info, err := e.Info()
		var size int64
		var mtime string
		if err != nil {
			log.Printf("list dir entry %s: %v", logSafe(e.Name()), err)
		} else if info != nil {
			size = info.Size()
			mtime = info.ModTime().Format("2006-01-02T15:04:05Z07:00")
		}
		out = append(out, pkg.FileEntry{Name: e.Name(), IsDir: e.IsDir(), Size: size, Mtime: mtime})
	}
	return pkg.ListDirResponse{Type: pkg.TypeListDir, RequestID: req.RequestID, Entries: out}
}

func handleReadFile(root string, req *pkg.ReadFileRequest) pkg.ReadFileResponse {
	path := safePath(root, req.Path)
	if path == "" {
		return pkg.ReadFileResponse{Type: pkg.TypeReadFile, RequestID: req.RequestID, Error: "invalid path"}
	}
	// Bound memory: this single-shot path is for small files only; large files
	// must use read_chunk. Reject oversized files instead of reading them whole.
	if info, err := os.Stat(path); err == nil && info.Size() > maxReadBytes {
		return pkg.ReadFileResponse{Type: pkg.TypeReadFile, RequestID: req.RequestID, Error: "file too large; use chunked read"}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return pkg.ReadFileResponse{Type: pkg.TypeReadFile, RequestID: req.RequestID, Error: err.Error()}
	}
	if req.Offset > 0 || req.Size > 0 {
		if req.Offset >= int64(len(data)) {
			data = nil
		} else {
			end := req.Offset + req.Size
			if req.Size == 0 {
				end = int64(len(data))
			}
			if end > int64(len(data)) {
				end = int64(len(data))
			}
			data = data[req.Offset:end]
		}
	}
	return pkg.ReadFileResponse{
		Type:      pkg.TypeReadFile,
		RequestID: req.RequestID,
		Data:      base64Encode(data),
	}
}

func handleWriteFile(root string, req *pkg.WriteFileRequest) pkg.WriteFileResponse {
	path := safePath(root, req.Path)
	if path == "" {
		return pkg.WriteFileResponse{Type: pkg.TypeWriteFile, RequestID: req.RequestID, Error: "invalid path"}
	}
	data, err := base64Decode(req.Data)
	if err != nil {
		return pkg.WriteFileResponse{Type: pkg.TypeWriteFile, RequestID: req.RequestID, Error: err.Error()}
	}
	if dir := filepath.Dir(path); dir != path {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return pkg.WriteFileResponse{Type: pkg.TypeWriteFile, RequestID: req.RequestID, Error: err.Error()}
		}
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return pkg.WriteFileResponse{Type: pkg.TypeWriteFile, RequestID: req.RequestID, Error: err.Error()}
	}
	return pkg.WriteFileResponse{Type: pkg.TypeWriteFile, RequestID: req.RequestID}
}

func handleGetMeta(root string, req *pkg.GetMetaRequest) pkg.GetMetaResponse {
	path := safePath(root, req.Path)
	if path == "" {
		return pkg.GetMetaResponse{Type: pkg.TypeGetMeta, RequestID: req.RequestID, Error: "invalid path"}
	}
	info, err := os.Stat(path)
	if err != nil {
		return pkg.GetMetaResponse{Type: pkg.TypeGetMeta, RequestID: req.RequestID, Error: err.Error()}
	}
	return pkg.GetMetaResponse{
		Type:      pkg.TypeGetMeta,
		RequestID: req.RequestID,
		Size:      info.Size(),
		Mtime:     info.ModTime().Format("2006-01-02T15:04:05Z07:00"),
		IsDir:     info.IsDir(),
	}
}

func handleGetDisk(root string, req *pkg.GetDiskRequest) pkg.GetDiskResponse {
	free, total, err := getDiskSpace(root)
	if err != nil {
		return pkg.GetDiskResponse{Type: pkg.TypeGetDisk, RequestID: req.RequestID, Error: err.Error()}
	}
	return pkg.GetDiskResponse{
		Type:       pkg.TypeGetDisk,
		RequestID:  req.RequestID,
		FreeBytes:  free,
		TotalBytes: total,
	}
}

func handleDeleteFile(root string, req *pkg.DeleteFileRequest) pkg.DeleteFileResponse {
	path := safePath(root, req.Path)
	if path == "" {
		return pkg.DeleteFileResponse{Type: pkg.TypeDeleteFile, RequestID: req.RequestID, Error: "invalid path"}
	}
	if err := os.RemoveAll(path); err != nil {
		return pkg.DeleteFileResponse{Type: pkg.TypeDeleteFile, RequestID: req.RequestID, Error: err.Error()}
	}
	return pkg.DeleteFileResponse{Type: pkg.TypeDeleteFile, RequestID: req.RequestID}
}
