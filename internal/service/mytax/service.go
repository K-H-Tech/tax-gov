package mytax

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/K-H-Tech/auto-tax-gov/internal/client"
	"github.com/K-H-Tech/auto-tax-gov/internal/config"
	"github.com/K-H-Tech/auto-tax-gov/internal/models"
	"github.com/K-H-Tech/auto-tax-gov/internal/service/mygovauth"
	"github.com/K-H-Tech/auto-tax-gov/internal/session"
)

// uuidPattern matches UUID in URLs like /Page/ShowDocumentsDetails/{uuid}
var uuidPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

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

				// Return Result with Success=false so we can include registration ID for deletion
				errorResult := &Result{
					Success: false,
					Message: errorMsg,
					Data:    make(map[string]any),
				}
				errorResult.Data["error"] = errorMsg

				// Try to extract registration ID from the response body (for delete functionality)
				if match := uuidPattern.FindString(string(body)); match != "" {
					errorResult.Data["registrationId"] = match
					s.logger.Info("extracted registration ID from error response", "registrationId", match)
				}

				return errorResult, nil
			}

			result.Message = "شروع پرونده مالیاتی موفقیت‌آمیز بود"
			result.Data["statusCode"] = resp.StatusCode
			result.Data["url"] = s.cfg.Services.MyTax.RegistrationURL
			result.Data["response"] = jsonResponse

			if msg, ok := jsonResponse["msg"].(string); ok && msg != "" {
				result.Message = msg
			}

			// Try to extract registration ID from JSON response
			if regID, ok := jsonResponse["registrationId"].(string); ok && regID != "" {
				sess.SetRegistrationID(regID)
				result.Data["registrationId"] = regID
				s.logger.Info("extracted registration ID from JSON", "registrationId", regID)
			} else if data, ok := jsonResponse["data"].(map[string]any); ok {
				if regID, ok := data["registrationId"].(string); ok && regID != "" {
					sess.SetRegistrationID(regID)
					result.Data["registrationId"] = regID
					s.logger.Info("extracted registration ID from data", "registrationId", regID)
				}
			}

			// If no ID found in JSON, try to find UUID in the response body
			if sess.GetRegistrationID() == "" {
				if match := uuidPattern.FindString(string(body)); match != "" {
					sess.SetRegistrationID(match)
					result.Data["registrationId"] = match
					s.logger.Info("extracted registration ID from body", "registrationId", match)
				}
			}
		} else {
			result.Message = "شروع پرونده مالیاتی موفقیت‌آمیز بود"
			result.Data["statusCode"] = resp.StatusCode
			result.Data["url"] = s.cfg.Services.MyTax.RegistrationURL
			result.Data["bodyLength"] = len(body)

			// Try to find UUID in the response body
			if match := uuidPattern.FindString(string(body)); match != "" {
				sess.SetRegistrationID(match)
				result.Data["registrationId"] = match
				s.logger.Info("extracted registration ID from body", "registrationId", match)
			}
		}
	} else if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		redirectLocation := resp.Header.Get("Location")
		result.Message = fmt.Sprintf("Redirected to: %s", redirectLocation)
		result.Data["redirectLocation"] = redirectLocation
		result.Data["statusCode"] = resp.StatusCode

		// Extract registration ID from redirect URL
		if match := uuidPattern.FindString(redirectLocation); match != "" {
			sess.SetRegistrationID(match)
			result.Data["registrationId"] = match
			s.logger.Info("extracted registration ID from redirect", "registrationId", match)
		}
	} else {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return result, nil
}

