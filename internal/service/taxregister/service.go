package taxregister

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
	"github.com/K-H-Tech/auto-tax-gov/internal/session"
)

// uuidPattern matches UUID in URLs and responses.
var uuidPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// Service handles the complete 11-step tax registration flow.
type Service struct {
	cfg    *config.Config
	client *Client
	logger *slog.Logger
}

// New creates a new TaxRegister service instance.
func New(cfg *config.Config, logger *slog.Logger) *Service {
	return &Service{
		cfg:    cfg,
		client: NewClient(cfg),
		logger: logger,
	}
}

// Step 1: NewRegistration creates a new tax registration.
// POST https://my.tax.gov.ir/Page/NewRegistration/
// Returns the registration GUID on success.
func (s *Service) NewRegistration(sess *session.Session, req *RegistrationRequest) (*RegistrationResponse, error) {
	if !sess.IsAuthenticated() {
		return nil, fmt.Errorf("session not authenticated")
	}

	// Build form payload matching step_01.raw
	payload := url.Values{}
	payload.Set("NewRegistrationType", req.Type)
	payload.Set("NewRegistrationPostalCode", req.PostalCode)
	payload.Set("NewRegistrationBusinessName", req.BusinessName)

	httpReq, err := http.NewRequest("POST", s.cfg.Services.MyTax.RegistrationURL, strings.NewReader(payload.Encode()))
	if err != nil {
		return nil, fmt.Errorf("error creating registration request: %w", err)
	}

	// Set headers matching step_01.raw
	s.client.SetCommonHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	httpReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	httpReq.Header.Set("Origin", s.client.MyTaxOrigin())
	httpReq.Header.Set("Referer", s.cfg.Services.MyTax.DashboardURL)
	httpReq.Header.Set("Sec-Fetch-Site", "same-origin")
	httpReq.Header.Set("Sec-Fetch-Mode", "cors")
	httpReq.Header.Set("Sec-Fetch-Dest", "empty")
	s.client.AddCookies(httpReq, sess.GetCookies())

	s.logger.Info("Step 1: Creating new registration",
		"url", s.cfg.Services.MyTax.RegistrationURL,
		"type", req.Type,
		"postalCode", req.PostalCode)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error creating registration: %w", err)
	}
	defer resp.Body.Close()

	// Save cookies
	if cookies := resp.Cookies(); len(cookies) > 0 {
		sess.MergeCookies(cookies)
	}

	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("error reading registration response: %w", err)
	}

	s.logger.Debug("Step 1 response", "status", resp.StatusCode, "body", string(body))

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("registration returned status %d", resp.StatusCode)
	}

	// Parse JSON response
	var result RegistrationResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("error parsing registration response: %w", err)
	}

	if !result.Success {
		return &result, fmt.Errorf("registration failed: %s", result.Message)
	}

	// The GUID is in the "msg" field
	result.GUID = result.Message
	sess.SetRegistrationID(result.GUID)

	s.logger.Info("Step 1 complete: Registration created", "guid", result.GUID)

	return &result, nil
}

// Step 2: GetSSOUrl fetches the SSO redirect URL for cross-domain authentication.
// GET https://my.tax.gov.ir/Page/SSODoc/{registrationGUID}
// Returns the URL to TokenLoginProcessWithSignout with a different token UUID.
func (s *Service) GetSSOUrl(sess *session.Session, registrationGUID string) (*SSOResponse, error) {
	if !sess.IsAuthenticated() {
		return nil, fmt.Errorf("session not authenticated")
	}

	ssoDocURL := strings.TrimSuffix(s.cfg.Services.MyTax.SSODocURL, "/") + "/" + registrationGUID

	httpReq, err := http.NewRequest("GET", ssoDocURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating SSODoc request: %w", err)
	}

	// Set headers matching step_02.raw
	s.client.SetCommonHeaders(httpReq)
	httpReq.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	httpReq.Header.Set("Content-Type", "application/json;charset=utf-8")
	httpReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	httpReq.Header.Set("Referer", s.cfg.Services.MyTax.DashboardURL)
	httpReq.Header.Set("Sec-Fetch-Site", "same-origin")
	httpReq.Header.Set("Sec-Fetch-Mode", "cors")
	httpReq.Header.Set("Sec-Fetch-Dest", "empty")
	s.client.AddCookies(httpReq, sess.GetCookies())

	s.logger.Info("Step 2: Fetching SSO URL", "url", ssoDocURL)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error fetching SSO URL: %w", err)
	}
	defer resp.Body.Close()

	// Save cookies
	if cookies := resp.Cookies(); len(cookies) > 0 {
		sess.MergeCookies(cookies)
	}

	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("error reading SSO response: %w", err)
	}

	s.logger.Debug("Step 2 response", "status", resp.StatusCode, "body", string(body))

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("SSODoc returned status %d", resp.StatusCode)
	}

	// Parse JSON response
	var result SSOResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("error parsing SSO response: %w", err)
	}

	if !result.IsLogin {
		return nil, fmt.Errorf("SSO login check failed - user not authenticated")
	}

	s.logger.Info("Step 2 complete: Got SSO URL", "url", result.URL)

	return &result, nil
}

