package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"blackbox/pkg"

	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

const defaultBastionURL = "ws://localhost:8080/ws/agent"

var errAuthFailed = fmt.Errorf("auth failed")

func main() {
	bastionURL := flag.String("bastion-url", "", "blackbox-server WebSocket URL")
	token := flag.String("token", "", "blackbox agent token (from blackbox-console)")
	hostedPath := flag.String("hosted-path", "", "Root directory to expose (e.g. /path/to/dir or C:\\Users\\you\\files)")
	flag.Parse()

	url, tok, path := *bastionURL, *token, *hostedPath
	if tok == "" || path == "" {
		url, tok, path = runSetup(url, tok, path)
	}
	if url == "" {
		url = defaultBastionURL
	}

	root, err := resolveDir(path)
	if err != nil {
		log.Fatalf("hosted-path: %v", err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		log.Fatalf("hosted-path must be an existing directory: %v", err)
	}
	authFailures := 0
	for {
		err := runAgent(url, tok, root)
		if err == errAuthFailed {
			authFailures++
			if authFailures >= 3 {
				log.Fatalf("auth failed repeatedly; check your token in blackbox-console and restart the agent")
			}
		} else {
			authFailures = 0
		}
		log.Println("blackbox agent disconnected; reconnecting in 5s...")
		time.Sleep(5 * time.Second)
	}
}

// runSetup prompts for host, directory, and token when not provided. Returns (url, token, hostedPath).
func runSetup(url, token, hostedPath string) (string, string, string) {
	fmt.Println()
	fmt.Println("  [▪‿▪]  blackbox-agent setup")
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
		} else if path == "~/" || strings.HasPrefix(path, "~/") {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		} else {
			// ~user not supported; treat as literal
		}
	}
	return filepath.Abs(path)
}

func runAgent(bastionURL, token, root string) error {
	header := http.Header{}
	conn, _, err := websocket.DefaultDialer.Dial(bastionURL, header)
	if err != nil {
		log.Printf("dial: %v", err)
		return nil
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
		Type    string `json:"type"`
		AgentID string `json:"agent_id"`
		Error   string `json:"error"`
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
	log.Printf("blackbox agent connected (id %s)", authResp.AgentID)
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
				conn.WriteJSON(resp)
			}
		case pkg.TypeReadFile:
			var req pkg.ReadFileRequest
			if json.Unmarshal(data, &req) == nil {
				resp := handleReadFile(root, &req)
				conn.WriteJSON(resp)
			}
		case pkg.TypeWriteFile:
			var req pkg.WriteFileRequest
			if json.Unmarshal(data, &req) == nil {
				resp := handleWriteFile(root, &req)
				conn.WriteJSON(resp)
			}
		case pkg.TypeGetMeta:
			var req pkg.GetMetaRequest
			if json.Unmarshal(data, &req) == nil {
				resp := handleGetMeta(root, &req)
				conn.WriteJSON(resp)
			}
		case pkg.TypeDeleteFile:
			var req pkg.DeleteFileRequest
			if json.Unmarshal(data, &req) == nil {
				resp := handleDeleteFile(root, &req)
				conn.WriteJSON(resp)
			}
		case pkg.TypeGetDisk:
			var req pkg.GetDiskRequest
			if json.Unmarshal(data, &req) == nil {
				resp := handleGetDisk(root, &req)
				conn.WriteJSON(resp)
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
		info, _ := e.Info()
		var size int64
		var mtime string
		if info != nil {
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