// GetSSOToken calls my.tax.gov.ir/Page/SSODoc/{registrationId} to get the SSO auth token.
// The browser calls this endpoint which returns a redirect to register.tax.gov.ir
// with a DIFFERENT UUID that is required for TokenLoginProcessWithSignout.
func (s *Service) GetSSOToken(sess *session.Session, registrationID string) (string, error) {
	ssoDocURL := strings.TrimSuffix(s.cfg.Services.MyTax.SSODocURL, "/") + "/" + registrationID

	s.logger.Info("fetching SSO token", "url", ssoDocURL)

	req, err := http.NewRequest("GET", ssoDocURL, nil)
	if err != nil {
		return "", fmt.Errorf("error creating SSODoc request: %w", err)
	}

	s.client.SetNavigationHeaders(req, s.cfg.Services.MyTax.BaseURL+"/")
	s.client.AddCookies(req, sess.GetCookies())

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error calling SSODoc: %w", err)
	}
	defer resp.Body.Close()

	// Save cookies
	if cookies := resp.Cookies(); len(cookies) > 0 {
		sess.MergeCookies(cookies)
	}

	// The response should be a redirect to register.tax.gov.ir with the SSO token UUID
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		s.logger.Info("SSODoc redirect", "location", location)

		// Extract UUID from the redirect URL
		// Example: https://register.tax.gov.ir/Pages/Login/TokenLoginProcessWithSignout/55fb28fa-e6d0-484e-ae5a-f323cdf7117f
		if match := uuidPattern.FindString(location); match != "" {
			s.logger.Info("extracted SSO token UUID", "ssoTokenUUID", match)
			return match, nil
		}

		return "", fmt.Errorf("no UUID found in SSODoc redirect: %s", location)
	}

	// If 200, check response body for UUID
	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return "", fmt.Errorf("error reading SSODoc response: %w", err)
	}

	if match := uuidPattern.FindString(string(body)); match != "" {
		s.logger.Info("extracted SSO token UUID from body", "ssoTokenUUID", match)
		return match, nil
	}

	return "", fmt.Errorf("failed to extract SSO token from SSODoc response (status: %d)", resp.StatusCode)
}