// Steps 3-5: AuthenticateToRegister follows the redirect chain for cross-domain authentication.
// This handles the 302 redirect chain from TokenLoginProcessWithSignout to HomePage.
func (s *Service) AuthenticateToRegister(sess *session.Session, ssoURL string) error {
	if !sess.IsAuthenticated() {
		return fmt.Errorf("session not authenticated")
	}

	s.logger.Info("Steps 3-5: Starting cross-domain auth redirect chain", "url", ssoURL)

	currentURL := ssoURL
	maxRedirects := 10
	referer := s.cfg.Services.MyTax.BaseURL + "/"

	for i := 0; i < maxRedirects; i++ {
		httpReq, err := http.NewRequest("GET", currentURL, nil)
		if err != nil {
			return fmt.Errorf("error creating request at step %d: %w", i+3, err)
		}

		// Set navigation headers matching step_03/04/05.raw
		s.client.SetNavigationHeaders(httpReq, referer)
		s.client.AddCookies(httpReq, sess.GetCookies())

		// Log cookies being sent (first step only)
		if i == 0 {
			s.logger.Debug("Cookies being sent", "header", httpReq.Header.Get("Cookie"))
		}

		resp, err := s.client.Do(httpReq)
		if err != nil {
			return fmt.Errorf("error during redirect chain at step %d: %w", i+3, err)
		}

		// Save cookies from response - critical for ASP.NET_SessionId and PreregisterTAX.*
		if respCookies := resp.Cookies(); len(respCookies) > 0 {
			sess.MergeCookies(respCookies)
			s.logger.Debug("Saved cookies from redirect",
				"step", i+3,
				"count", len(respCookies))
		}

		_, _ = client.ReadResponseBody(resp) // Drain response body
		resp.Body.Close()

		redirectLocation := resp.Header.Get("Location")
		s.logger.Info("Redirect step",
			"step", i+3,
			"url", currentURL,
			"status", resp.StatusCode,
			"redirectTo", redirectLocation)

		if resp.StatusCode == 200 {
			// We've reached a final destination
			if strings.Contains(currentURL, "/Pages/Preaction/") {
				s.logger.Info("Steps 3-5 complete: Reached Preaction page", "finalURL", currentURL)
				return nil
			}

			// Check for login page (auth failure)
			if strings.Contains(currentURL, "/Pages/Login") && !strings.Contains(currentURL, "TokenLogin") {
				return fmt.Errorf("authentication failed - redirected to login page")
			}

			s.logger.Info("Steps 3-5 complete: Reached final destination", "finalURL", currentURL)
			return nil
		}

		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			location := resp.Header.Get("Location")
			if location == "" {
				return fmt.Errorf("redirect without Location header at step %d", i+3)
			}

			// Update referer for next request
			referer = currentURL

			// Handle relative URLs
			if strings.HasPrefix(location, "/") {
				parsedURL, err := url.Parse(currentURL)
				if err == nil {
					currentURL = parsedURL.Scheme + "://" + parsedURL.Host + location
				} else {
					currentURL = s.cfg.Services.RegisterTax.BaseURL + location
				}
			} else {
				currentURL = location
			}
			continue
		}

		return fmt.Errorf("unexpected status %d at step %d", resp.StatusCode, i+3)
	}

	return fmt.Errorf("too many redirects (max %d)", maxRedirects)
}

