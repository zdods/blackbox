package main

import (
	"testing"
)

func TestBase64RoundTrip(t *testing.T) {
	original := []byte("hello, world!")
	encoded := base64Encode(original)
	decoded, err := base64Decode(encoded)
	if err != nil {
		t.Fatalf("base64Decode: %v", err)
	}
	if string(decoded) != string(original) {
		t.Errorf("round trip: got %q, want %q", decoded, original)
	}
}

func TestBase64Empty(t *testing.T) {
	encoded := base64Encode(nil)
	decoded, err := base64Decode(encoded)
	if err != nil {
		t.Fatalf("base64Decode empty: %v", err)
	}
	if len(decoded) != 0 {
		t.Errorf("expected empty, got %d bytes", len(decoded))
	}
}

func TestBase64DecodeBadInput(t *testing.T) {
	_, err := base64Decode("not!valid!base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestBase64Binary(t *testing.T) {
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	encoded := base64Encode(data)
	decoded, err := base64Decode(encoded)
	if err != nil {
		t.Fatalf("base64Decode: %v", err)
	}
	if len(decoded) != 256 {
		t.Fatalf("decoded length = %d, want 256", len(decoded))
	}
	for i, b := range decoded {
		if b != byte(i) {
			t.Errorf("byte %d = %d, want %d", i, b, i)
			break
		}
	}
}