// AuthenticateToRegisterTax performs cross-domain authentication to register.tax.gov.ir.
// This follows the redirect chain starting from my.tax.gov.ir:
//  1. GET my.tax.gov.ir/Page/SSODoc/{uuid} → Gets SSO token UUID
//  2. GET register.tax.gov.ir/Pages/Login/TokenLoginProcessWithSignout/{ssoTokenUUID}
//  3. Follow redirect chain through register.tax.gov.ir authentication
//  4. End at register.tax.gov.ir/Pages/Preaction/HomePage
func (s *Service) AuthenticateToRegisterTax(sess *session.Session) error {
	if !sess.IsAuthenticated() {
		return fmt.Errorf("session not authenticated")
	}

	regID := sess.GetRegistrationID()
	if regID == "" {
		return fmt.Errorf("registration ID not found in session")
	}

	// Log cookies before starting (INFO level for visibility)
	cookies := sess.GetCookies()
	cookieNames := make([]string, 0, len(cookies))
	hasTaxpayerToken := false
	var taxpayerTokenDomain string
	for _, c := range cookies {
		cookieNames = append(cookieNames, c.Name)
		if c.Name == "TaxpayerToken" {
			hasTaxpayerToken = true
			taxpayerTokenDomain = c.Domain
		}
	}
	s.logger.Info("cookies before register.tax.gov.ir auth",
		"count", len(cookies),
		"names", cookieNames,
		"hasTaxpayerToken", hasTaxpayerToken,
		"taxpayerTokenDomain", taxpayerTokenDomain)

	if !hasTaxpayerToken {
		s.logger.Warn("TaxpayerToken cookie not found - authentication will likely fail")
	}

	// First call SSODoc to get the SSO auth token UUID
	// The browser calls my.tax.gov.ir/Page/SSODoc/{registrationId} which returns a DIFFERENT UUID
	// that must be used for TokenLoginProcessWithSignout
	ssoTokenUUID, err := s.GetSSOToken(sess, regID)
	if err != nil {
		return fmt.Errorf("error getting SSO token: %w", err)
	}

	s.logger.Info("got SSO token, using for TokenLoginProcessWithSignout",
		"registrationUUID", regID,
		"ssoTokenUUID", ssoTokenUUID)

	// Use ssoTokenUUID instead of regID for TokenLoginProcessWithSignout
	startURL := strings.TrimSuffix(s.cfg.Services.RegisterTax.TokenLoginURL, "/") + "/" + ssoTokenUUID

	s.logger.Info("starting cross-domain auth to register.tax.gov.ir", "url", startURL)

	currentURL := startURL
	maxRedirects := 10
	referer := s.cfg.Services.MyTax.BaseURL + "/"

	for i := 0; i < maxRedirects; i++ {
		req, err := http.NewRequest("GET", currentURL, nil)
		if err != nil {
			return fmt.Errorf("error creating request: %w", err)
		}

		// Use navigation headers with Sec-Fetch-* for cross-domain auth
		s.client.SetNavigationHeaders(req, referer)
		s.client.AddCookies(req, sess.GetCookies())

		// Log the actual Cookie header being sent (first step only)
		if i == 0 {
			s.logger.Info("Cookie header being sent to register.tax.gov.ir",
				"cookieHeader", req.Header.Get("Cookie"))
		}

		resp, err := s.client.Do(req)
		if err != nil {
			return fmt.Errorf("error during redirect chain: %w", err)
		}

		// Save cookies from response
		if respCookies := resp.Cookies(); len(respCookies) > 0 {
			sess.MergeCookies(respCookies)
			s.logger.Debug("saved cookies from register.tax.gov.ir",
				"step", i+1,
				"url", currentURL,
				"cookieCount", len(respCookies))
		}

		body, _ := client.ReadResponseBody(resp)
		resp.Body.Close()

		// Log redirect step at INFO level for debugging
		redirectLocation := resp.Header.Get("Location")
		s.logger.Info("register.tax.gov.ir redirect step",
			"step", i+1,
			"url", currentURL,
			"status", resp.StatusCode,
			"redirectTo", redirectLocation,
			"bodyLen", len(body))

		if resp.StatusCode == 200 {
			// Validate we reached the correct destination on register.tax.gov.ir
			if strings.Contains(currentURL, "/Pages/Login") && !strings.Contains(currentURL, "TokenLogin") {
				s.logger.Error("authentication failed - redirected to login page",
					"finalURL", currentURL,
					"steps", i+1)
				return fmt.Errorf("authentication to register.tax.gov.ir failed - redirected to login page")
			}

			// Check if we ended up on register.tax.gov.ir (success)
			if strings.Contains(currentURL, "register.tax.gov.ir") {
				if strings.Contains(currentURL, "/Pages/Preaction/") ||
					strings.Contains(currentURL, "/Pages/prelogin/") {
					s.logger.Info("successfully authenticated to register.tax.gov.ir",
						"finalURL", currentURL,
						"steps", i+1)
					return nil
				}
			}

			// If we're still on my.tax.gov.ir with 200, that might be OK too
			if strings.Contains(currentURL, "my.tax.gov.ir") {
				s.logger.Info("ended at my.tax.gov.ir (may need to follow additional redirects)",
					"finalURL", currentURL,
					"steps", i+1)
				return nil
			}

			s.logger.Warn("unexpected final destination",
				"finalURL", currentURL,
				"steps", i+1)
			return nil
		}

		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			// Follow redirect
			location := resp.Header.Get("Location")
			if location == "" {
				return fmt.Errorf("redirect without Location header at step %d", i+1)
			}

			// Update referer to current URL before moving to next
			referer = currentURL

			// Handle relative URLs - resolve against current URL's host
			if strings.HasPrefix(location, "/") {
				// Parse current URL to get scheme and host
				parsedURL, err := url.Parse(currentURL)
				if err == nil {
					currentURL = parsedURL.Scheme + "://" + parsedURL.Host + location
				} else {
					// Fallback to register.tax.gov.ir
					currentURL = s.cfg.Services.RegisterTax.BaseURL + location
				}
			} else {
				currentURL = location
			}

			s.logger.Debug("following redirect", "to", currentURL)
			continue
		}

		return fmt.Errorf("unexpected status %d at step %d", resp.StatusCode, i+1)
	}

	return fmt.Errorf("too many redirects (max %d)", maxRedirects)
}

