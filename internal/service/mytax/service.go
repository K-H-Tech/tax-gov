package mytax

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/K-H-Tech/auto-tax-gov/internal/client"
	"github.com/K-H-Tech/auto-tax-gov/internal/config"
	"github.com/K-H-Tech/auto-tax-gov/internal/models"
	"github.com/K-H-Tech/auto-tax-gov/internal/service/mygovauth"
	"github.com/K-H-Tech/auto-tax-gov/internal/session"
)

// Service handles tax portal operations.
type Service struct {
	cfg    *config.Config
	client *Client
	auth   *mygovauth.Service
	logger *slog.Logger
}

// New creates a new MyTax service instance.
func New(cfg *config.Config, auth *mygovauth.Service, logger *slog.Logger) *Service {
	return &Service{
		cfg:    cfg,
		client: NewClient(cfg),
		auth:   auth,
		logger: logger,
	}
}

// Result represents the result of a tax operation.
type Result struct {
	Success bool
	Message string
	Data    map[string]any
}

// GetAuthURL returns the OAuth2 authorization URL for this service.
func (s *Service) GetAuthURL() string {
	return s.cfg.GetMyTaxAuthURL()
}

// InitiateLogin starts the login flow using mygovauth service.
func (s *Service) InitiateLogin(sess *session.Session) (*models.CaptchaData, []models.RedirectStep, error) {
	authURL := s.GetAuthURL()
	s.logger.Info("initiating MyTax login", "auth_url", authURL)
	return s.auth.InitiateAuth(sess, authURL)
}

// SendOTP sends OTP through mygovauth service.
func (s *Service) SendOTP(sess *session.Session, mobile, captchaCode, captchaKey, csrfToken string) (*mygovauth.AuthResult, error) {
	return s.auth.SendOTP(sess, mobile, captchaCode, captchaKey, csrfToken)
}

// VerifyOTP verifies OTP through mygovauth service.
func (s *Service) VerifyOTP(sess *session.Session, mobile, otpCode, csrfToken string) (*mygovauth.AuthResult, error) {
	return s.auth.VerifyOTP(sess, mobile, otpCode, csrfToken)
}

// RefreshCaptcha refreshes captcha through mygovauth service.
func (s *Service) RefreshCaptcha(sess *session.Session) (*models.CaptchaData, error) {
	return s.auth.RefreshCaptcha(sess)
}

// AccessDashboard accesses the tax dashboard.
func (s *Service) AccessDashboard(sess *session.Session) (*Result, error) {
	if !sess.IsAuthenticated() {
		return nil, fmt.Errorf("session not authenticated")
	}

	req, err := http.NewRequest("GET", s.cfg.Services.MyTax.DashboardURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating dashboard request: %w", err)
	}

	s.client.SetCommonHeaders(req)
	req.Header.Set("Referer", s.client.BaseURL()+"/")
	s.client.AddCookies(req, sess.GetCookies())

	s.logger.Info("accessing dashboard", "url", s.cfg.Services.MyTax.DashboardURL)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error accessing dashboard: %w", err)
	}
	defer resp.Body.Close()

	s.logger.Debug("dashboard response", "status", resp.StatusCode)

	// Save new cookies
	if cookies := resp.Cookies(); len(cookies) > 0 {
		sess.MergeCookies(cookies)
	}

	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("error reading dashboard response: %w", err)
	}

	responseBody := string(body)
	result := &Result{
		Success: true,
		Data:    make(map[string]any),
	}

	if resp.StatusCode == 200 {
		result.Message = "Successfully accessed dashboard"
		result.Data["statusCode"] = resp.StatusCode
		result.Data["url"] = s.cfg.Services.MyTax.DashboardURL
		result.Data["bodyLength"] = len(responseBody)

		// Check if we got actual dashboard content
		if strings.Contains(responseBody, "Dashboard") || strings.Contains(responseBody, "داشبورد") {
			result.Message = "Successfully accessed dashboard - Dashboard content loaded"
		}

		// Save response body preview
		bodyPreview := responseBody
		if len(bodyPreview) > 1000 {
			bodyPreview = bodyPreview[:1000]
		}
		result.Data["bodyPreview"] = bodyPreview
	} else if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		redirectLocation := resp.Header.Get("Location")
		result.Message = fmt.Sprintf("Dashboard redirected to: %s", redirectLocation)
		result.Data["redirectLocation"] = redirectLocation
		result.Data["statusCode"] = resp.StatusCode
	} else {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return result, nil
}