// Step 6 & 11: GetHomePage fetches the registration HomePage and parses its content.
// GET https://register.tax.gov.ir/Pages/Preaction/HomePage
func (s *Service) GetHomePage(sess *session.Session) (*HomePageData, error) {
	httpReq, err := http.NewRequest("GET", s.cfg.Services.RegisterTax.HomePageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating HomePage request: %w", err)
	}

	s.client.SetNavigationHeaders(httpReq, s.cfg.Services.RegisterTax.BaseURL+"/")
	s.client.AddCookies(httpReq, sess.GetCookies())

	s.logger.Info("Step 6/11: Fetching HomePage", "url", s.cfg.Services.RegisterTax.HomePageURL)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error fetching HomePage: %w", err)
	}
	defer resp.Body.Close()

	// Save cookies
	if cookies := resp.Cookies(); len(cookies) > 0 {
		sess.MergeCookies(cookies)
	}

	if resp.StatusCode == 302 {
		location := resp.Header.Get("Location")
		if strings.Contains(location, "/Login") {
			return nil, fmt.Errorf("not authenticated - redirected to login page")
		}
		return nil, fmt.Errorf("HomePage redirected to %s", location)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HomePage returned status %d", resp.StatusCode)
	}

	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("error reading HomePage: %w", err)
	}

	html := string(body)

	result := &HomePageData{
		RawHTML:  html,
		FormData: make(map[string]string),
	}

	// Extract GUID from hidden field
	result.GUID = ExtractGUIDFromHTML(html)

	// Parse status from HTML (basic extraction)
	if strings.Contains(html, "گام1") {
		result.Status = "step1"
		result.StatusMessage = "مرحله اول"
	} else if strings.Contains(html, "گام2") {
		result.Status = "step2"
		result.StatusMessage = "مرحله دوم"
	} else if strings.Contains(html, "گام3") {
		result.Status = "step3"
		result.StatusMessage = "مرحله سوم"
	} else if strings.Contains(html, "تکمیل") || strings.Contains(html, "completed") {
		result.Status = "completed"
		result.StatusMessage = "تکمیل شده"
	}

	s.logger.Info("Step 6/11 complete: HomePage loaded",
		"guid", result.GUID,
		"status", result.Status,
		"bodyLen", len(html))

	return result, nil
}

// Step 7: GetPublicDataForm fetches the PublicData form and extracts ASP.NET state.
// GET https://register.tax.gov.ir/Pages/Preaction/PublicData
func (s *Service) GetPublicDataForm(sess *session.Session) (*PublicDataForm, error) {
	httpReq, err := http.NewRequest("GET", s.cfg.Services.RegisterTax.PublicDataURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating PublicData request: %w", err)
	}

	s.client.SetNavigationHeaders(httpReq, s.cfg.Services.RegisterTax.HomePageURL)
	s.client.AddCookies(httpReq, sess.GetCookies())

	s.logger.Info("Step 7: Fetching PublicData form", "url", s.cfg.Services.RegisterTax.PublicDataURL)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error fetching PublicData form: %w", err)
	}
	defer resp.Body.Close()

	// Save cookies
	if cookies := resp.Cookies(); len(cookies) > 0 {
		sess.MergeCookies(cookies)
	}

	if resp.StatusCode == 302 {
		location := resp.Header.Get("Location")
		if strings.Contains(location, "/Login") {
			return nil, fmt.Errorf("not authenticated - redirected to login page")
		}
		return nil, fmt.Errorf("PublicData redirected to %s", location)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("PublicData returned status %d", resp.StatusCode)
	}

	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("error reading PublicData form: %w", err)
	}

	html := string(body)

	// Parse ASP.NET form
	form, err := ParseASPNetForm(html)
	if err != nil {
		return nil, fmt.Errorf("error parsing ASP.NET form: %w", err)
	}

	s.logger.Info("Step 7 complete: PublicData form loaded",
		"guid", form.GUID,
		"viewStateLen", len(form.ViewState),
		"dropdownCount", len(form.DropdownOptions))

	return form, nil
}

