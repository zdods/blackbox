package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"blackbox/pkg"

	"github.com/google/uuid"
)

const proxyTimeout = 30 * time.Second
const maxUploadSize = 512 << 20 // 512 MB

func (s *Server) DaemonFiles(w http.ResponseWriter, r *http.Request) {
	daemonID := r.PathValue("id")
	if daemonID == "" {
		writeJSONError(w, http.StatusBadRequest, "daemon id required")
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
		s.proxyReadFile(ctx, w, ac, path)
		return
	}
	if r.Method == http.MethodGet {
		s.proxyListDir(ctx, w, ac, path)
		return
	}
	if r.Method == http.MethodPut {
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
	daemonID := r.PathValue("id")
	if daemonID == "" {
		writeJSONError(w, http.StatusBadRequest, "daemon id required")
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
		log.Printf("daemon meta request: %v", err)
		writeJSONError(w, http.StatusBadGateway, errMsgUnavailable)
		return
	}
	var resp pkg.GetMetaResponse
	if json.Unmarshal(respData, &resp) != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid response")
		return
	}
	if resp.Error != "" {
		log.Printf("daemon meta error: %s", resp.Error)
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
		log.Printf("daemon list-dir request: %v", err)
		writeJSONError(w, http.StatusBadGateway, errMsgUnavailable)
		return
	}
	var resp pkg.ListDirResponse
	if json.Unmarshal(respData, &resp) != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid response")
		return
	}
	if resp.Error != "" {
		log.Printf("daemon list-dir error: %s", resp.Error)
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
		log.Printf("daemon read-file meta: %v", err)
		writeJSONError(w, http.StatusBadGateway, errMsgUnavailable)
		return
	}
	var meta pkg.GetMetaResponse
	if json.Unmarshal(metaData, &meta) != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid response")
		return
	}
	if meta.Error != "" {
		log.Printf("daemon read-file meta error: %s", meta.Error)
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
	w.Header().Set("Content-Disposition", "attachment")
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
			log.Printf("daemon read-chunk: %v", err)
			return // headers already sent, can't write JSON error
		}
		var resp pkg.ReadChunkResponse
		if json.Unmarshal(respJSON, &resp) != nil || resp.Error != "" {
			if resp.Error != "" {
				log.Printf("daemon read-chunk error: %s", resp.Error)
			}
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
		log.Printf("daemon read-file request: %v", err)
		writeJSONError(w, http.StatusBadGateway, errMsgUnavailable)
		return
	}
	var resp pkg.ReadFileResponse
	if json.Unmarshal(respData, &resp) != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid response")
		return
	}
	if resp.Error != "" {
		log.Printf("daemon read-file error: %s", resp.Error)
		writeJSONError(w, http.StatusBadRequest, "operation failed")
		return
	}
	data, err := base64.StdEncoding.DecodeString(resp.Data)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid data")
		return
	}
	w.Header().Set("Content-Disposition", "attachment")
	_, _ = w.Write(data) // client disconnect mid-download is not actionable
}

func (s *Server) proxyWriteFile(ctx context.Context, w http.ResponseWriter, r *http.Request, ac *DaemonConn, path string) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	data, err := io.ReadAll(r.Body)
	if err != nil {
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
		log.Printf("daemon write-file request: %v", err)
		writeJSONError(w, http.StatusBadGateway, errMsgUnavailable)
		return
	}
	var resp pkg.WriteFileResponse
	if json.Unmarshal(respData, &resp) != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid response")
		return
	}
	if resp.Error != "" {
		log.Printf("daemon write-file error: %s", resp.Error)
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

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	chunkData, err := io.ReadAll(r.Body)
	if err != nil {
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
		log.Printf("daemon write-chunk request: %v", err)
		writeJSONError(w, http.StatusBadGateway, errMsgUnavailable)
		return
	}
	var resp pkg.WriteChunkResponse
	if json.Unmarshal(respData, &resp) != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid response")
		return
	}
	if resp.Error != "" {
		log.Printf("daemon write-chunk error: %s", resp.Error)
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
		log.Printf("daemon delete-file request: %v", err)
		writeJSONError(w, http.StatusBadGateway, errMsgUnavailable)
		return
	}
	var resp pkg.DeleteFileResponse
	if json.Unmarshal(respData, &resp) != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid response")
		return
	}
	if resp.Error != "" {
		log.Printf("daemon delete-file error: %s", resp.Error)
		writeJSONError(w, http.StatusBadRequest, "operation failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