// StartTaxFileRegistration initiates tax file registration.
func (s *Service) StartTaxFileRegistration(sess *session.Session, postalCode, businessName, regType string) (*Result, error) {
	if !sess.IsAuthenticated() {
		return nil, fmt.Errorf("session not authenticated")
	}

	payload := fmt.Sprintf("NewRegistrationType=%s&NewRegistrationPostalCode=%s&NewRegistrationBusinessName=%s",
		url.QueryEscape(regType),
		url.QueryEscape(postalCode),
		url.QueryEscape(businessName))

	req, err := http.NewRequest("POST", s.cfg.Services.MyTax.RegistrationURL, strings.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("error creating tax registration request: %w", err)
	}

	s.client.SetAPIHeaders(req, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("Origin", s.client.Origin())
	req.Header.Set("Referer", s.client.BaseURL()+"/Page/ShowDocuments/")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	s.client.AddCookies(req, sess.GetCookies())

	s.logger.Info("starting tax file registration", "url", s.cfg.Services.MyTax.RegistrationURL)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error starting tax registration: %w", err)
	}
	defer resp.Body.Close()

	s.logger.Debug("tax registration response", "status", resp.StatusCode)

	// Save new cookies
	if cookies := resp.Cookies(); len(cookies) > 0 {
		sess.MergeCookies(cookies)
	}

	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("error reading tax registration response: %w", err)
	}

	result := &Result{
		Success: true,
		Data:    make(map[string]any),
	}

	if resp.StatusCode == 200 {
		// Try to parse JSON response
		var jsonResponse map[string]any
		if err := json.Unmarshal(body, &jsonResponse); err == nil {
			if isSuccess, ok := jsonResponse["isSuccess"].(bool); ok && !isSuccess {
				errorMsg := "خطا در شروع پرونده مالیاتی"
				if msg, ok := jsonResponse["msg"].(string); ok && msg != "" {
					errorMsg = msg
				}
				return nil, fmt.Errorf("%s", errorMsg)
			}

			result.Message = "شروع پرونده مالیاتی موفقیت‌آمیز بود"
			result.Data["statusCode"] = resp.StatusCode
			result.Data["url"] = s.cfg.Services.MyTax.RegistrationURL
			result.Data["response"] = jsonResponse

			if msg, ok := jsonResponse["msg"].(string); ok && msg != "" {
				result.Message = msg
			}
		} else {
			result.Message = "شروع پرونده مالیاتی موفقیت‌آمیز بود"
			result.Data["statusCode"] = resp.StatusCode
			result.Data["url"] = s.cfg.Services.MyTax.RegistrationURL
			result.Data["bodyLength"] = len(body)
		}
	} else if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		redirectLocation := resp.Header.Get("Location")
		result.Message = fmt.Sprintf("Redirected to: %s", redirectLocation)
		result.Data["redirectLocation"] = redirectLocation
		result.Data["statusCode"] = resp.StatusCode
	} else {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return result, nil
}

