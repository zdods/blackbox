package main

import (
	"strings"
	"testing"
)

func TestValidEmail(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"zach@zdods.com", true},
		{"a@b.co", true},
		{"first.last+tag@sub.domain.org", true},
		{"", false},
		{"not-an-email", false},
		{"@no-local.com", false},
		{"no-domain@", false},
		{"two@@at.com", false},
		{"Name <a@b.com>", false}, // display-name form is rejected
		{"spaces in@x.com", false},
	}
	for _, c := range cases {
		if got := validEmail(c.in); got != c.want {
			t.Errorf("validEmail(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestValidEmailLengthCap(t *testing.T) {
	// A local-part that pushes the address past the 254-char cap is rejected
	// even though it is otherwise well-formed.
	long := ""
	for len(long) < maxEmailLen {
		long += "a"
	}
	if validEmail(long + "@example.com") {
		t.Errorf("over-long address should be rejected")
	}
}

func TestPasswordPolicyError(t *testing.T) {
	cases := []struct {
		name   string
		pw     string
		wantOK bool
	}{
		{"too short", "short", false},
		{"min boundary", "12345678", true},
		{"typical", "correct horse battery", true},
		{"max boundary", strings.Repeat("a", maxPasswordLen), true},
		{"too long", strings.Repeat("a", maxPasswordLen+1), false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := passwordPolicyError(tc.pw)
			if (msg == "") != tc.wantOK {
				t.Errorf("passwordPolicyError(%q) = %q, wantOK=%v", tc.pw, msg, tc.wantOK)
			}
		})
	}
}
