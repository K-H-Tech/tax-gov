package server

import (
	"net/http"
	"sync"

	"github.com/K-H-Tech/auto-tax-gov/internal/models"
)

// Session holds the state for a user session.
type Session struct {
	Cookies []*http.Cookie
	Captcha *models.CaptchaData
	mu      sync.RWMutex
}

// NewSession creates a new empty session.
func NewSession() *Session {
	return &Session{
		Cookies: make([]*http.Cookie, 0),
	}
}

// GetCookies returns a copy of the session cookies.
func (s *Session) GetCookies() []*http.Cookie {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cookies := make([]*http.Cookie, len(s.Cookies))
	copy(cookies, s.Cookies)
	return cookies
}

// SetCookies sets the session cookies.
func (s *Session) SetCookies(cookies []*http.Cookie) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Cookies = cookies
}

// AddCookies appends cookies to the session.
func (s *Session) AddCookies(cookies []*http.Cookie) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Cookies = append(s.Cookies, cookies...)
}

// GetCaptcha returns the current captcha data.
func (s *Session) GetCaptcha() *models.CaptchaData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Captcha
}

// SetCaptcha sets the current captcha data.
func (s *Session) SetCaptcha(captcha *models.CaptchaData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Captcha = captcha
}

// GetCSRFToken returns the CSRF token from the current captcha.
func (s *Session) GetCSRFToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Captcha != nil {
		return s.Captcha.CSRFToken
	}
	return ""
}

// IsActive returns true if the session has cookies.
func (s *Session) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Cookies) > 0
}

// Reset clears the session state.
func (s *Session) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Cookies = make([]*http.Cookie, 0)
	s.Captcha = nil
}
