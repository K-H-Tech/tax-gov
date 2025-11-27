package tracker

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/K-H-Tech/auto-tax-gov/internal/client"
	"github.com/K-H-Tech/auto-tax-gov/internal/models"
)

// SendOTP sends an OTP request to the SSO server.
func (t *Tracker) SendOTP(cookies []*http.Cookie, mobile, captchaCode, captchaKey, csrfToken string) (*models.LoginResult, []*http.Cookie, error) {
	loginURL := fmt.Sprintf("%s?mobile=%s&captcha_key=%s&captcha_value=%s",
		t.cfg.SSO.SendOTPURL, mobile, captchaKey, captchaCode)

	req, err := http.NewRequest("POST", loginURL, strings.NewReader(""))
	if err != nil {
		return nil, cookies, fmt.Errorf("error creating OTP request: %w", err)
	}

	client.SetAPIHeaders(req, csrfToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Content-Length", "0")
	req.Header.Set("Origin", "https://sso.my.gov.ir")
	req.Header.Set("Referer", t.cfg.SSO.LoginURL)
	client.AddCookies(req, cookies)

	t.logger.Info("sending OTP request", "mobile", mobile, "url", loginURL)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, cookies, fmt.Errorf("error sending OTP request: %w", err)
	}
	defer resp.Body.Close()

	t.logger.Debug("OTP response", "status", resp.StatusCode)

	// Save new cookies
	if responseCookies := resp.Cookies(); len(responseCookies) > 0 {
		cookies = append(cookies, responseCookies...)
		t.logger.Debug("saved cookies from OTP response", "count", len(responseCookies))
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, cookies, fmt.Errorf("error reading OTP response: %w", err)
	}

	responseBody := string(body)
	result := &models.LoginResult{
		Data: make(map[string]any),
	}

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		redirectLocation := resp.Header.Get("Location")
		result.Message = fmt.Sprintf("OTP sent! Redirecting to: %s", redirectLocation)
		result.Data["redirectLocation"] = redirectLocation
		result.Data["statusCode"] = resp.StatusCode
	} else if resp.StatusCode == 200 {
		if strings.Contains(responseBody, "خطا") || strings.Contains(responseBody, "error") {
			result.Message = "OTP request may have failed. Please check the response."
			result.Data["statusCode"] = resp.StatusCode
			if len(responseBody) > 500 {
				responseBody = responseBody[:500]
			}
			result.Data["bodyPreview"] = responseBody
		} else {
			result.Message = "OTP sent successfully"
			result.Data["statusCode"] = resp.StatusCode
		}
	} else {
		return nil, cookies, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return result, cookies, nil
}

// VerifyOTP verifies the OTP code and completes login.
func (t *Tracker) VerifyOTP(cookies []*http.Cookie, mobile, otpCode, csrfToken string) (*models.LoginResult, []*http.Cookie, error) {
	formData := fmt.Sprintf("password=%s&_csrf=%s&username=%s", otpCode, csrfToken, mobile)

	req, err := http.NewRequest("POST", t.cfg.SSO.VerifyOTPURL, strings.NewReader(formData))
	if err != nil {
		return nil, cookies, fmt.Errorf("error creating OTP verification request: %w", err)
	}

	client.SetFormHeaders(req, t.cfg.SSO.LoginURL)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(formData)))
	client.AddCookies(req, cookies)

	t.logger.Info("verifying OTP", "mobile", mobile)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, cookies, fmt.Errorf("error verifying OTP: %w", err)
	}
	defer resp.Body.Close()

	t.logger.Debug("OTP verification response", "status", resp.StatusCode)

	// Save new cookies
	if responseCookies := resp.Cookies(); len(responseCookies) > 0 {
		cookies = append(cookies, responseCookies...)
		t.logger.Debug("saved cookies from OTP verification", "count", len(responseCookies))
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, cookies, fmt.Errorf("error reading OTP verification response: %w", err)
	}

	responseBody := string(body)
	result := &models.LoginResult{
		Data: make(map[string]any),
	}

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		redirectLocation := resp.Header.Get("Location")
		result.Message = fmt.Sprintf("Login successful! Redirecting to: %s", redirectLocation)
		result.Data["redirectLocation"] = redirectLocation
		result.Data["statusCode"] = resp.StatusCode

		// Follow redirect chain to capture final destination
		if redirectLocation != "" {
			finalURL, newCookies, err := t.FollowRedirectChain(cookies, redirectLocation)
			if err == nil && finalURL != "" {
				cookies = newCookies
				result.Data["finalDestination"] = finalURL
				result.Message = fmt.Sprintf("Login successful! Final destination: %s", finalURL)
			}
		}
	} else if resp.StatusCode == 200 {
		if strings.Contains(responseBody, "خطا") || strings.Contains(responseBody, "error") || strings.Contains(responseBody, "invalid") {
			result.Message = "Login may have failed. Please check OTP code."
			result.Data["statusCode"] = resp.StatusCode
			if len(responseBody) > 500 {
				responseBody = responseBody[:500]
			}
			result.Data["bodyPreview"] = responseBody
			return nil, cookies, fmt.Errorf("OTP verification failed: invalid code or expired")
		}
		result.Message = "OTP verified successfully"
		result.Data["statusCode"] = resp.StatusCode
	} else {
		return nil, cookies, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return result, cookies, nil
}

