package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"blackbox/pkg"

	"github.com/google/uuid"
)

const proxyTimeout = 30 * time.Second

func (s *Server) AgentFiles(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "blackbox agent id required", http.StatusBadRequest)
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "."
	}
	ac := s.hub.Get(agentID)
	if ac == nil {
		http.Error(w, "blackbox agent not connected", http.StatusServiceUnavailable)
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
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) AgentMeta(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "blackbox agent id required", http.StatusBadRequest)
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "."
	}
	ac := s.hub.Get(agentID)
	if ac == nil {
		http.Error(w, "blackbox agent not connected", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), proxyTimeout)
	defer cancel()
	reqID := uuid.New().String()
	req := pkg.GetMetaRequest{Type: pkg.TypeGetMeta, RequestID: reqID, Path: path}
	respData, err := ac.Request(ctx, reqID, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	var resp pkg.GetMetaResponse
	if json.Unmarshal(respData, &resp) != nil {
		http.Error(w, "invalid response", http.StatusBadGateway)
		return
	}
	if resp.Error != "" {
		http.Error(w, resp.Error, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"size":   resp.Size,
		"mtime":  resp.Mtime,
		"is_dir": resp.IsDir,
	})
}

func (s *Server) proxyListDir(ctx context.Context, w http.ResponseWriter, ac *AgentConn, path string) {
	reqID := uuid.New().String()
	req := pkg.ListDirRequest{Type: pkg.TypeListDir, RequestID: reqID, Path: path}
	respData, err := ac.Request(ctx, reqID, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	var resp pkg.ListDirResponse
	if json.Unmarshal(respData, &resp) != nil {
		http.Error(w, "invalid response", http.StatusBadGateway)
		return
	}
	if resp.Error != "" {
		http.Error(w, resp.Error, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp.Entries)
}

func (s *Server) proxyReadFile(ctx context.Context, w http.ResponseWriter, ac *AgentConn, path string) {
	reqID := uuid.New().String()
	req := pkg.ReadFileRequest{Type: pkg.TypeReadFile, RequestID: reqID, Path: path}
	respData, err := ac.Request(ctx, reqID, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	var resp pkg.ReadFileResponse
	if json.Unmarshal(respData, &resp) != nil {
		http.Error(w, "invalid response", http.StatusBadGateway)
		return
	}
	if resp.Error != "" {
		http.Error(w, resp.Error, http.StatusBadRequest)
		return
	}
	data, err := base64.StdEncoding.DecodeString(resp.Data)
	if err != nil {
		http.Error(w, "invalid data", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Disposition", "attachment")
	w.Write(data)
}

func (s *Server) proxyWriteFile(ctx context.Context, w http.ResponseWriter, r *http.Request, ac *AgentConn, path string) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
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
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	var resp pkg.WriteFileResponse
	if json.Unmarshal(respData, &resp) != nil {
		http.Error(w, "invalid response", http.StatusBadGateway)
		return
	}
	if resp.Error != "" {
		http.Error(w, resp.Error, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) proxyDeleteFile(ctx context.Context, w http.ResponseWriter, ac *AgentConn, path string) {
	reqID := uuid.New().String()
	req := pkg.DeleteFileRequest{Type: pkg.TypeDeleteFile, RequestID: reqID, Path: path}
	respData, err := ac.Request(ctx, reqID, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	var resp pkg.DeleteFileResponse
	if json.Unmarshal(respData, &resp) != nil {
		http.Error(w, "invalid response", http.StatusBadGateway)
		return
	}
	if resp.Error != "" {
		http.Error(w, resp.Error, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