// SubmitBasicInfo submits the basic information form (Step 2) to register.tax.gov.ir.
// This uses a three-step process:
// 1. Authenticate to register.tax.gov.ir via cross-domain redirect chain
// 2. GET the form page to extract __VIEWSTATE and __EVENTVALIDATION
// 3. POST the form with ASP.NET hidden fields + form data
func (s *Service) SubmitBasicInfo(sess *session.Session, req *models.BasicInfoRequest) (*Result, error) {
	if !sess.IsAuthenticated() {
		return nil, fmt.Errorf("session not authenticated")
	}

	// Get registration ID from session
	regID := sess.GetRegistrationID()
	if regID == "" {
		return nil, fmt.Errorf("شناسه ثبت‌نام یافت نشد. لطفاً ابتدا پرونده مالیاتی را شروع کنید")
	}

	// Validate with config options
	validation := ValidateBasicInfo(req, &s.cfg.FormOptions)
	if !validation.Valid {
		return nil, fmt.Errorf("%s", validation.FirstError())
	}

	// Step 1: Authenticate to register.tax.gov.ir via cross-domain redirect chain
	s.logger.Info("authenticating to register.tax.gov.ir")
	if err := s.AuthenticateToRegisterTax(sess); err != nil {
		return nil, fmt.Errorf("error authenticating to register.tax.gov.ir: %w", err)
	}

	// Form URL on register.tax.gov.ir
	formURL := s.cfg.Services.RegisterTax.PublicDataURL

	// Step 2: GET the form page to extract ASP.NET state
	s.logger.Info("fetching basic info form page", "url", formURL)

	getReq, err := http.NewRequest("GET", formURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating GET request: %w", err)
	}

	// Use navigation headers for fetching the form page on register.tax.gov.ir
	s.client.SetNavigationHeaders(getReq, s.cfg.Services.RegisterTax.BaseURL+"/")
	s.client.AddCookies(getReq, sess.GetCookies())

	getResp, err := s.client.Do(getReq)
	if err != nil {
		return nil, fmt.Errorf("error fetching form page: %w", err)
	}
	defer getResp.Body.Close()

	// Save cookies from GET response
	if cookies := getResp.Cookies(); len(cookies) > 0 {
		sess.MergeCookies(cookies)
	}

	if getResp.StatusCode != 200 {
		// Handle 302 redirect specifically - indicates auth failure
		if getResp.StatusCode == 302 {
			location := getResp.Header.Get("Location")
			if strings.Contains(location, "/Login") || strings.Contains(location, "/login") {
				return nil, fmt.Errorf("not authenticated to register.tax.gov.ir - redirected to login page (session may have expired)")
			}
			return nil, fmt.Errorf("form page redirected to %s - session may have expired", location)
		}
		return nil, fmt.Errorf("form page returned status %d", getResp.StatusCode)
	}

	getBody, err := client.ReadResponseBody(getResp)
	if err != nil {
		return nil, fmt.Errorf("error reading form page: %w", err)
	}

	// Extract ASP.NET form data
	aspnetData, err := ExtractASPNetFormData(string(getBody))
	if err != nil {
		return nil, fmt.Errorf("error extracting ASP.NET form data: %w", err)
	}

	s.logger.Debug("extracted ASP.NET form data",
		"viewStateLen", len(aspnetData.ViewState),
		"eventValidationLen", len(aspnetData.EventValidation))

	// Step 3: Build and submit form with ASP.NET fields + our data
	formFields := map[string]string{
		"DDLNewRegistrationCause":   req.RegistrationReason,
		"DDLIsTejari":               req.ActivityType,
		"TextBoxFinantialStartDate": req.StartDate,
		"TextBoxPDName":             req.UnitTitle,
		"DDLGroupOneTypes":          req.EightCategoryJob,
		"DDLGroupTwoTypes":          req.IndividualJob,
		"DDLPDNewLegalGroup":        req.ProfessionalGuild,
		"DDLPDLegalType":            req.GuildUnion,
		"DDLHasJobLicence":          req.BusinessLicense,
		"DDLPDOwnership":            req.OwnershipType,
	}

	// Add optional fields
	if req.Website != "" {
		formFields["TextBoxWebsite"] = req.Website
	}
	if req.Mobile != "" {
		formFields["TextBoxMobile"] = req.Mobile
	}
	if req.Email != "" {
		formFields["TextBoxEmail"] = req.Email
	}

	payload := BuildASPNetPayload(aspnetData, formFields)

	s.logger.Info("submitting basic info form", "url", formURL, "registrationId", regID)

	postReq, err := http.NewRequest("POST", formURL, strings.NewReader(payload.Encode()))
	if err != nil {
		return nil, fmt.Errorf("error creating POST request: %w", err)
	}

	s.client.SetCommonHeaders(postReq)
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.Header.Set("Origin", s.cfg.Services.RegisterTax.BaseURL)
	postReq.Header.Set("Referer", formURL)
	s.client.AddCookies(postReq, sess.GetCookies())

	postResp, err := s.client.Do(postReq)
	if err != nil {
		return nil, fmt.Errorf("error submitting basic info: %w", err)
	}
	defer postResp.Body.Close()

	s.logger.Debug("basic info response", "status", postResp.StatusCode)

	// Save new cookies
	if cookies := postResp.Cookies(); len(cookies) > 0 {
		sess.MergeCookies(cookies)
	}

	body, err := client.ReadResponseBody(postResp)
	if err != nil {
		return nil, fmt.Errorf("error reading basic info response: %w", err)
	}

	result := &Result{
		Success: true,
		Data:    make(map[string]any),
	}

	responseBody := string(body)

	if postResp.StatusCode == 200 {
		// Check for success indicators in HTML response
		if strings.Contains(responseBody, "موفقیت") ||
			strings.Contains(responseBody, "success") ||
			strings.Contains(responseBody, "ثبت شد") {
			result.Message = "اطلاعات پایه با موفقیت ثبت شد"
			result.Data["statusCode"] = postResp.StatusCode
			result.Data["url"] = formURL
			result.Data["registrationId"] = regID
		} else if strings.Contains(responseBody, "خطا") ||
			strings.Contains(responseBody, "error") {
			// Check for error messages
			result.Success = false
			result.Message = "خطا در ثبت اطلاعات پایه"
			result.Data["statusCode"] = postResp.StatusCode
			result.Data["bodyPreview"] = truncateString(responseBody, 500)
		} else {
			// Assume success if no error indicators
			result.Message = "اطلاعات پایه ارسال شد"
			result.Data["statusCode"] = postResp.StatusCode
			result.Data["url"] = formURL
			result.Data["registrationId"] = regID
			result.Data["bodyLength"] = len(body)
		}
	} else if postResp.StatusCode >= 300 && postResp.StatusCode < 400 {
		redirectLocation := postResp.Header.Get("Location")
		result.Message = fmt.Sprintf("Redirected to: %s", redirectLocation)
		result.Data["redirectLocation"] = redirectLocation
		result.Data["statusCode"] = postResp.StatusCode
	} else {
		return nil, fmt.Errorf("unexpected status code: %d", postResp.StatusCode)
	}

	return result, nil
}

