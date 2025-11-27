package mygovauth

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/K-H-Tech/auto-tax-gov/internal/client"
	"github.com/K-H-Tech/auto-tax-gov/internal/session"
)

// SendOTP sends an OTP request to the SSO server.
func (s *Service) SendOTP(sess *session.Session, mobile, captchaCode, captchaKey, csrfToken string) (*AuthResult, error) {
	loginURL := fmt.Sprintf("%s?mobile=%s&captcha_key=%s&captcha_value=%s",
		s.cfg.Services.MyGovAuth.SendOTPURL, mobile, captchaKey, captchaCode)

	req, err := http.NewRequest("POST", loginURL, strings.NewReader(""))
	if err != nil {
		return nil, fmt.Errorf("error creating OTP request: %w", err)
	}

	s.client.SetAPIHeaders(req, csrfToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Content-Length", "0")
	req.Header.Set("Origin", s.client.Origin())
	req.Header.Set("Referer", s.cfg.Services.MyGovAuth.LoginURL)
	s.client.AddCookies(req, sess.GetCookies())

	s.logger.Info("sending OTP request", "mobile", mobile, "url", loginURL)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending OTP request: %w", err)
	}
	defer resp.Body.Close()

	s.logger.Debug("OTP response", "status", resp.StatusCode)

	// Save new cookies
	if cookies := resp.Cookies(); len(cookies) > 0 {
		sess.MergeCookies(cookies)
		s.logger.Debug("saved cookies from OTP response", "count", len(cookies))
	}

	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("error reading OTP response: %w", err)
	}

	responseBody := string(body)
	result := &AuthResult{
		Success: true,
		Data:    make(map[string]any),
	}

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		redirectLocation := resp.Header.Get("Location")
		result.Message = fmt.Sprintf("OTP sent! Redirecting to: %s", redirectLocation)
		result.RedirectURL = redirectLocation
		result.Data["redirectLocation"] = redirectLocation
		result.Data["statusCode"] = resp.StatusCode
	} else if resp.StatusCode == 200 {
		if strings.Contains(responseBody, "خطا") || strings.Contains(responseBody, "error") {
			result.Success = false
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
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return result, nil
}

// VerifyOTP verifies the OTP code and completes login.
// Returns the final redirect URL after successful authentication.
func (s *Service) VerifyOTP(sess *session.Session, mobile, otpCode, csrfToken string) (*AuthResult, error) {
	formData := fmt.Sprintf("password=%s&_csrf=%s&username=%s", otpCode, csrfToken, mobile)

	req, err := http.NewRequest("POST", s.cfg.Services.MyGovAuth.VerifyOTPURL, strings.NewReader(formData))
	if err != nil {
		return nil, fmt.Errorf("error creating OTP verification request: %w", err)
	}

	s.client.SetFormHeaders(req, s.cfg.Services.MyGovAuth.LoginURL)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(formData)))
	s.client.AddCookies(req, sess.GetCookies())

	s.logger.Info("verifying OTP", "mobile", mobile)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error verifying OTP: %w", err)
	}
	defer resp.Body.Close()

	s.logger.Debug("OTP verification response", "status", resp.StatusCode)

	// Save new cookies
	if cookies := resp.Cookies(); len(cookies) > 0 {
		sess.MergeCookies(cookies)
		s.logger.Debug("saved cookies from OTP verification", "count", len(cookies))
	}

	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("error reading OTP verification response: %w", err)
	}

	responseBody := string(body)
	result := &AuthResult{
		Success: true,
		Data:    make(map[string]any),
	}

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		redirectLocation := resp.Header.Get("Location")
		result.Message = fmt.Sprintf("Login successful! Redirecting to: %s", redirectLocation)
		result.RedirectURL = redirectLocation
		result.Data["redirectLocation"] = redirectLocation
		result.Data["statusCode"] = resp.StatusCode

		// Follow redirect chain to capture final destination
		if redirectLocation != "" {
			finalURL, err := s.FollowRedirectChain(sess, redirectLocation)
			if err == nil && finalURL != "" {
				result.Data["finalDestination"] = finalURL
				result.Message = fmt.Sprintf("Login successful! Final destination: %s", finalURL)
				sess.SetRedirectURL(finalURL)
				sess.SetAuthenticated(true)
			}
		}
	} else if resp.StatusCode == 200 {
		if strings.Contains(responseBody, "خطا") || strings.Contains(responseBody, "error") || strings.Contains(responseBody, "invalid") {
			result.Success = false
			result.Message = "Login may have failed. Please check OTP code."
			result.Data["statusCode"] = resp.StatusCode
			if len(responseBody) > 500 {
				responseBody = responseBody[:500]
			}
			result.Data["bodyPreview"] = responseBody
			return nil, fmt.Errorf("OTP verification failed: invalid code or expired")
		}
		result.Message = "OTP verified successfully"
		result.Data["statusCode"] = resp.StatusCode
		sess.SetAuthenticated(true)
	} else {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return result, nil
}
