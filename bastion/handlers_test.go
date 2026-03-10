package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSONError(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSONError(rr, http.StatusBadRequest, "bad input")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["error"] != "bad input" {
		t.Errorf("error = %q, want %q", body["error"], "bad input")
	}
}

func TestWriteJSONErrorVariousCodes(t *testing.T) {
	codes := []int{
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusInternalServerError,
		http.StatusServiceUnavailable,
	}
	for _, code := range codes {
		rr := httptest.NewRecorder()
		writeJSONError(rr, code, "test")
		if rr.Code != code {
			t.Errorf("status = %d, want %d", rr.Code, code)
		}
	}
}