// Step 8: UpdateDropdown performs an AJAX partial postback to cascade dropdown values.
// POST https://register.tax.gov.ir/Pages/Preaction/PublicData with X-MicrosoftAjax: Delta=true
func (s *Service) UpdateDropdown(sess *session.Session, form *PublicDataForm, eventTarget, updatePanel string, fieldValues map[string]string) (*PublicDataForm, error) {
	payload := BuildAjaxPayload(form, eventTarget, updatePanel, fieldValues)

	httpReq, err := http.NewRequest("POST", s.cfg.Services.RegisterTax.PublicDataURL, strings.NewReader(payload.Encode()))
	if err != nil {
		return nil, fmt.Errorf("error creating AJAX request: %w", err)
	}

	s.client.SetAjaxHeaders(httpReq, s.cfg.Services.RegisterTax.PublicDataURL)
	s.client.AddCookies(httpReq, sess.GetCookies())

	s.logger.Info("Step 8: AJAX dropdown update",
		"eventTarget", eventTarget,
		"updatePanel", updatePanel)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error performing AJAX request: %w", err)
	}
	defer resp.Body.Close()

	// Save cookies
	if cookies := resp.Cookies(); len(cookies) > 0 {
		sess.MergeCookies(cookies)
	}

	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("error reading AJAX response: %w", err)
	}

	s.logger.Debug("Step 8 AJAX response", "status", resp.StatusCode, "bodyLen", len(body))

	// Parse AJAX response and update form state
	ajaxResp, err := ParseAjaxResponse(string(body))
	if err != nil {
		return nil, fmt.Errorf("error parsing AJAX response: %w", err)
	}

	// Update form with new state
	UpdateFormState(form, ajaxResp)

	s.logger.Info("Step 8 complete: Dropdown updated",
		"updatedDropdowns", len(ajaxResp.UpdatedDropdowns))

	return form, nil
}

// Steps 9-10: SubmitPublicData submits the complete PublicData form.
// POST https://register.tax.gov.ir/Pages/Preaction/PublicData
func (s *Service) SubmitPublicData(sess *session.Session, form *PublicDataForm, data *PublicDataRequest) error {
	// Build form fields matching step_09.raw
	fields := map[string]string{
		// Required fields
		"ctl00$CPC$DDLNewRegistrationCause":   data.RegistrationCause,
		"ctl00$CPC$DDLIsTejari":               data.IsTejari,
		"ctl00$CPC$TextBoxFinantialStartDate": data.FinancialStartDate,
		"ctl00$CPC$TextBoxPDName":             data.BusinessName,
		"ctl00$CPC$DDLGroupOneTypes":          data.GroupOneType,
		"ctl00$CPC$DDLPDLegalType":            data.LegalType,
		"ctl00$CPC$DDLPDNewLegalGroup":        data.NewLegalGroup,
		"ctl00$CPC$DDLPDNewLegalType":         data.NewLegalType,
		"ctl00$CPC$DDLHasJobLicence":          data.HasJobLicense,
		"ctl00$CPC$DDLPDOwnership":            data.Ownership,
		"ctl00$CPC$DDLFinantialDayStart":      data.FinancialDayStart,
		"ctl00$CPC$DDLFinantialMonthStart":    data.FinancialMonthStart,

		// Hidden fields
		"ctl00$CPC$HFGUID": form.GUID,

		// Submit button
		"ctl00$CPC$ButtonPRSave": "ثبت",
	}

	// Optional fields
	if data.Website != "" {
		fields["ctl00$CPC$TextBoxAddressWebsite"] = data.Website
	}
	if data.Email != "" {
		fields["ctl00$CPC$TextBoxAddressEmail"] = data.Email
	}

	payload := BuildFormPayload(form, fields)

	httpReq, err := http.NewRequest("POST", s.cfg.Services.RegisterTax.PublicDataURL, strings.NewReader(payload.Encode()))
	if err != nil {
		return fmt.Errorf("error creating form submit request: %w", err)
	}

	s.client.SetFormSubmitHeaders(httpReq, s.cfg.Services.RegisterTax.PublicDataURL)
	s.client.AddCookies(httpReq, sess.GetCookies())

	s.logger.Info("Steps 9-10: Submitting PublicData form",
		"url", s.cfg.Services.RegisterTax.PublicDataURL,
		"guid", form.GUID)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("error submitting form: %w", err)
	}
	defer resp.Body.Close()

	// Save cookies
	if cookies := resp.Cookies(); len(cookies) > 0 {
		sess.MergeCookies(cookies)
	}

	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return fmt.Errorf("error reading form response: %w", err)
	}

	s.logger.Debug("Steps 9-10 response", "status", resp.StatusCode, "bodyLen", len(body))

	responseHTML := string(body)

	// Check for success/error indicators
	if resp.StatusCode == 200 {
		if strings.Contains(responseHTML, "خطا") && !strings.Contains(responseHTML, "بدون خطا") {
			s.logger.Warn("Form submission may have errors", "preview", truncateString(responseHTML, 500))
			return fmt.Errorf("form submission returned errors")
		}

		s.logger.Info("Steps 9-10 complete: Form submitted successfully")
		return nil
	}

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		s.logger.Info("Steps 9-10: Form submission redirected", "location", location)
		return nil
	}

	return fmt.Errorf("form submission returned status %d", resp.StatusCode)
}

