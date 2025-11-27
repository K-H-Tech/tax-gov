package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/K-H-Tech/auto-tax-gov/internal/config"
	"github.com/K-H-Tech/auto-tax-gov/internal/models"
	"github.com/K-H-Tech/auto-tax-gov/internal/service/mytax"
	"github.com/K-H-Tech/auto-tax-gov/internal/session"
)

// Handler holds dependencies for HTTP handlers.
type Handler struct {
	cfg     *config.Config
	mytax   *mytax.Service
	session *session.Session
	logger  *slog.Logger
	webDir  string
}

// NewHandler creates a new Handler instance.
func NewHandler(cfg *config.Config, mytaxSvc *mytax.Service, logger *slog.Logger, webDir string) *Handler {
	return &Handler{
		cfg:     cfg,
		mytax:   mytaxSvc,
		session: session.New(),
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

	h.logger.Info("starting MyTax login flow")

	// Reset session for new login
	h.session.Reset()

	captcha, steps, err := h.mytax.InitiateLogin(h.session)
	if err != nil {
		h.logger.Error("error initiating login", "error", err)
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	h.logger.Info("login initiated", "steps", len(steps))

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

	captcha, err := h.mytax.RefreshCaptcha(h.session)
	if err != nil {
		h.logger.Error("error refreshing captcha", "error", err)
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   err.Error(),
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

	result, err := h.mytax.SendOTP(h.session, req.Mobile, req.CaptchaCode, req.CaptchaKey, req.CSRFToken)
	if err != nil {
		h.logger.Error("send OTP error", "error", err)
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(models.APIResponse{
		Success: result.Success,
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

	result, err := h.mytax.VerifyOTP(h.session, req.Mobile, req.OTPCode, req.CSRFToken)
	if err != nil {
		h.logger.Error("verify OTP error", "error", err)
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(models.APIResponse{
		Success: result.Success,
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

	result, err := h.mytax.AccessDashboard(h.session)
	if err != nil {
		h.logger.Error("dashboard access error", "error", err)
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(models.APIResponse{
		Success: result.Success,
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

	result, err := h.mytax.StartTaxFileRegistration(h.session)
	if err != nil {
		h.logger.Error("tax file registration error", "error", err)
		json.NewEncoder(w).Encode(models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(models.APIResponse{
		Success: result.Success,
		Message: result.Message,
		Data:    result.Data,
	})
}