// truncateString truncates a string to maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
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

// SubmitPartners submits the partners form (Step 3 - شرکا و اعضا).
func (s *Service) SubmitPartners(sess *session.Session, req *models.PartnersRequest) (*Result, error) {
	if !sess.IsAuthenticated() {
		return nil, fmt.Errorf("session not authenticated")
	}

	// Validate partners
	if len(req.Partners) == 0 {
		return nil, fmt.Errorf("حداقل یک شریک باید وارد شود")
	}

	totalShare := 0
	for _, p := range req.Partners {
		if p.NationalID == "" {
			return nil, fmt.Errorf("کد ملی شریک الزامی است")
		}
		if len(p.NationalID) != 10 {
			return nil, fmt.Errorf("کد ملی باید ۱۰ رقم باشد")
		}
		if p.FullName == "" {
			return nil, fmt.Errorf("نام و نام خانوادگی شریک الزامی است")
		}
		if p.SharePercent <= 0 || p.SharePercent > 100 {
			return nil, fmt.Errorf("درصد سهم باید بین ۱ تا ۱۰۰ باشد")
		}
		totalShare += p.SharePercent
	}

	if totalShare != 100 {
		return nil, fmt.Errorf("مجموع درصد سهم شرکا باید ۱۰۰ باشد (فعلی: %d)", totalShare)
	}

	// Build JSON payload
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshaling partners request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", s.cfg.Services.MyTax.PartnersURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("error creating partners request: %w", err)
	}

	s.client.SetAPIHeaders(httpReq, "")
	httpReq.Header.Set("Content-Type", "application/json; charset=UTF-8")
	httpReq.Header.Set("Origin", s.client.Origin())
	httpReq.Header.Set("Referer", s.client.BaseURL()+"/Page/NewRegistration/")
	httpReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	s.client.AddCookies(httpReq, sess.GetCookies())

	s.logger.Info("submitting partners", "url", s.cfg.Services.MyTax.PartnersURL, "count", len(req.Partners))

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error submitting partners: %w", err)
	}
	defer resp.Body.Close()

	s.logger.Debug("partners response", "status", resp.StatusCode)

	// Save new cookies
	if cookies := resp.Cookies(); len(cookies) > 0 {
		sess.MergeCookies(cookies)
	}

	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("error reading partners response: %w", err)
	}

	result := &Result{
		Success: true,
		Data:    make(map[string]any),
	}

	if resp.StatusCode == 200 {
		var jsonResponse map[string]any
		if err := json.Unmarshal(body, &jsonResponse); err == nil {
			if isSuccess, ok := jsonResponse["isSuccess"].(bool); ok && !isSuccess {
				errorMsg := "خطا در ثبت اطلاعات شرکا"
				if msg, ok := jsonResponse["msg"].(string); ok && msg != "" {
					errorMsg = msg
				}
				return nil, fmt.Errorf("%s", errorMsg)
			}

			result.Message = "اطلاعات شرکا با موفقیت ثبت شد"
			result.Data["statusCode"] = resp.StatusCode
			result.Data["response"] = jsonResponse

			if msg, ok := jsonResponse["msg"].(string); ok && msg != "" {
				result.Message = msg
			}
		} else {
			result.Message = "اطلاعات شرکا با موفقیت ثبت شد"
			result.Data["statusCode"] = resp.StatusCode
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

// SubmitBankAccounts submits the bank accounts form (Step 4 - حساب‌های بانکی).
func (s *Service) SubmitBankAccounts(sess *session.Session, req *models.BankAccountsRequest) (*Result, error) {
	if !sess.IsAuthenticated() {
		return nil, fmt.Errorf("session not authenticated")
	}

	// Validate bank accounts
	if len(req.Accounts) == 0 {
		return nil, fmt.Errorf("حداقل یک حساب بانکی باید وارد شود")
	}

	for i, acc := range req.Accounts {
		// Validate IBAN (24 digits without IR prefix)
		if acc.IBAN == "" {
			return nil, fmt.Errorf("شماره شبا حساب %d الزامی است", i+1)
		}
		// Remove IR prefix if present
		iban := strings.TrimPrefix(strings.ToUpper(acc.IBAN), "IR")
		if len(iban) != 24 {
			return nil, fmt.Errorf("شماره شبا باید ۲۴ رقم باشد (حساب %d)", i+1)
		}
		// Check if all characters are digits
		for _, c := range iban {
			if c < '0' || c > '9' {
				return nil, fmt.Errorf("شماره شبا باید فقط شامل اعداد باشد (حساب %d)", i+1)
			}
		}

		if acc.StartDate == "" {
			return nil, fmt.Errorf("تاریخ شروع استفاده از حساب %d الزامی است", i+1)
		}
	}

	// Build JSON payload
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshaling bank accounts request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", s.cfg.Services.MyTax.AddAccountURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("error creating bank accounts request: %w", err)
	}

	s.client.SetAPIHeaders(httpReq, "")
	httpReq.Header.Set("Content-Type", "application/json; charset=UTF-8")
	httpReq.Header.Set("Origin", s.client.Origin())
	httpReq.Header.Set("Referer", s.client.BaseURL()+"/Page/Accounts/")
	httpReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	s.client.AddCookies(httpReq, sess.GetCookies())

	s.logger.Info("submitting bank accounts", "url", s.cfg.Services.MyTax.AddAccountURL, "count", len(req.Accounts))

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error submitting bank accounts: %w", err)
	}
	defer resp.Body.Close()

	s.logger.Debug("bank accounts response", "status", resp.StatusCode)

	// Save new cookies
	if cookies := resp.Cookies(); len(cookies) > 0 {
		sess.MergeCookies(cookies)
	}

	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("error reading bank accounts response: %w", err)
	}

	result := &Result{
		Success: true,
		Data:    make(map[string]any),
	}

	if resp.StatusCode == 200 {
		var jsonResponse map[string]any
		if err := json.Unmarshal(body, &jsonResponse); err == nil {
			if isSuccess, ok := jsonResponse["isSuccess"].(bool); ok && !isSuccess {
				errorMsg := "خطا در ثبت حساب‌های بانکی"
				if msg, ok := jsonResponse["msg"].(string); ok && msg != "" {
					errorMsg = msg
				}
				return nil, fmt.Errorf("%s", errorMsg)
			}

			result.Message = "حساب‌های بانکی با موفقیت ثبت شدند"
			result.Data["statusCode"] = resp.StatusCode
			result.Data["response"] = jsonResponse

			if msg, ok := jsonResponse["msg"].(string); ok && msg != "" {
				result.Message = msg
			}
		} else {
			result.Message = "حساب‌های بانکی با موفقیت ثبت شدند"
			result.Data["statusCode"] = resp.StatusCode
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

// DeleteRegistration deletes an incomplete registration.
func (s *Service) DeleteRegistration(sess *session.Session, registrationID string) (*Result, error) {
	if !sess.IsAuthenticated() {
		return nil, fmt.Errorf("session not authenticated")
	}

	if registrationID == "" {
		return nil, fmt.Errorf("شناسه ثبت‌نام الزامی است")
	}

	// Build URL with registration ID
	deleteURL := strings.TrimSuffix(s.cfg.Services.MyTax.DeleteRegURL, "/") + "/" + registrationID

	// Create GET request (tax portal uses GET for deletion)
	req, err := http.NewRequest("GET", deleteURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating delete request: %w", err)
	}

	// Set headers matching the tax portal's expected format
	s.client.SetAPIHeaders(req, "")
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", s.client.BaseURL()+"/Page/ShowDocumentsDetails/"+registrationID)
	s.client.AddCookies(req, sess.GetCookies())

	s.logger.Info("deleting registration", "url", deleteURL, "registrationId", registrationID)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error deleting registration: %w", err)
	}
	defer resp.Body.Close()

	s.logger.Debug("delete registration response", "status", resp.StatusCode)

	// Save new cookies
	if cookies := resp.Cookies(); len(cookies) > 0 {
		sess.MergeCookies(cookies)
	}

	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("error reading delete response: %w", err)
	}

	result := &Result{
		Success: true,
		Data:    make(map[string]any),
	}

	if resp.StatusCode == 200 {
		var jsonResponse map[string]any
		if err := json.Unmarshal(body, &jsonResponse); err == nil {
			if isSuccess, ok := jsonResponse["isSuccess"].(bool); ok && !isSuccess {
				errorMsg := "خطا در حذف درخواست ثبت‌نام"
				if msg, ok := jsonResponse["msg"].(string); ok && msg != "" {
					errorMsg = msg
				}
				return nil, fmt.Errorf("%s", errorMsg)
			}

			result.Message = "درخواست ثبت‌نام با موفقیت حذف شد"
			result.Data["statusCode"] = resp.StatusCode
			result.Data["response"] = jsonResponse

			if msg, ok := jsonResponse["msg"].(string); ok && msg != "" {
				result.Message = msg
			}

			// Clear registration ID from session on success
			sess.SetRegistrationID("")
		} else {
			// Non-JSON response, check if it looks successful
			bodyStr := string(body)
			if strings.Contains(bodyStr, "success") || strings.Contains(bodyStr, "موفق") {
				result.Message = "درخواست ثبت‌نام با موفقیت حذف شد"
				sess.SetRegistrationID("")
			} else {
				result.Message = "درخواست ثبت‌نام حذف شد"
				sess.SetRegistrationID("")
			}
			result.Data["statusCode"] = resp.StatusCode
			result.Data["bodyLength"] = len(body)
		}
	} else if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		redirectLocation := resp.Header.Get("Location")
		result.Message = fmt.Sprintf("Redirected to: %s", redirectLocation)
		result.Data["redirectLocation"] = redirectLocation
		result.Data["statusCode"] = resp.StatusCode
		// Clear registration ID on redirect (likely success)
		sess.SetRegistrationID("")
	} else {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return result, nil
}

