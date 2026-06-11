package main

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestDialFailureMessage(t *testing.T) {
	errHandshake := errors.New("websocket: bad handshake")
	cases := []struct {
		name string
		url  string
		resp *http.Response
		want string
	}{
		{
			name: "no response (connection refused)",
			url:  "ws://localhost:8080/ws/daemon",
			resp: nil,
			want: "dial ws://localhost:8080/ws/daemon:",
		},
		{
			name: "http to https redirect suggests wss",
			url:  "ws://example.com/ws/daemon",
			resp: &http.Response{
				StatusCode: http.StatusMovedPermanently,
				Header:     http.Header{"Location": []string{"https://example.com/ws/daemon"}},
			},
			want: "use a wss:// URL",
		},
		{
			name: "400 suggests proxy upgrade headers",
			url:  "wss://example.com/ws/daemon",
			resp: &http.Response{StatusCode: http.StatusBadRequest, Header: http.Header{}},
			want: "Upgrade and Connection headers",
		},
		{
			name: "other status is reported plainly",
			url:  "wss://example.com/ws/daemon",
			resp: &http.Response{StatusCode: http.StatusBadGateway, Header: http.Header{}},
			want: "(HTTP 502)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dialFailureMessage(tc.url, tc.resp, errHandshake)
			if !strings.Contains(got, tc.want) {
				t.Errorf("message %q does not contain %q", got, tc.want)
			}
		})
	}
}