// AccessDashboard accesses the tax dashboard.
func (t *Tracker) AccessDashboard(cookies []*http.Cookie) (*models.LoginResult, []*http.Cookie, error) {
	req, err := http.NewRequest("GET", t.cfg.Tax.DashboardURL, nil)
	if err != nil {
		return nil, cookies, fmt.Errorf("error creating dashboard request: %w", err)
	}

	client.SetCommonHeaders(req)
	req.Header.Set("Referer", "https://my.tax.gov.ir/")
	client.AddCookies(req, cookies)

	t.logger.Info("accessing dashboard", "url", t.cfg.Tax.DashboardURL)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, cookies, fmt.Errorf("error accessing dashboard: %w", err)
	}
	defer resp.Body.Close()

	t.logger.Debug("dashboard response", "status", resp.StatusCode)

	// Save new cookies
	if responseCookies := resp.Cookies(); len(responseCookies) > 0 {
		cookies = append(cookies, responseCookies...)
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, cookies, fmt.Errorf("error reading dashboard response: %w", err)
	}

	responseBody := string(body)
	result := &models.LoginResult{
		Data: make(map[string]any),
	}

	if resp.StatusCode == 200 {
		result.Message = "Successfully accessed dashboard"
		result.Data["statusCode"] = resp.StatusCode
		result.Data["url"] = t.cfg.Tax.DashboardURL
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
		return nil, cookies, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return result, cookies, nil
}

// StartTaxFileRegistration initiates tax file registration.
func (t *Tracker) StartTaxFileRegistration(cookies []*http.Cookie) (*models.LoginResult, []*http.Cookie, error) {
	payload := "NewRegistrationType=Single&NewRegistrationPostalCode=1111123123&NewRegistrationBusinessName=اسم بیزینس"

	req, err := http.NewRequest("POST", t.cfg.Tax.RegistrationURL, strings.NewReader(payload))
	if err != nil {
		return nil, cookies, fmt.Errorf("error creating tax registration request: %w", err)
	}

	client.SetAPIHeaders(req, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("Origin", "https://my.tax.gov.ir")
	req.Header.Set("Referer", "https://my.tax.gov.ir/Page/ShowDocuments/")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	client.AddCookies(req, cookies)

	t.logger.Info("starting tax file registration", "url", t.cfg.Tax.RegistrationURL)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, cookies, fmt.Errorf("error starting tax registration: %w", err)
	}
	defer resp.Body.Close()

	t.logger.Debug("tax registration response", "status", resp.StatusCode)

	// Save new cookies
	if responseCookies := resp.Cookies(); len(responseCookies) > 0 {
		cookies = append(cookies, responseCookies...)
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, cookies, fmt.Errorf("error reading tax registration response: %w", err)
	}

	result := &models.LoginResult{
		Data: make(map[string]any),
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
				return nil, cookies, fmt.Errorf("%s", errorMsg)
			}

			result.Message = "شروع پرونده مالیاتی موفقیت‌آمیز بود"
			result.Data["statusCode"] = resp.StatusCode
			result.Data["url"] = t.cfg.Tax.RegistrationURL
			result.Data["response"] = jsonResponse

			if msg, ok := jsonResponse["msg"].(string); ok && msg != "" {
				result.Message = msg
			}
		} else {
			result.Message = "شروع پرونده مالیاتی موفقیت‌آمیز بود"
			result.Data["statusCode"] = resp.StatusCode
			result.Data["url"] = t.cfg.Tax.RegistrationURL
			result.Data["bodyLength"] = len(body)
		}
	} else if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		redirectLocation := resp.Header.Get("Location")
		result.Message = fmt.Sprintf("Redirected to: %s", redirectLocation)
		result.Data["redirectLocation"] = redirectLocation
		result.Data["statusCode"] = resp.StatusCode
	} else {
		return nil, cookies, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return result, cookies, nil
}
