package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/K-H-Tech/auto-tax-gov/internal/config"
	"github.com/K-H-Tech/auto-tax-gov/internal/models"
	"github.com/K-H-Tech/auto-tax-gov/internal/tracker"
)

// Handler holds dependencies for HTTP handlers.
type Handler struct {
	cfg     *config.Config
	tracker *tracker.Tracker
	session *Session
	logger  *slog.Logger
	webDir  string
}

// NewHandler creates a new Handler instance.
func NewHandler(cfg *config.Config, t *tracker.Tracker, logger *slog.Logger, webDir string) *Handler {
	return &Handler{
		cfg:     cfg,
		tracker: t,
		session: NewSession(),
		logger:  logger,
		webDir:  webDir,
	}
}

// ServeHome serves the main HTML page.
func (h *Handler) ServeHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	indexPath := filepath.Join(h.webDir, "index.html")
	content, err := os.ReadFile(indexPath)
	if err != nil {
		h.logger.Error("error loading index.html", "error", err, "path", indexPath)
		http.Error(w, "Error loading page", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}

// HandleStartTracker initiates redirect tracking and returns captcha.
func (h *Handler) HandleStartTracker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	h.logger.Info("starting redirect tracking")

	startURL := h.cfg.GetStartURL()
	steps, cookies, err := h.tracker.TrackRedirects(startURL, nil)
	if err != nil {
		h.logger.Error("error tracking redirects", "error", err)
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	h.logger.Info("tracking complete", "steps", len(steps))

	// Save cookies to session
	h.session.SetCookies(cookies)

	// Save results to files (async)
	go func() {
		if err := h.tracker.SaveResults(steps); err != nil {
			h.logger.Error("error saving results", "error", err)
		}
	}()

	// Get captcha from last step
	var captcha *models.CaptchaData
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].CaptchaData != nil {
			captcha = steps[i].CaptchaData
			h.session.SetCaptcha(captcha)
			break
		}
	}

	if captcha == nil {
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   "No captcha found in redirect chain",
		})
		return
	}

	json.NewEncoder(w).Encode(models.CaptchaResponse{
		Success: true,
		Captcha: &struct {
			Key       string `json:"key"`
			ImageData string `json:"imageData"`
			CSRFToken string `json:"csrfToken"`
		}{
			Key:       captcha.Key,
			ImageData: captcha.ImageData,
			CSRFToken: captcha.CSRFToken,
		},
	})
}

// HandleRefreshCaptcha fetches a new captcha.
func (h *Handler) HandleRefreshCaptcha(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if !h.session.IsActive() {
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   "No active session. Please reload the page.",
		})
		return
	}

	csrfToken := h.session.GetCSRFToken()
	captcha, err := h.tracker.RefreshCaptcha(h.session.GetCookies(), csrfToken)
	if err != nil {
		h.logger.Error("error refreshing captcha", "error", err)
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	h.session.SetCaptcha(captcha)

	json.NewEncoder(w).Encode(models.CaptchaResponse{
		Success: true,
		Captcha: &struct {
			Key       string `json:"key"`
			ImageData string `json:"imageData"`
			CSRFToken string `json:"csrfToken"`
		}{
			Key:       captcha.Key,
			ImageData: captcha.ImageData,
			CSRFToken: captcha.CSRFToken,
		},
	})
}

// HandleSendOTP sends an OTP to the user's mobile.
func (h *Handler) HandleSendOTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var req models.OTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   "Invalid request",
		})
		return
	}

	h.logger.Info("send OTP attempt", "mobile", req.Mobile)

	if !h.session.IsActive() {
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   "No active session. Please reload the page.",
		})
		return
	}

	result, cookies, err := h.tracker.SendOTP(h.session.GetCookies(), req.Mobile, req.CaptchaCode, req.CaptchaKey, req.CSRFToken)
	if err != nil {
		h.logger.Error("send OTP error", "error", err)
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	h.session.SetCookies(cookies)

	json.NewEncoder(w).Encode(models.APIResponse{
		Success: true,
		Message: result.Message,
		Data:    result.Data,
	})
}

// HandleVerifyOTP verifies the OTP code.
func (h *Handler) HandleVerifyOTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var req models.OTPVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   "Invalid request",
		})
		return
	}

	h.logger.Info("verify OTP attempt", "mobile", req.Mobile)

	if !h.session.IsActive() {
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   "No active session. Please reload the page.",
		})
		return
	}

	result, cookies, err := h.tracker.VerifyOTP(h.session.GetCookies(), req.Mobile, req.OTPCode, req.CSRFToken)
	if err != nil {
		h.logger.Error("verify OTP error", "error", err)
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	h.session.SetCookies(cookies)

	json.NewEncoder(w).Encode(models.APIResponse{
		Success: true,
		Message: result.Message,
		Data:    result.Data,
	})
}

// HandleAccessDashboard accesses the tax dashboard.
func (h *Handler) HandleAccessDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	h.logger.Info("attempting to access dashboard")

	if !h.session.IsActive() {
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   "No active session. Please reload the page.",
		})
		return
	}

	result, cookies, err := h.tracker.AccessDashboard(h.session.GetCookies())
	if err != nil {
		h.logger.Error("dashboard access error", "error", err)
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	h.session.SetCookies(cookies)

	json.NewEncoder(w).Encode(models.APIResponse{
		Success: true,
		Message: result.Message,
		Data:    result.Data,
	})
}

// HandleStartTaxFile initiates tax file registration.
func (h *Handler) HandleStartTaxFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	h.logger.Info("starting tax file registration")

	if !h.session.IsActive() {
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   "No active session. Please reload the page.",
		})
		return
	}

	result, cookies, err := h.tracker.StartTaxFileRegistration(h.session.GetCookies())
	if err != nil {
		h.logger.Error("tax file registration error", "error", err)
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	h.session.SetCookies(cookies)

	json.NewEncoder(w).Encode(models.APIResponse{
		Success: true,
		Message: result.Message,
		Data:    result.Data,
	})
}