// incompleteStatusPattern matches incomplete registration statuses (گام1, گام2, گام3)
var incompleteStatusPattern = regexp.MustCompile(`گام[123]`)

// rowDataPattern extracts registration data from table rows
var rowDataPattern = regexp.MustCompile(`(?s)<tr[^>]*>.*?ShowDocumentsDetails/([0-9a-fA-F-]{36}).*?<span class=green>([^<]+)</span>.*?</tr>`)

// GetIncompleteRegistrations fetches and parses incomplete registrations from Dashboard.
func (s *Service) GetIncompleteRegistrations(sess *session.Session) ([]models.IncompleteRegistration, error) {
	if !sess.IsAuthenticated() {
		return nil, fmt.Errorf("session not authenticated")
	}

	// Fetch Dashboard HTML
	req, err := http.NewRequest("GET", s.cfg.Services.MyTax.DashboardURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating dashboard request: %w", err)
	}

	s.client.SetCommonHeaders(req)
	req.Header.Set("Referer", s.client.BaseURL()+"/")
	s.client.AddCookies(req, sess.GetCookies())

	s.logger.Info("fetching dashboard for incomplete registrations")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching dashboard: %w", err)
	}
	defer resp.Body.Close()

	// Save new cookies
	if cookies := resp.Cookies(); len(cookies) > 0 {
		sess.MergeCookies(cookies)
	}

	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("error reading dashboard response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("dashboard returned status %d", resp.StatusCode)
	}

	// Parse HTML to extract registrations
	htmlContent := string(body)
	var incompleteRegs []models.IncompleteRegistration

	// Find all table rows with registration data
	matches := rowDataPattern.FindAllStringSubmatch(htmlContent, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			uuid := match[1]
			status := match[2]

			// Only include incomplete registrations (گام1, گام2, گام3)
			if incompleteStatusPattern.MatchString(status) {
				reg := models.IncompleteRegistration{
					UUID:   uuid,
					Status: status,
				}

				// Try to extract more details from the row
				// Extract business name if present
				if nameMatch := regexp.MustCompile(`<span>([^<]+)</span>`).FindStringSubmatch(match[0]); len(nameMatch) > 1 {
					reg.BusinessName = nameMatch[1]
				}

				incompleteRegs = append(incompleteRegs, reg)
				s.logger.Debug("found incomplete registration", "uuid", uuid, "status", status)
			}
		}
	}

	s.logger.Info("found incomplete registrations", "count", len(incompleteRegs))
	return incompleteRegs, nil
}
