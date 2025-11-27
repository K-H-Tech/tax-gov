package tracker

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"

	"github.com/K-H-Tech/auto-tax-gov/internal/client"
	"github.com/K-H-Tech/auto-tax-gov/internal/config"
	"github.com/K-H-Tech/auto-tax-gov/internal/models"
)

// Tracker handles redirect tracking operations.
type Tracker struct {
	cfg    *config.Config
	client *http.Client
	logger *slog.Logger
}

// New creates a new Tracker instance.
func New(cfg *config.Config, logger *slog.Logger) *Tracker {
	return &Tracker{
		cfg:    cfg,
		client: client.NewHTTPClient(&cfg.Tracker),
		logger: logger,
	}
}

// GetClient returns the HTTP client for external use.
func (t *Tracker) GetClient() *http.Client {
	return t.client
}

// TrackRedirects follows a redirect chain and records each step.
func (t *Tracker) TrackRedirects(startURL string, cookies []*http.Cookie) ([]models.RedirectStep, []*http.Cookie, error) {
	var steps []models.RedirectStep
	currentURL := startURL
	stepNumber := 1

	for stepNumber <= t.cfg.Tracker.MaxSteps {
		t.logger.Info("tracking step", "step", stepNumber, "url", currentURL)

		step := models.RedirectStep{
			StepNumber: stepNumber,
			URL:        currentURL,
			Method:     "GET",
			Timestamp:  time.Now(),
		}

		req, err := http.NewRequest("GET", currentURL, nil)
		if err != nil {
			return steps, cookies, fmt.Errorf("error creating request for step %d: %w", stepNumber, err)
		}

		client.SetCommonHeaders(req)

		if t.cfg.Tracker.SaveCookies {
			client.AddCookies(req, cookies)
		}

		step.RequestHeaders = req.Header.Clone()

		requestDump, err := httputil.DumpRequestOut(req, true)
		if err == nil {
			step.RequestBody = string(requestDump)
		}

		resp, err := t.client.Do(req)
		if err != nil {
			return steps, cookies, fmt.Errorf("error making request for step %d: %w", stepNumber, err)
		}

		step.StatusCode = resp.StatusCode
		step.Status = resp.Status
		step.ResponseHeaders = resp.Header.Clone()

		if t.cfg.Tracker.SaveCookies {
			if responseCookies := resp.Cookies(); len(responseCookies) > 0 {
				cookies = append(cookies, responseCookies...)
				t.logger.Debug("saved cookies", "count", len(responseCookies))
			}
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return steps, cookies, fmt.Errorf("error reading response body for step %d: %w", stepNumber, err)
		}
		step.ResponseBody = string(bodyBytes)

		// Try to fetch captcha if this looks like a login page
		if strings.Contains(currentURL, "sso.my.gov.ir/login") && resp.StatusCode == 200 {
			captchaData, err := t.FetchCaptcha(cookies, string(bodyBytes), currentURL)
			if err != nil {
				t.logger.Warn("could not fetch captcha", "error", err)
			} else if captchaData != nil {
				t.logger.Debug("fetched captcha", "key", captchaData.Key)
				step.CaptchaData = captchaData
			}
		}

		// Check for redirect
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			location := resp.Header.Get("Location")
			if location == "" {
				t.logger.Info("redirect status but no Location header, stopping", "status", resp.StatusCode)
				steps = append(steps, step)
				break
			}

			step.RedirectLocation = location
			steps = append(steps, step)

			// Check if redirect location is /Page/Dashboard - stop tracking
			if strings.Contains(location, "/Page/Dashboard") {
				t.logger.Info("reached dashboard, stopping redirect tracking")
				break
			}

			// Handle relative URLs
			if strings.HasPrefix(location, "/") {
				req.URL.Path = location
				req.URL.RawQuery = ""
				if strings.Contains(location, "?") {
					parts := strings.SplitN(location, "?", 2)
					req.URL.Path = parts[0]
					req.URL.RawQuery = parts[1]
				}
				currentURL = req.URL.String()
			} else if strings.HasPrefix(location, "http") {
				currentURL = location
			} else {
				baseURL := req.URL.Scheme + "://" + req.URL.Host
				currentURL = baseURL + "/" + location
			}

			t.logger.Debug("redirect to", "url", currentURL)
			stepNumber++
		} else {
			steps = append(steps, step)
			t.logger.Info("final response", "status", resp.StatusCode)
			break
		}
	}

	if stepNumber > t.cfg.Tracker.MaxSteps {
		return steps, cookies, fmt.Errorf("exceeded maximum redirect steps (%d)", t.cfg.Tracker.MaxSteps)
	}

	return steps, cookies, nil
}

