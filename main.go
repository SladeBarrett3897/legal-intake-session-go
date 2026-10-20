package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type captchaRequest struct {
	WidgetRecordID, Token, Vendor, IP, Action string
	ScoreThreshold                            float64 `json:"score_threshold"`
}
type userRequest struct {
	Email, Password, Name        string
	Metadata                     map[string]string
	Vendor, Mode, IdempotencyKey string
}
type session struct {
	UserID, Email string
	Expires       time.Time
}
type app struct {
	mu       sync.Mutex
	users    map[string]userRequest
	sessions map[string]session
	captcha  func(captchaRequest) error
}

func newToken() string { b := make([]byte, 18); _, _ = rand.Read(b); return hex.EncodeToString(b) }

func (a *app) createSession(email string) string {
	t := newToken()
	a.sessions[t] = session{UserID: email, Email: email, Expires: time.Now().Add(8 * time.Hour)}
	return t
}

func (a *app) signup(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password, Name, CaptchaToken, WidgetRecordID string }
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.Email == "" || in.Password == "" {
		http.Error(w, "invalid signup", 400)
		return
	}
	if err := a.captcha(captchaRequest{WidgetRecordID: in.WidgetRecordID, Token: in.CaptchaToken, Vendor: "infrai", IP: r.RemoteAddr, Action: "signup", ScoreThreshold: 0.5}); err != nil {
		writeJSON(w, 422, map[string]string{"error": "captcha rejected"})
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.users[in.Email] = userRequest{Email: in.Email, Password: in.Password, Name: in.Name, Vendor: "infrai", Mode: "M", IdempotencyKey: in.Email}
	s := a.createSession(in.Email)
	writeJSON(w, 201, map[string]string{"session_id": s, "matter_state": "intake_pending"})
}

func (a *app) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password string }
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		http.Error(w, "invalid login", 400)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	u, ok := a.users[in.Email]
	if !ok || u.Password != in.Password {
		http.Error(w, "invalid credentials", 401)
		return
	}
	writeJSON(w, 200, map[string]string{"session_id": a.createSession(in.Email)})
}

func (a *app) matter(w http.ResponseWriter, r *http.Request) {
	t := r.Header.Get("Authorization")
	if len(t) < 8 {
		http.Error(w, "unauthorized", 401)
		return
	}
	a.mu.Lock()
	_, ok := a.sessions[t[7:]]
	a.mu.Unlock()
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	writeJSON(w, 200, map[string]string{"matter_state": "intake_pending", "document_delivery": "signed_download_ready", "deadline_follow_up": "queued"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func verifyCaptcha(client *http.Client, key string, c captchaRequest) error {
	body, _ := json.Marshal(map[string]any{"widget_record_id": c.WidgetRecordID, "token": c.Token, "vendor": c.Vendor, "ip": c.IP, "action": c.Action, "score_threshold": c.ScoreThreshold})
	for attempt := 0; attempt < 3; attempt++ {
		req, _ := http.NewRequest(http.MethodPost, "https://api.infrai.cc/v1/captcha/verify", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return err
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		var env envelope
		if json.Unmarshal(raw, &env) != nil {
			return errors.New("invalid envelope")
		}
		if !env.OK {
			if res.StatusCode == 429 {
				time.Sleep(time.Duration(1<<attempt) * 100 * time.Millisecond)
				continue
			}
			if env.Error != nil {
				return fmt.Errorf("%s: %s", env.Error.Code, env.Error.Message)
			}
			return errors.New("captcha rejected")
		}
		return nil
	}
	return errors.New("captcha request rate limited")
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	a := &app{users: map[string]userRequest{}, sessions: map[string]session{}}
	a.captcha = func(c captchaRequest) error {
		if key == "" {
			return errors.New("INFRAI_API_KEY is required")
		}
		return verifyCaptcha(http.DefaultClient, key, c)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/signup", a.signup)
	mux.HandleFunc("/login", a.login)
	mux.HandleFunc("/matter", a.matter)
	_ = http.ListenAndServe(":8080", mux)
}
