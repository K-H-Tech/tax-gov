package tracker

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/K-H-Tech/auto-tax-gov/internal/client"
	"github.com/K-H-Tech/auto-tax-gov/internal/models"
)

// FetchCaptcha retrieves captcha data from the SSO server.
func (t *Tracker) FetchCaptcha(cookies []*http.Cookie, htmlBody, refererURL string) (*models.CaptchaData, error) {
	csrfToken := extractCSRFToken(htmlBody)
	if csrfToken == "" {
		return nil, fmt.Errorf("could not extract CSRF token from HTML")
	}

	req, err := http.NewRequest("GET", t.cfg.SSO.CaptchaURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating captcha request: %w", err)
	}

	client.SetAPIHeaders(req, csrfToken)
	req.Header.Set("Referer", refererURL)
	req.Header.Set("Authorization", t.cfg.SSO.AuthHeader)
	client.AddCookies(req, cookies)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making captcha request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bodyPreview, _ := io.ReadAll(resp.Body)
		preview := string(bodyPreview)
		if len(preview) > 200 {
			preview = preview[:200]
		}
		return nil, fmt.Errorf("captcha request failed with status: %s, body preview: %s", resp.Status, preview)
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("error reading captcha response: %w", err)
	}

	var captchaResp struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}

	if err := json.Unmarshal(body, &captchaResp); err != nil {
		return nil, fmt.Errorf("error parsing captcha JSON: %w", err)
	}

	return &models.CaptchaData{
		Key:        captchaResp.Key,
		ImageData:  captchaResp.Value,
		CSRFToken:  csrfToken,
		AuthHeader: t.cfg.SSO.AuthHeader,
	}, nil
}

// RefreshCaptcha fetches a new captcha using existing session data.
func (t *Tracker) RefreshCaptcha(cookies []*http.Cookie, existingCSRFToken string) (*models.CaptchaData, error) {
	if existingCSRFToken == "" {
		return nil, fmt.Errorf("CSRF token required for captcha refresh")
	}

	req, err := http.NewRequest("GET", t.cfg.SSO.CaptchaURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating captcha request: %w", err)
	}

	client.SetAPIHeaders(req, existingCSRFToken)
	req.Header.Set("Referer", t.cfg.SSO.LoginURL)
	req.Header.Set("Authorization", t.cfg.SSO.AuthHeader)
	client.AddCookies(req, cookies)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making captcha request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("captcha request failed with status: %s", resp.Status)
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("error reading captcha response: %w", err)
	}

	var captchaResp struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}

	if err := json.Unmarshal(body, &captchaResp); err != nil {
		return nil, fmt.Errorf("error parsing captcha JSON: %w", err)
	}

	return &models.CaptchaData{
		Key:        captchaResp.Key,
		ImageData:  captchaResp.Value,
		CSRFToken:  existingCSRFToken,
		AuthHeader: t.cfg.SSO.AuthHeader,
	}, nil
}

// extractCSRFToken extracts CSRF token from HTML content.
func extractCSRFToken(html string) string {
	patterns := []string{
		`<meta name="_csrf" content="([^"]+)"`,
		`<input[^>]*name="_csrf"[^>]*value="([^"]+)"`,
		`"_csrf":"([^"]+)"`,
		`_csrf["\s:]+["']([^"']+)["']`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(html)
		if len(matches) > 1 && matches[1] != "" {
			return matches[1]
		}
	}

	return ""
}

// readResponseBody reads the response body, handling gzip compression.
func readResponseBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error creating gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	return io.ReadAll(reader)
}
