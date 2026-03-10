package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
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

func (s *Server) proxyReadFile(ctx context.Context, w http.ResponseWriter, ac *DaemonConn, path string) {
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
	w.Write(data)
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