// FollowRedirectChain follows redirects from a starting URL until a non-redirect response.
func (t *Tracker) FollowRedirectChain(cookies []*http.Cookie, startURL string) (string, []*http.Cookie, error) {
	currentURL := startURL
	maxRedirects := 10

	for i := 0; i < maxRedirects; i++ {
		req, err := http.NewRequest("GET", currentURL, nil)
		if err != nil {
			return currentURL, cookies, err
		}

		client.SetCommonHeaders(req)
		client.AddCookies(req, cookies)

		resp, err := t.client.Do(req)
		if err != nil {
			return currentURL, cookies, err
		}
		resp.Body.Close()

		if responseCookies := resp.Cookies(); len(responseCookies) > 0 {
			cookies = append(cookies, responseCookies...)
		}

		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			location := resp.Header.Get("Location")
			if location == "" {
				return currentURL, cookies, nil
			}
			t.logger.Debug("redirect chain", "step", i+1, "location", location, "status", resp.StatusCode)
			currentURL = location
		} else {
			t.logger.Debug("final URL reached", "url", currentURL, "status", resp.StatusCode)
			return currentURL, cookies, nil
		}
	}

	return currentURL, cookies, nil
}

// SaveResults saves tracking results to markdown files.
func (t *Tracker) SaveResults(steps []models.RedirectStep) error {
	outputDir := t.cfg.Tracker.OutputDir
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("error creating output directory: %w", err)
	}

	for _, step := range steps {
		if err := t.generateMarkdownFile(outputDir, step); err != nil {
			t.logger.Error("error generating markdown", "step", step.StepNumber, "error", err)
			continue
		}
		t.logger.Info("generated step file", "path", fmt.Sprintf("%s/step_%02d.md", outputDir, step.StepNumber))
	}

	if err := t.generateSummaryFile(outputDir, steps); err != nil {
		return fmt.Errorf("error generating summary: %w", err)
	}
	t.logger.Info("generated summary", "path", fmt.Sprintf("%s/summary.md", outputDir))

	return nil
}

