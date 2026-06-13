package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"blackhaul/pkg"

	"github.com/google/uuid"
)

const proxyTimeout = 30 * time.Second

// Upload memory bounds. Large files MUST use the chunked protocol; the
// single-shot PUT is only for small files. Both caps sit just above the
// client's 5 MB chunk size so the bastion never buffers a large body in RAM
// for a single request — bounded per-request memory is what makes uploads
// "stream" through this WebSocket proxy.
const (
	maxSingleShotUpload    = 6 << 20 // single, non-chunked PUT
	maxChunkUpload         = 6 << 20 // one chunk of a chunked PUT
	maxConcurrentTransfers = 16      // simultaneous uploads/downloads proxied
)

// acquireTransfer bounds how many uploads/downloads proxy through the bastion
// at once, capping worst-case memory under concurrent load. It blocks until a
// slot frees or the request context is cancelled; returns false on the latter.
func (s *Server) acquireTransfer(ctx context.Context) bool {
	select {
	case s.transferSem <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *Server) releaseTransfer() { <-s.transferSem }

// contentDisposition builds an RFC 6266 attachment header so a direct-navigation
// download saves under the file's real name. It emits both a sanitized ASCII
// filename and a UTF-8 filename* for non-ASCII names.
func contentDisposition(p string) string {
	name := p
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		name = p[i+1:]
	}
	if name == "" {
		name = "download"
	}
	ascii := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '"' || r == '\\' || r > 0x7e {
			return '_'
		}
		return r
	}, name)
	return fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s", ascii, url.PathEscape(name))
}

func (s *Server) DaemonFiles(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	daemonID := r.PathValue("id")
	if daemonID == "" {
		writeJSONError(w, http.StatusBadRequest, "daemon id required")
		return
	}
	if claims == nil || !s.daemonOwnedBy(r.Context(), daemonID, claims.UserID) {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "."
	}
	ac := s.hub.Get(daemonID)
	if ac == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "daemon not connected")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), proxyTimeout)
	defer cancel()
	if r.Method == http.MethodGet && r.URL.Query().Get("download") == "1" {
		if !s.acquireTransfer(ctx) {
			writeJSONError(w, http.StatusServiceUnavailable, "server busy; try again")
			return
		}
		defer s.releaseTransfer()
		s.proxyReadFile(ctx, w, ac, path)
		return
	}
	if r.Method == http.MethodGet {
		s.proxyListDir(ctx, w, ac, path)
		return
	}
	if r.Method == http.MethodPut {
		if !s.acquireTransfer(ctx) {
			writeJSONError(w, http.StatusServiceUnavailable, "server busy; try again")
			return
		}
		defer s.releaseTransfer()
		// Chunked upload if upload_id, chunk_index, total_chunks are present
		if r.URL.Query().Get("upload_id") != "" {
			s.proxyWriteChunk(ctx, w, r, ac, path)
			return
		}
		s.proxyWriteFile(ctx, w, r, ac, path)
		return
	}
	if r.Method == http.MethodDelete {
		s.proxyDeleteFile(ctx, w, ac, path)
		return
	}
	writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (s *Server) DaemonMeta(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	daemonID := r.PathValue("id")
	if daemonID == "" {
		writeJSONError(w, http.StatusBadRequest, "daemon id required")
		return
	}
	if claims == nil || !s.daemonOwnedBy(r.Context(), daemonID, claims.UserID) {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "."
	}
	ac := s.hub.Get(daemonID)
	if ac == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "daemon not connected")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), proxyTimeout)
	defer cancel()
	reqID := uuid.New().String()
	req := pkg.GetMetaRequest{Type: pkg.TypeGetMeta, RequestID: reqID, Path: path}
	respData, err := ac.Request(ctx, reqID, req)
	if err != nil {
		reqLog(ctx).Error("daemon meta request failed", "err", err)
		writeJSONError(w, http.StatusBadGateway, errMsgUnavailable)
		return
	}
	var resp pkg.GetMetaResponse
	if json.Unmarshal(respData, &resp) != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid response")
		return
	}
	if resp.Error != "" {
		reqLog(ctx).Warn("daemon meta error", "daemon_err", resp.Error)
		writeJSONError(w, http.StatusBadRequest, "operation failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"size":   resp.Size,
		"mtime":  resp.Mtime,
		"is_dir": resp.IsDir,
	})
}

