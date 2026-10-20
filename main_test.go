package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVerifyCaptchaSendsWidgetRecordID(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["widget_record_id"] != "widget-123" || body["token"] != "token-123" {
			t.Fatalf("captcha request fields: %v", body)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	})}
	if err := verifyCaptcha(client, "key", captchaRequest{WidgetRecordID: "widget-123", Token: "token-123"}); err != nil {
		t.Fatal(err)
	}
}

func TestSignupRejectsCaptchaBeforeMatterCreation(t *testing.T) {
	a := &app{users: map[string]userRequest{}, sessions: map[string]session{}, captcha: func(captchaRequest) error { return errCaptcha{} }}
	r := httptest.NewRequest("POST", "/signup", strings.NewReader(`{"email":"a@b.test","password":"pw","captchaToken":"x"}`))
	w := httptest.NewRecorder()
	a.signup(w, r)
	if w.Code != 422 {
		t.Fatalf("status=%d", w.Code)
	}
	if len(a.users) != 0 {
		t.Fatal("user created after rejected captcha")
	}
}

type errCaptcha struct{}

func (errCaptcha) Error() string { return "rejected" }