func (t *Tracker) generateMarkdownFile(outputDir string, step models.RedirectStep) error {
	filename := fmt.Sprintf("%s/step_%02d.md", outputDir, step.StepNumber)

	var content strings.Builder

	content.WriteString(fmt.Sprintf("# Step %d: %s\n\n", step.StepNumber, step.Status))
	content.WriteString(fmt.Sprintf("**Timestamp:** %s\n\n", step.Timestamp.Format(time.RFC3339)))
	content.WriteString(fmt.Sprintf("**URL:** `%s`\n\n", step.URL))

	content.WriteString("## Request\n\n")
	content.WriteString(fmt.Sprintf("**Method:** %s\n\n", step.Method))

	content.WriteString("### Request Headers\n\n```http\n")
	for key, values := range step.RequestHeaders {
		for _, value := range values {
			content.WriteString(fmt.Sprintf("%s: %s\n", key, value))
		}
	}
	content.WriteString("```\n\n")

	content.WriteString("## Response\n\n")
	content.WriteString(fmt.Sprintf("**Status Code:** %d\n\n", step.StatusCode))
	content.WriteString(fmt.Sprintf("**Status:** %s\n\n", step.Status))

	if step.RedirectLocation != "" {
		content.WriteString(fmt.Sprintf("**Redirect Location:** `%s`\n\n", step.RedirectLocation))
	}

	content.WriteString("### Response Headers\n\n```http\n")
	for key, values := range step.ResponseHeaders {
		for _, value := range values {
			content.WriteString(fmt.Sprintf("%s: %s\n", key, value))
		}
	}
	content.WriteString("```\n\n")

	if step.CaptchaData != nil {
		content.WriteString("### Captcha\n\n")
		content.WriteString(fmt.Sprintf("**Captcha Key:** `%s`\n\n", step.CaptchaData.Key))
		content.WriteString(fmt.Sprintf("**CSRF Token:** `%s`\n\n", step.CaptchaData.CSRFToken))
	}

	content.WriteString("### Response Body\n\n")
	if len(step.ResponseBody) > 0 {
		contentType := step.ResponseHeaders.Get("Content-Type")
		if strings.Contains(contentType, "html") {
			content.WriteString("```html\n")
		} else if strings.Contains(contentType, "json") {
			content.WriteString("```json\n")
		} else {
			content.WriteString("```\n")
		}
		body := step.ResponseBody
		if len(body) > 50000 {
			body = body[:50000] + "\n... (truncated)"
		}
		content.WriteString(body)
		content.WriteString("\n```\n\n")
	} else {
		content.WriteString("*(empty)*\n\n")
	}

	content.WriteString("---\n\n")
	if step.StepNumber > 1 {
		content.WriteString(fmt.Sprintf("[← Previous Step](step_%02d.md) | ", step.StepNumber-1))
	}
	content.WriteString("[Summary](summary.md)")
	if step.RedirectLocation != "" {
		content.WriteString(fmt.Sprintf(" | [Next Step →](step_%02d.md)", step.StepNumber+1))
	}
	content.WriteString("\n")

	return os.WriteFile(filename, []byte(content.String()), 0644)
}

func (t *Tracker) generateSummaryFile(outputDir string, steps []models.RedirectStep) error {
	filename := fmt.Sprintf("%s/summary.md", outputDir)

	var content strings.Builder

	content.WriteString("# Redirect Tracking Summary\n\n")
	content.WriteString(fmt.Sprintf("**Total Steps:** %d\n\n", len(steps)))
	content.WriteString(fmt.Sprintf("**Start Time:** %s\n\n", steps[0].Timestamp.Format(time.RFC3339)))
	if len(steps) > 1 {
		duration := steps[len(steps)-1].Timestamp.Sub(steps[0].Timestamp)
		content.WriteString(fmt.Sprintf("**Total Duration:** %v\n\n", duration))
	}

	content.WriteString("## Redirect Chain\n\n")

	for i, step := range steps {
		content.WriteString(fmt.Sprintf("### [Step %d](step_%02d.md)\n\n", step.StepNumber, step.StepNumber))
		content.WriteString(fmt.Sprintf("- **URL:** `%s`\n", step.URL))
		content.WriteString(fmt.Sprintf("- **Status:** %s\n", step.Status))

		if step.RedirectLocation != "" {
			content.WriteString(fmt.Sprintf("- **Redirects to:** `%s`\n", step.RedirectLocation))
		} else if i == len(steps)-1 {
			content.WriteString("- **Final destination** (no further redirects)\n")
		}

		content.WriteString("\n")
	}

	content.WriteString("## Quick Navigation\n\n")
	for _, step := range steps {
		content.WriteString(fmt.Sprintf("- [Step %d: %s](step_%02d.md)\n", step.StepNumber, step.Status, step.StepNumber))
	}

	return os.WriteFile(filename, []byte(content.String()), 0644)
}
