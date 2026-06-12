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

	// Load config file (missing file is not an error)
	cfgURL, cfgHosted, err := loadConfig(cfgPath)
	if err != nil {
		log.Printf("warning: could not read config %s: %v", cfgPath, err)
	}

	// Load token from OS keyring
	keyringTok, err := loadToken()
	if err != nil && err != keyring.ErrNotFound {
		log.Printf("warning: could not read token from keyring: %v", err)
	}

	// Priority: flags > env vars > keyring > config file
	url := firstNonEmpty(*bastionURL, os.Getenv("BLACKHAUL_BASTION_URL"), cfgURL)
	tok := firstNonEmpty(*token, os.Getenv("BLACKHAUL_TOKEN"), keyringTok)
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

	// Offer to save config after interactive setup
	if fromSetup {
		fmt.Printf("\n  save settings? (config: %s, token: OS keyring) [y/N]: ", cfgPath)
		reader := bufio.NewReader(os.Stdin)
		ans, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(ans)) == "y" {
			if err := saveToken(tok); err != nil {
				log.Printf("warning: could not save token to keyring: %v", err)
			} else {
				log.Printf("token saved to OS keyring")
			}
			if err := saveConfig(cfgPath, url, path); err != nil {
				log.Printf("warning: could not save config: %v", err)
			} else {
				log.Printf("config saved to %s", cfgPath)
			}
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
				log.Fatalf("auth failed repeatedly; check your token in blackhaul-console and restart the daemon")
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

// firstNonEmpty returns the first non-empty string from vals.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
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
	path        string
	totalChunks int
	received    map[int]bool
	tmpDir      string
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

func init() {
	// Background goroutine to clean up stale uploads
	go func() {
		for {
			time.Sleep(2 * time.Minute)
			activeUploads.Lock()
			for id, u := range activeUploads.m {
				u.mu.Lock()
				if time.Since(u.lastActive) > uploadTimeout {
					os.RemoveAll(u.tmpDir)
					delete(activeUploads.m, id)
					log.Printf("cleaned up stale upload %s", id)
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

	destPath := safePath(root, req.Path)
	if destPath == "" {
		return errResp("invalid path")
	}

	// Get or create upload state
	activeUploads.Lock()
	u, exists := activeUploads.m[req.UploadID]
	if !exists {
		tmpDir := filepath.Join(root, tmpDirPrefix, req.UploadID)
		if err := os.MkdirAll(tmpDir, 0755); err != nil {
			activeUploads.Unlock()
			return errResp("failed to create temp dir")
		}
		u = &uploadState{
			path:        req.Path,
			totalChunks: req.TotalChunks,
			received:    make(map[int]bool),
			tmpDir:      tmpDir,
			lastActive:  time.Now(),
		}
		activeUploads.m[req.UploadID] = u
	}
	activeUploads.Unlock()

	u.mu.Lock()
	defer u.mu.Unlock()
	u.lastActive = time.Now()

	// Write chunk to temp file
	chunkFile := filepath.Join(u.tmpDir, fmt.Sprintf("chunk_%d", req.ChunkIndex))
	if err := os.WriteFile(chunkFile, chunkData, 0644); err != nil {
		return errResp("failed to write chunk")
	}
	u.received[req.ChunkIndex] = true

	// If all chunks received, assemble the file
	if len(u.received) == u.totalChunks {
		if dir := filepath.Dir(destPath); dir != destPath {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return errResp("failed to create directory")
			}
		}
		outFile, err := os.Create(destPath)
		if err != nil {
			return errResp("failed to create output file")
		}
		for i := 0; i < u.totalChunks; i++ {
			cf := filepath.Join(u.tmpDir, fmt.Sprintf("chunk_%d", i))
			data, err := os.ReadFile(cf)
			if err != nil {
				outFile.Close()
				os.Remove(destPath)
				return errResp("missing chunk during assembly")
			}
			if _, err := outFile.Write(data); err != nil {
				outFile.Close()
				os.Remove(destPath)
				return errResp("failed to write assembled file")
			}
		}
		outFile.Close()

		// Clean up temp dir and upload state
		os.RemoveAll(u.tmpDir)
		activeUploads.Lock()
		delete(activeUploads.m, req.UploadID)
		activeUploads.Unlock()
		log.Printf("assembled chunked upload: %s (%d chunks)", req.Path, u.totalChunks)
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
		log.Printf("auth failed: %s", authResp.Error)
		return errAuthFailed
	}
	if authResp.Type != pkg.TypeAuthOK {
		log.Printf("unexpected auth response: %s", authResp.Type)
		return nil
	}
	log.Printf("blackhaul daemon connected (id %s)", authResp.DaemonID)
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

// safePath returns absolute path under root, or empty string if escape.
func safePath(root, rel string) string {
	rel = filepath.Clean(rel)
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	abs := filepath.Join(root, rel)
	abs = filepath.Clean(abs)
	if !strings.HasPrefix(abs, filepath.Clean(root)+string(filepath.Separator)) && abs != filepath.Clean(root) {
		return ""
	}
	return abs
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
			log.Printf("list dir entry %s: %v", e.Name(), err)
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
		if err := os.MkdirAll(dir, 0755); err != nil {
			return pkg.WriteFileResponse{Type: pkg.TypeWriteFile, RequestID: req.RequestID, Error: err.Error()}
		}
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
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