func (s *Server) proxyListDir(ctx context.Context, w http.ResponseWriter, ac *DaemonConn, path string) {
	reqID := uuid.New().String()
	req := pkg.ListDirRequest{Type: pkg.TypeListDir, RequestID: reqID, Path: path}
	respData, err := ac.Request(ctx, reqID, req)
	if err != nil {
		reqLog(ctx).Error("daemon list-dir request failed", "err", err)
		writeJSONError(w, http.StatusBadGateway, errMsgUnavailable)
		return
	}
	var resp pkg.ListDirResponse
	if json.Unmarshal(respData, &resp) != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid response")
		return
	}
	if resp.Error != "" {
		reqLog(ctx).Warn("daemon list-dir error", "daemon_err", resp.Error)
		writeJSONError(w, http.StatusBadRequest, "operation failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp.Entries)
}

const downloadChunkSize = 5 * 1024 * 1024 // 5 MB

func (s *Server) proxyReadFile(ctx context.Context, w http.ResponseWriter, ac *DaemonConn, path string) {
	// Get file size first
	metaReqID := uuid.New().String()
	metaReq := pkg.GetMetaRequest{Type: pkg.TypeGetMeta, RequestID: metaReqID, Path: path}
	metaData, err := ac.Request(ctx, metaReqID, metaReq)
	if err != nil {
		reqLog(ctx).Error("daemon read-file meta failed", "err", err)
		writeJSONError(w, http.StatusBadGateway, errMsgUnavailable)
		return
	}
	var meta pkg.GetMetaResponse
	if json.Unmarshal(metaData, &meta) != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid response")
		return
	}
	if meta.Error != "" {
		reqLog(ctx).Warn("daemon read-file meta error", "daemon_err", meta.Error)
		writeJSONError(w, http.StatusBadRequest, "operation failed")
		return
	}
	if meta.IsDir {
		writeJSONError(w, http.StatusBadRequest, "cannot download a directory")
		return
	}

	// Small files: use legacy single-request path (avoids overhead of chunked protocol)
	if meta.Size <= int64(downloadChunkSize) {
		s.proxyReadFileSmall(ctx, w, ac, path)
		return
	}

	// Large files: stream via read_chunk with binary frames
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", contentDisposition(path))
	w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))
	flusher, _ := w.(http.Flusher)
	var offset int64
	for offset < meta.Size {
		chunkSize := downloadChunkSize
		if remaining := meta.Size - offset; remaining < int64(chunkSize) {
			chunkSize = int(remaining)
		}
		chunkCtx, cancel := context.WithTimeout(ctx, chunkTimeout)
		reqID := uuid.New().String()
		req := pkg.ReadChunkRequest{
			Type:      pkg.TypeReadChunk,
			RequestID: reqID,
			Path:      path,
			Offset:    offset,
			Size:      chunkSize,
		}
		respJSON, chunkData, err := ac.RequestExpectBinary(chunkCtx, reqID, req)
		cancel()
		if err != nil {
			reqLog(ctx).Error("daemon read-chunk failed", "err", err)
			return // headers already sent, can't write JSON error
		}
		var resp pkg.ReadChunkResponse
		if json.Unmarshal(respJSON, &resp) != nil || resp.Error != "" {
			if resp.Error != "" {
				reqLog(ctx).Warn("daemon read-chunk error", "daemon_err", resp.Error)
			}
			return
		}
		// Don't trust the daemon to honor the requested size: a chunk larger
		// than asked, or an empty chunk while bytes remain, would overrun the
		// declared Content-Length or spin forever. Bail rather than emit a body
		// that doesn't match the announced length.
		if len(chunkData) == 0 || len(chunkData) > chunkSize {
			reqLog(ctx).Warn("daemon read-chunk size mismatch", "got", len(chunkData), "want", chunkSize)
			return
		}
		if _, err := w.Write(chunkData); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
		offset += int64(len(chunkData))
	}
}