// SubmitBasicInfo submits the basic information form (Step 2).
func (s *Service) SubmitBasicInfo(sess *session.Session, req *models.BasicInfoRequest) (*Result, error) {
	if !sess.IsAuthenticated() {
		return nil, fmt.Errorf("session not authenticated")
	}

	// Validate with config options
	validation := ValidateBasicInfo(req, &s.cfg.FormOptions)
	if !validation.Valid {
		return nil, fmt.Errorf("%s", validation.FirstError())
	}

	// Build form payload
	payload := url.Values{}
	payload.Set("RegistrationReason", req.RegistrationReason)
	payload.Set("ActivityType", req.ActivityType)
	payload.Set("StartDate", req.StartDate)
	payload.Set("UnitTitle", req.UnitTitle)
	payload.Set("EightCategoryJob", req.EightCategoryJob)
	payload.Set("IndividualJob", req.IndividualJob)
	payload.Set("ProfessionalGuild", req.ProfessionalGuild)
	payload.Set("ProfessionalAssembly", req.ProfessionalAssembly)
	payload.Set("GuildUnion", req.GuildUnion)
	payload.Set("BusinessLicense", req.BusinessLicense)
	payload.Set("OwnershipType", req.OwnershipType)
	if req.Website != "" {
		payload.Set("Website", req.Website)
	}

	httpReq, err := http.NewRequest("POST", s.cfg.Services.MyTax.BasicInfoURL, strings.NewReader(payload.Encode()))
	if err != nil {
		return nil, fmt.Errorf("error creating basic info request: %w", err)
	}

	s.client.SetAPIHeaders(httpReq, "")
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	httpReq.Header.Set("Origin", s.client.Origin())
	httpReq.Header.Set("Referer", s.client.BaseURL()+"/Page/NewRegistration/")
	httpReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	s.client.AddCookies(httpReq, sess.GetCookies())

	s.logger.Info("submitting basic info", "url", s.cfg.Services.MyTax.BasicInfoURL)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error submitting basic info: %w", err)
	}
	defer resp.Body.Close()

	s.logger.Debug("basic info response", "status", resp.StatusCode)

	// Save new cookies
	if cookies := resp.Cookies(); len(cookies) > 0 {
		sess.MergeCookies(cookies)
	}

	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("error reading basic info response: %w", err)
	}

	result := &Result{
		Success: true,
		Data:    make(map[string]any),
	}

	if resp.StatusCode == 200 {
		// Try to parse JSON response
		var jsonResponse map[string]any
		if err := json.Unmarshal(body, &jsonResponse); err == nil {
			if isSuccess, ok := jsonResponse["isSuccess"].(bool); ok && !isSuccess {
				errorMsg := "خطا در ثبت اطلاعات پایه"
				if msg, ok := jsonResponse["msg"].(string); ok && msg != "" {
					errorMsg = msg
				}
				return nil, fmt.Errorf("%s", errorMsg)
			}

			result.Message = "اطلاعات پایه با موفقیت ثبت شد"
			result.Data["statusCode"] = resp.StatusCode
			result.Data["url"] = s.cfg.Services.MyTax.BasicInfoURL
			result.Data["response"] = jsonResponse

			if msg, ok := jsonResponse["msg"].(string); ok && msg != "" {
				result.Message = msg
			}
		} else {
			result.Message = "اطلاعات پایه با موفقیت ثبت شد"
			result.Data["statusCode"] = resp.StatusCode
			result.Data["url"] = s.cfg.Services.MyTax.BasicInfoURL
			result.Data["bodyLength"] = len(body)
		}
	} else if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		redirectLocation := resp.Header.Get("Location")
		result.Message = fmt.Sprintf("Redirected to: %s", redirectLocation)
		result.Data["redirectLocation"] = redirectLocation
		result.Data["statusCode"] = resp.StatusCode
	} else {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return result, nil
}

// GetFormOptions returns the form dropdown options from config.
func (s *Service) GetFormOptions() *config.FormOptionsConfig {
	return &s.cfg.FormOptions
}

// GetConfig returns the service configuration.
func (s *Service) GetConfig() *config.MyTaxConfig {
	return &s.cfg.Services.MyTax
}

// GetClient returns the service client.
func (s *Service) GetClient() *Client {
	return s.client
}
