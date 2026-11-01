package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCookieToken(t *testing.T) {
	a, err := newAuth("family", "1234", strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	other, err := newAuth("family", "1234", strings.Repeat("cd", 32))
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().Unix()
	good := a.token(familyCookie, now+60)
	_, sig, _ := strings.Cut(good, ".")

	tests := []struct {
		name   string
		cookie string // which cookie we check
		value  string
		want   bool
	}{
		{"valid", familyCookie, good, true},
		{"expired", familyCookie, a.token(familyCookie, now-1), false},
		{"expiry changed by hand", familyCookie, "4102444800." + sig, false},
		{"family token used as parent", parentCookie, good, false},
		{"signed with another key", familyCookie, other.token(familyCookie, now+60), false},
		{"not a token at all", familyCookie, "hello", false},
		{"empty", familyCookie, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.AddCookie(&http.Cookie{Name: tt.cookie, Value: tt.value})
			if got := a.valid(r, tt.cookie); got != tt.want {
				t.Errorf("valid() = %v, want %v", got, tt.want)
			}
		})
	}
}