func (s *Server) proxyReadFileSmall(ctx context.Context, w http.ResponseWriter, ac *DaemonConn, path string) {
	reqID := uuid.New().String()
	req := pkg.ReadFileRequest{Type: pkg.TypeReadFile, RequestID: reqID, Path: path}
	respData, err := ac.Request(ctx, reqID, req)
	if err != nil {
		reqLog(ctx).Error("daemon read-file request failed", "err", err)
		writeJSONError(w, http.StatusBadGateway, errMsgUnavailable)
		return
	}
	var resp pkg.ReadFileResponse
	if json.Unmarshal(respData, &resp) != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid response")
		return
	}
	if resp.Error != "" {
		reqLog(ctx).Warn("daemon read-file error", "daemon_err", resp.Error)
		writeJSONError(w, http.StatusBadRequest, "operation failed")
		return
	}
	data, err := base64.StdEncoding.DecodeString(resp.Data)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid data")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", contentDisposition(path))
	_, _ = w.Write(data) // client disconnect mid-download is not actionable
}

func (s *Server) proxyWriteFile(ctx context.Context, w http.ResponseWriter, r *http.Request, ac *DaemonConn, path string) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSingleShotUpload)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "file too large for a single request; use chunked upload")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	reqID := uuid.New().String()
	req := pkg.WriteFileRequest{
		Type:      pkg.TypeWriteFile,
		RequestID: reqID,
		Path:      path,
		Data:      base64.StdEncoding.EncodeToString(data),
	}
	respData, err := ac.Request(ctx, reqID, req)
	if err != nil {
		reqLog(ctx).Error("daemon write-file request failed", "err", err)
		writeJSONError(w, http.StatusBadGateway, errMsgUnavailable)
		return
	}
	var resp pkg.WriteFileResponse
	if json.Unmarshal(respData, &resp) != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid response")
		return
	}
	if resp.Error != "" {
		reqLog(ctx).Warn("daemon write-file error", "daemon_err", resp.Error)
		writeJSONError(w, http.StatusBadRequest, "operation failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

const chunkTimeout = 60 * time.Second

func (s *Server) proxyWriteChunk(ctx context.Context, w http.ResponseWriter, r *http.Request, ac *DaemonConn, path string) {
	q := r.URL.Query()
	uploadID := q.Get("upload_id")
	chunkIndex, err1 := strconv.Atoi(q.Get("chunk_index"))
	totalChunks, err2 := strconv.Atoi(q.Get("total_chunks"))
	if err1 != nil || err2 != nil || totalChunks <= 0 || chunkIndex < 0 || chunkIndex >= totalChunks {
		writeJSONError(w, http.StatusBadRequest, "invalid chunk parameters")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxChunkUpload)
	chunkData, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "chunk too large")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "failed to read chunk body")
		return
	}

	chunkCtx, cancel := context.WithTimeout(ctx, chunkTimeout)
	defer cancel()

	reqID := uuid.New().String()
	req := pkg.WriteChunkRequest{
		Type:        pkg.TypeWriteChunk,
		RequestID:   reqID,
		UploadID:    uploadID,
		Path:        path,
		ChunkIndex:  chunkIndex,
		TotalChunks: totalChunks,
		ChunkSize:   len(chunkData),
	}
	respData, err := ac.RequestWithBinary(chunkCtx, reqID, req, chunkData)
	if err != nil {
		reqLog(ctx).Error("daemon write-chunk request failed", "err", err)
		writeJSONError(w, http.StatusBadGateway, errMsgUnavailable)
		return
	}
	var resp pkg.WriteChunkResponse
	if json.Unmarshal(respData, &resp) != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid response")
		return
	}
	if resp.Error != "" {
		reqLog(ctx).Warn("daemon write-chunk error", "daemon_err", resp.Error)
		writeJSONError(w, http.StatusBadRequest, "operation failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"chunk_index": resp.ChunkIndex,
		"upload_id":   resp.UploadID,
	})
}

func (s *Server) proxyDeleteFile(ctx context.Context, w http.ResponseWriter, ac *DaemonConn, path string) {
	reqID := uuid.New().String()
	req := pkg.DeleteFileRequest{Type: pkg.TypeDeleteFile, RequestID: reqID, Path: path}
	respData, err := ac.Request(ctx, reqID, req)
	if err != nil {
		reqLog(ctx).Error("daemon delete-file request failed", "err", err)
		writeJSONError(w, http.StatusBadGateway, errMsgUnavailable)
		return
	}
	var resp pkg.DeleteFileResponse
	if json.Unmarshal(respData, &resp) != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid response")
		return
	}
	if resp.Error != "" {
		reqLog(ctx).Warn("daemon delete-file error", "daemon_err", resp.Error)
		writeJSONError(w, http.StatusBadRequest, "operation failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