// ExecuteFullFlow executes the complete 11-step registration flow.
func (s *Service) ExecuteFullFlow(sess *session.Session, req *RegistrationFlowRequest) (*RegistrationFlowResponse, error) {
	response := &RegistrationFlowResponse{
		Steps: make([]StepResult, 0, 11),
	}

	// Step 1: Create new registration
	regReq := &RegistrationRequest{
		Type:         req.Type,
		PostalCode:   req.PostalCode,
		BusinessName: req.BusinessName,
	}

	regResp, err := s.NewRegistration(sess, regReq)
	if err != nil {
		response.Steps = append(response.Steps, StepResult{Step: 1, Name: "NewRegistration", Success: false, Message: err.Error()})
		return response, err
	}
	response.Steps = append(response.Steps, StepResult{Step: 1, Name: "NewRegistration", Success: true, URL: s.cfg.Services.MyTax.RegistrationURL})
	response.GUID = regResp.GUID

	// Step 2: Get SSO URL
	ssoResp, err := s.GetSSOUrl(sess, regResp.GUID)
	if err != nil {
		response.Steps = append(response.Steps, StepResult{Step: 2, Name: "GetSSOUrl", Success: false, Message: err.Error()})
		return response, err
	}
	response.Steps = append(response.Steps, StepResult{Step: 2, Name: "GetSSOUrl", Success: true, URL: ssoResp.URL})

	// Steps 3-5: Authenticate to register.tax.gov.ir
	if err := s.AuthenticateToRegister(sess, ssoResp.URL); err != nil {
		response.Steps = append(response.Steps, StepResult{Step: 3, Name: "AuthenticateToRegister", Success: false, Message: err.Error()})
		return response, err
	}
	response.Steps = append(response.Steps, StepResult{Step: 3, Name: "AuthenticateToRegister", Success: true, Message: "Redirect chain completed"})

	// Step 6: Get HomePage
	homeData, err := s.GetHomePage(sess)
	if err != nil {
		response.Steps = append(response.Steps, StepResult{Step: 6, Name: "GetHomePage", Success: false, Message: err.Error()})
		return response, err
	}
	response.Steps = append(response.Steps, StepResult{Step: 6, Name: "GetHomePage", Success: true, URL: s.cfg.Services.RegisterTax.HomePageURL})

	// Update GUID from HomePage if available
	if homeData.GUID != "" {
		response.GUID = homeData.GUID
	}

	// Step 7: Get PublicData form
	form, err := s.GetPublicDataForm(sess)
	if err != nil {
		response.Steps = append(response.Steps, StepResult{Step: 7, Name: "GetPublicDataForm", Success: false, Message: err.Error()})
		return response, err
	}
	response.Steps = append(response.Steps, StepResult{Step: 7, Name: "GetPublicDataForm", Success: true, URL: s.cfg.Services.RegisterTax.PublicDataURL})

	// Steps 8-10: Submit form if data provided
	if req.PublicData != nil {
		// Step 8: AJAX dropdown updates (if needed)
		// This would be called multiple times for cascade dropdowns
		// For now, we skip AJAX if all values are pre-selected
		response.Steps = append(response.Steps, StepResult{Step: 8, Name: "UpdateDropdown", Success: true, Message: "Skipped (values pre-selected)"})

		// Steps 9-10: Submit form
		if err := s.SubmitPublicData(sess, form, req.PublicData); err != nil {
			response.Steps = append(response.Steps, StepResult{Step: 9, Name: "SubmitPublicData", Success: false, Message: err.Error()})
			return response, err
		}
		response.Steps = append(response.Steps, StepResult{Step: 9, Name: "SubmitPublicData", Success: true, URL: s.cfg.Services.RegisterTax.PublicDataURL})

		// Step 11: Verify on HomePage
		finalHomeData, err := s.GetHomePage(sess)
		if err != nil {
			response.Steps = append(response.Steps, StepResult{Step: 11, Name: "FinalHomePage", Success: false, Message: err.Error()})
			return response, err
		}
		response.Steps = append(response.Steps, StepResult{Step: 11, Name: "FinalHomePage", Success: true, URL: s.cfg.Services.RegisterTax.HomePageURL})
		response.Status = finalHomeData.Status
		response.FinalPageURL = s.cfg.Services.RegisterTax.HomePageURL
	}

	response.Success = true
	response.Message = "Registration flow completed successfully"

	return response, nil
}

// GetConfig returns the service configuration.
func (s *Service) GetConfig() *config.Config {
	return s.cfg
}

// truncateString truncates a string to maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
