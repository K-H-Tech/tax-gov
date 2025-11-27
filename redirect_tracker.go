package main

import (
	"compress/gzip"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"regexp"
	"strings"
	"time"
)

type RedirectStep struct {
	StepNumber       int
	URL              string
	Method           string
	RequestHeaders   http.Header
	RequestBody      string
	StatusCode       int
	Status           string
	ResponseHeaders  http.Header
	ResponseBody     string
	RedirectLocation string
	Timestamp        time.Time
	CaptchaData      *CaptchaData
}

type CaptchaData struct {
	Key        string
	ImageData  string
	CSRFToken  string
	AuthHeader string
}

type Config struct {
	Timeout         time.Duration
	MaxSteps        int
	FollowRedirects bool
	VerifyTLS       bool
	SaveCookies     bool
}

var (
	currentCaptcha *CaptchaData
	currentClient  *http.Client
	currentCookies []*http.Cookie
)

func main() {
	// Start HTTP server
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/api/start-tracker", handleStartTracker)
	http.HandleFunc("/api/refresh-captcha", handleRefreshCaptcha)
	http.HandleFunc("/api/send-otp", handleSendOTP)
	http.HandleFunc("/api/verify-otp", handleVerifyOTP)
	http.HandleFunc("/api/access-dashboard", handleAccessDashboard)
	http.HandleFunc("/api/start-tax-file", handleStartTaxFile)

	port := "8080"
	fmt.Printf("Starting web server on http://localhost:%s\n", port)
	fmt.Printf("Open your browser and navigate to: http://localhost:%s\n\n", port)

	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Printf("Error starting server: %v\n", err)
		os.Exit(1)
	}
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	content, err := os.ReadFile("index.html")
	if err != nil {
		http.Error(w, "Error loading page", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}

func handleStartTracker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	startURL := "https://sso.my.gov.ir/oauth2/authorize?response_type=code&scope=openid%20profile&client_id=my.tax&state=state1&redirect_uri=https://my.tax.gov.ir/myiran/sso"

	config := Config{
		Timeout:         60 * time.Second,
		MaxSteps:        20,
		FollowRedirects: true,
		VerifyTLS:       true,
		SaveCookies:     true,
	}

	fmt.Println("Starting redirect tracking...")

	steps, err := trackRedirects(startURL, config)
	if err != nil {
		fmt.Printf("Error tracking redirects: %v\n", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	fmt.Printf("Total steps: %d\n", len(steps))

	// Save results to files
	go saveResults(steps)

	// Get captcha from last step if available
	var captcha *CaptchaData
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].CaptchaData != nil {
			captcha = steps[i].CaptchaData
			currentCaptcha = captcha
			break
		}
	}

	if captcha == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "No captcha found in redirect chain",
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"captcha": map[string]string{
			"key":       captcha.Key,
			"imageData": captcha.ImageData,
			"csrfToken": captcha.CSRFToken,
		},
	})
}

func handleRefreshCaptcha(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if currentClient == nil || currentCookies == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "No active session. Please reload the page.",
		})
		return
	}

	// Fetch new captcha using existing session
	captcha, err := fetchCaptcha(currentClient, currentCookies, "", "https://sso.my.gov.ir/login")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	currentCaptcha = captcha

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"captcha": map[string]string{
			"key":       captcha.Key,
			"imageData": captcha.ImageData,
			"csrfToken": captcha.CSRFToken,
		},
	})
}

func handleSendOTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var otpReq struct {
		Mobile      string `json:"mobile"`
		CaptchaCode string `json:"captchaCode"`
		CaptchaKey  string `json:"captchaKey"`
		CSRFToken   string `json:"csrfToken"`
	}

	err := json.NewDecoder(r.Body).Decode(&otpReq)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid request",
		})
		return
	}

	fmt.Printf("Send OTP attempt - Mobile: %s, CaptchaCode: %s, CaptchaKey: %s\n",
		otpReq.Mobile, otpReq.CaptchaCode, otpReq.CaptchaKey)

	if currentClient == nil || currentCookies == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "No active session. Please reload the page.",
		})
		return
	}

	// Send OTP request to /sendOtp endpoint
	result, err := sendOTPRequest(currentClient, currentCookies, otpReq.Mobile, otpReq.CaptchaCode, otpReq.CaptchaKey, otpReq.CSRFToken)
	if err != nil {
		fmt.Printf("Send OTP error: %v\n", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": result.Message,
		"data":    result.Data,
	})
}

func handleVerifyOTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var verifyReq struct {
		Mobile    string `json:"mobile"`
		OTPCode   string `json:"otpCode"`
		CSRFToken string `json:"csrfToken"`
	}

	err := json.NewDecoder(r.Body).Decode(&verifyReq)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid request",
		})
		return
	}

	fmt.Printf("Verify OTP attempt - Mobile: %s, OTPCode: %s\n",
		verifyReq.Mobile, verifyReq.OTPCode)

	if currentClient == nil || currentCookies == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "No active session. Please reload the page.",
		})
		return
	}

	// Submit OTP to /login_otp endpoint
	result, err := verifyOTPRequest(currentClient, currentCookies, verifyReq.Mobile, verifyReq.OTPCode, verifyReq.CSRFToken)
	if err != nil {
		fmt.Printf("Verify OTP error: %v\n", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": result.Message,
		"data":    result.Data,
	})
}

func handleAccessDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	fmt.Println("Attempting to access dashboard...")

	if currentClient == nil || currentCookies == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "No active session. Please reload the page.",
		})
		return
	}

	// Access the dashboard
	result, err := accessDashboard(currentClient, currentCookies)
	if err != nil {
		fmt.Printf("Dashboard access error: %v\n", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": result.Message,
		"data":    result.Data,
	})
}

func handleStartTaxFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	fmt.Println("Starting tax file registration...")

	if currentClient == nil || currentCookies == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "No active session. Please reload the page.",
		})
		return
	}

	// Start tax file registration
	result, err := startTaxFileRegistration(currentClient, currentCookies)
	if err != nil {
		fmt.Printf("Tax file registration error: %v\n", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": result.Message,
		"data":    result.Data,
	})
}

type LoginResult struct {
	Message string
	Data    map[string]interface{}
}

func sendOTPRequest(client *http.Client, cookies []*http.Cookie, mobile string, captchaCode string, captchaKey string, csrfToken string) (*LoginResult, error) {
	// Build the sendOtp URL with query parameters
	loginURL := fmt.Sprintf("https://sso.my.gov.ir/sendOtp?mobile=%s&captcha_key=%s&captcha_value=%s",
		mobile, captchaKey, captchaCode)

	// Create POST request with empty body (Content-Length: 0)
	req, err := http.NewRequest("POST", loginURL, strings.NewReader(""))
	if err != nil {
		return nil, fmt.Errorf("error creating login request: %w", err)
	}

	// Set headers to match browser request
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,fa;q=0.8,ar;q=0.7")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Content-Length", "0")
	req.Header.Set("Origin", "https://sso.my.gov.ir")
	req.Header.Set("Referer", "https://sso.my.gov.ir/login")
	req.Header.Set("priority", "u=1, i")
	req.Header.Set("sec-ch-ua", `"Chromium";v="142", "Google Chrome";v="142", "Not_A Brand";v="99"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"macOS"`)
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "same-origin")
	req.Header.Set("x-csrf-token", csrfToken)

	// Add cookies
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	// Make request
	fmt.Printf("Sending OTP request to: %s\n", loginURL)
	fmt.Printf("Request params: mobile=%s, captcha_value=%s, captcha_key=%s\n", mobile, captchaCode, captchaKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error submitting login form: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("Login response status: %d %s\n", resp.StatusCode, resp.Status)
	fmt.Printf("Login response headers: %v\n", resp.Header)
	fmt.Printf("Login response body: %v\n", resp.Body)

	// Check if response is gzip-compressed
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error creating gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	// Read response body
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	responseBody := string(bodyBytes)
	fmt.Printf("Response body length: %d bytes\n", len(responseBody))

	// Check for redirect (302/301) or success (200)
	result := &LoginResult{
		Data: make(map[string]interface{}),
	}

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		// Redirect - likely means login was successful
		redirectLocation := resp.Header.Get("Location")
		result.Message = fmt.Sprintf("Login successful! Redirecting to: %s", redirectLocation)
		result.Data["redirectLocation"] = redirectLocation
		result.Data["statusCode"] = resp.StatusCode

		// Save new cookies if any
		if responseCookies := resp.Cookies(); len(responseCookies) > 0 {
			currentCookies = append(currentCookies, responseCookies...)
			fmt.Printf("Saved %d new cookie(s) from login response\n", len(responseCookies))
		}
	} else if resp.StatusCode == 200 {
		// Check if there's an error message in the response
		if strings.Contains(responseBody, "خطا") || strings.Contains(responseBody, "error") {
			result.Message = "Login may have failed. Please check the response."
			result.Data["statusCode"] = resp.StatusCode
			result.Data["bodyPreview"] = responseBody[:min(500, len(responseBody))]
		} else {
			result.Message = "Login form submitted successfully"
			result.Data["statusCode"] = resp.StatusCode
		}
	} else {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return result, nil
}

func verifyOTPRequest(client *http.Client, cookies []*http.Cookie, mobile string, otpCode string, csrfToken string) (*LoginResult, error) {
	// Prepare form data for login_otp endpoint
	formData := fmt.Sprintf("password=%s&_csrf=%s&username=%s",
		otpCode, csrfToken, mobile)

	// Create POST request to login_otp endpoint
	loginURL := "https://sso.my.gov.ir/login_otp"
	req, err := http.NewRequest("POST", loginURL, strings.NewReader(formData))
	if err != nil {
		return nil, fmt.Errorf("error creating OTP verification request: %w", err)
	}

	// Set headers to match browser request
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,fa;q=0.8,ar;q=0.7")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(formData)))
	req.Header.Set("Origin", "https://sso.my.gov.ir")
	req.Header.Set("Referer", "https://sso.my.gov.ir/login")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("priority", "u=0, i")
	req.Header.Set("sec-ch-ua", `"Chromium";v="142", "Google Chrome";v="142", "Not_A Brand";v="99"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"macOS"`)
	req.Header.Set("sec-fetch-dest", "document")
	req.Header.Set("sec-fetch-mode", "navigate")
	req.Header.Set("sec-fetch-site", "same-origin")
	req.Header.Set("sec-fetch-user", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	// Add cookies
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	// Make request
	fmt.Printf("Verifying OTP at: %s\n", loginURL)
	fmt.Printf("Form data: username=%s, password=%s\n", mobile, otpCode)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error verifying OTP: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("OTP verification response status: %d %s\n", resp.StatusCode, resp.Status)
	fmt.Printf("OTP verification response headers: %v\n", resp.Header)

	// Save new cookies if any
	if responseCookies := resp.Cookies(); len(responseCookies) > 0 {
		currentCookies = append(currentCookies, responseCookies...)
		fmt.Printf("Saved %d new cookie(s) from OTP verification response\n", len(responseCookies))
	}

	// Check if response is gzip-compressed
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error creating gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	// Read response body
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	responseBody := string(bodyBytes)
	fmt.Printf("Response body length: %d bytes\n", len(responseBody))

	// Check for redirect (302/301) or success (200)
	result := &LoginResult{
		Data: make(map[string]interface{}),
	}

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		// Redirect - likely means login was successful
		redirectLocation := resp.Header.Get("Location")
		result.Message = fmt.Sprintf("Login successful! Redirecting to: %s", redirectLocation)
		result.Data["redirectLocation"] = redirectLocation
		result.Data["statusCode"] = resp.StatusCode

		// Track redirects if needed
		fmt.Printf("Following redirect to: %s\n", redirectLocation)

		// Optionally follow the redirect to capture final destination
		if redirectLocation != "" {
			finalURL, err := followRedirectChain(client, currentCookies, redirectLocation)
			if err == nil && finalURL != "" {
				result.Data["finalDestination"] = finalURL
				result.Message = fmt.Sprintf("Login successful! Final destination: %s", finalURL)
			}
		}
	} else if resp.StatusCode == 200 {
		// Check if there's an error message in the response
		if strings.Contains(responseBody, "خطا") || strings.Contains(responseBody, "error") || strings.Contains(responseBody, "invalid") {
			result.Message = "Login may have failed. Please check OTP code."
			result.Data["statusCode"] = resp.StatusCode
			result.Data["bodyPreview"] = responseBody[:min(500, len(responseBody))]
			return nil, fmt.Errorf("OTP verification failed: invalid code or expired")
		} else {
			result.Message = "OTP verified successfully"
			result.Data["statusCode"] = resp.StatusCode
		}
	} else {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return result, nil
}

func accessDashboard(client *http.Client, cookies []*http.Cookie) (*LoginResult, error) {
	// Dashboard URL
	dashboardURL := "https://my.tax.gov.ir/Page/Dashboard"

	// Create GET request
	req, err := http.NewRequest("GET", dashboardURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating dashboard request: %w", err)
	}

	// Set headers to match browser request
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,fa;q=0.8,ar;q=0.7")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Referer", "https://my.tax.gov.ir/")
	req.Header.Set("sec-ch-ua", `"Chromium";v="142", "Google Chrome";v="142", "Not_A Brand";v="99"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"macOS"`)
	req.Header.Set("sec-fetch-dest", "document")
	req.Header.Set("sec-fetch-mode", "navigate")
	req.Header.Set("sec-fetch-site", "cross-site")
	req.Header.Set("sec-fetch-user", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	// Add cookies from previous requests
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	// Make request
	fmt.Printf("Accessing dashboard: %s\n", dashboardURL)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error accessing dashboard: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("Dashboard response status: %d %s\n", resp.StatusCode, resp.Status)
	fmt.Printf("Dashboard response headers: %v\n", resp.Header)

	// Save new cookies if any
	if responseCookies := resp.Cookies(); len(responseCookies) > 0 {
		currentCookies = append(currentCookies, responseCookies...)
		fmt.Printf("Saved %d new cookie(s) from dashboard response\n", len(responseCookies))
	}

	// Check if response is gzip-compressed
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error creating gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	// Read response body
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	responseBody := string(bodyBytes)
	fmt.Printf("Response body length: %d bytes\n", len(responseBody))

	// Check for success
	result := &LoginResult{
		Data: make(map[string]interface{}),
	}

	if resp.StatusCode == 200 {
		result.Message = "Successfully accessed dashboard"
		result.Data["statusCode"] = resp.StatusCode
		result.Data["url"] = dashboardURL
		result.Data["bodyLength"] = len(responseBody)

		// Save cookies for reference
		cookieStrings := make([]string, 0)
		for _, cookie := range currentCookies {
			cookieStrings = append(cookieStrings, fmt.Sprintf("%s=%s", cookie.Name, cookie.Value))
		}
		result.Data["cookies"] = cookieStrings

		// Check if we got the actual dashboard content (not an error page)
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
		// Redirect
		redirectLocation := resp.Header.Get("Location")
		result.Message = fmt.Sprintf("Dashboard redirected to: %s", redirectLocation)
		result.Data["redirectLocation"] = redirectLocation
		result.Data["statusCode"] = resp.StatusCode
	} else {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return result, nil
}

func startTaxFileRegistration(client *http.Client, cookies []*http.Cookie) (*LoginResult, error) {
	// Tax registration URL from step_0.md
	taxURL := "https://my.tax.gov.ir/Page/NewRegistration/"

	// Payload from step_0.md
	payload := "NewRegistrationType=Single&NewRegistrationPostalCode=1111123123&NewRegistrationBusinessName=اسم بیزینس"

	// Create POST request with the payload
	req, err := http.NewRequest("POST", taxURL, strings.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("error creating tax registration request: %w", err)
	}

	// Set headers to match browser request from step_0.md
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,fa;q=0.8,ar;q=0.7")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("Origin", "https://my.tax.gov.ir")
	req.Header.Set("Referer", "https://my.tax.gov.ir/Page/ShowDocuments/")
	req.Header.Set("sec-ch-ua", `"Chromium";v="142", "Google Chrome";v="142", "Not_A Brand";v="99"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"macOS"`)
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "same-origin")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	// Add cookies from previous requests
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	// Make request
	fmt.Printf("Starting tax file registration: %s\n", taxURL)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error starting tax registration: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("Tax registration response status: %d %s\n", resp.StatusCode, resp.Status)
	fmt.Printf("Tax registration response headers: %v\n", resp.Header)

	// Save new cookies if any
	if responseCookies := resp.Cookies(); len(responseCookies) > 0 {
		currentCookies = append(currentCookies, responseCookies...)
		fmt.Printf("Saved %d new cookie(s) from tax registration response\n", len(responseCookies))
	}

	// Check if response is gzip-compressed
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error creating gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	// Read response body
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	responseBody := string(bodyBytes)
	fmt.Printf("Response body length: %d bytes\n", len(responseBody))
	fmt.Printf("Response body: %s\n", responseBody)

	// Check for success
	result := &LoginResult{
		Data: make(map[string]interface{}),
	}

	if resp.StatusCode == 200 {
		// Try to parse JSON response
		var jsonResponse map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &jsonResponse); err == nil {
			// Check for isSuccess field
			if isSuccess, ok := jsonResponse["isSuccess"].(bool); ok {
				if !isSuccess {
					// Extract error message from msg field
					errorMsg := "خطا در شروع پرونده مالیاتی"
					if msg, ok := jsonResponse["msg"].(string); ok && msg != "" {
						errorMsg = msg
					}
					return nil, fmt.Errorf("%s", errorMsg)
				}
			}

			// Success case
			result.Message = "شروع پرونده مالیاتی موفقیت‌آمیز بود"
			result.Data["statusCode"] = resp.StatusCode
			result.Data["url"] = taxURL
			result.Data["response"] = jsonResponse

			// If there's a success message in the response, use it
			if msg, ok := jsonResponse["msg"].(string); ok && msg != "" {
				result.Message = msg
			}
		} else {
			// Not JSON response or parsing failed
			result.Message = "شروع پرونده مالیاتی موفقیت‌آمیز بود"
			result.Data["statusCode"] = resp.StatusCode
			result.Data["url"] = taxURL
			result.Data["bodyLength"] = len(responseBody)

			// Save response body preview
			bodyPreview := responseBody
			if len(bodyPreview) > 1000 {
				bodyPreview = bodyPreview[:1000]
			}
			result.Data["bodyPreview"] = bodyPreview
		}
	} else if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		// Redirect
		redirectLocation := resp.Header.Get("Location")
		result.Message = fmt.Sprintf("Redirected to: %s", redirectLocation)
		result.Data["redirectLocation"] = redirectLocation
		result.Data["statusCode"] = resp.StatusCode
	} else {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return result, nil
}

func followRedirectChain(client *http.Client, cookies []*http.Cookie, startURL string) (string, error) {
	currentURL := startURL
	maxRedirects := 10

	for i := 0; i < maxRedirects; i++ {
		req, err := http.NewRequest("GET", currentURL, nil)
		if err != nil {
			return currentURL, err
		}

		// Set headers
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")

		// Add cookies
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}

		resp, err := client.Do(req)
		if err != nil {
			return currentURL, err
		}
		resp.Body.Close()

		// Save cookies
		if responseCookies := resp.Cookies(); len(responseCookies) > 0 {
			currentCookies = append(currentCookies, responseCookies...)
		}

		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			location := resp.Header.Get("Location")
			fmt.Println("---------")
			fmt.Println(location)
			fmt.Println("---------")
			if location == "" {
				return currentURL, nil
			}
			fmt.Printf("  -> Redirect %d: %s (status %d)\n", i+1, location, resp.StatusCode)
			currentURL = location
		} else {
			fmt.Println("==========")
			fmt.Println("Final URL reached:", currentURL)
			fmt.Println(resp.StatusCode)
			fmt.Println("==========")
			return currentURL, nil
		}
	}

	return currentURL, nil
}

func saveResults(steps []RedirectStep) {
	outputDir := "redirect_steps"
	err := os.MkdirAll(outputDir, 0755)
	if err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		return
	}

	for _, step := range steps {
		err := generateMarkdownFile(outputDir, step)
		if err != nil {
			fmt.Printf("Error generating markdown for step %d: %v\n", step.StepNumber, err)
			continue
		}
		fmt.Printf("Generated: %s/step_%02d.md\n", outputDir, step.StepNumber)
	}

	err = generateSummaryFile(outputDir, steps)
	if err != nil {
		fmt.Printf("Error generating summary: %v\n", err)
	} else {
		fmt.Printf("Generated summary: %s/summary.md\n", outputDir)
	}

	fmt.Println("Results saved!")
}

func trackRedirects(startURL string, config Config) ([]RedirectStep, error) {
	var steps []RedirectStep
	currentURL := startURL
	stepNumber := 1

	// Create HTTP client that doesn't follow redirects automatically
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: config.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: !config.VerifyTLS,
			},
			DisableKeepAlives: false,
		},
	}

	// Store client globally for captcha refresh
	currentClient = client

	// Cookie jar for maintaining session
	var cookies []*http.Cookie

	for stepNumber <= config.MaxSteps {
		fmt.Printf("Step %d: %s\n", stepNumber, currentURL)

		step := RedirectStep{
			StepNumber: stepNumber,
			URL:        currentURL,
			Method:     "GET",
			Timestamp:  time.Now(),
		}

		// Create request
		req, err := http.NewRequest("GET", currentURL, nil)
		if err != nil {
			return steps, fmt.Errorf("error creating request for step %d: %w", stepNumber, err)
		}

		// Set common headers
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Upgrade-Insecure-Requests", "1")

		// Add cookies from previous requests
		if config.SaveCookies {
			for _, cookie := range cookies {
				req.AddCookie(cookie)
			}
		}

		step.RequestHeaders = req.Header.Clone()

		// Dump request
		requestDump, err := httputil.DumpRequestOut(req, true)
		if err == nil {
			step.RequestBody = string(requestDump)
		}

		// Make request
		resp, err := client.Do(req)
		if err != nil {
			return steps, fmt.Errorf("error making request for step %d: %w", stepNumber, err)
		}

		step.StatusCode = resp.StatusCode
		step.Status = resp.Status
		step.ResponseHeaders = resp.Header.Clone()

		// Save cookies from response
		if config.SaveCookies {
			if responseCookies := resp.Cookies(); len(responseCookies) > 0 {
				cookies = append(cookies, responseCookies...)
				currentCookies = cookies // Store globally for captcha refresh
				fmt.Printf("  -> Saved %d cookie(s)\n", len(responseCookies))
			}
		}

		// Read response body
		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return steps, fmt.Errorf("error reading response body for step %d: %w", stepNumber, err)
		}
		step.ResponseBody = string(bodyBytes)

		// Try to fetch captcha if this looks like a login page
		if strings.Contains(currentURL, "sso.my.gov.ir/login") && resp.StatusCode == 200 {
			captchaData, err := fetchCaptcha(client, cookies, string(bodyBytes), currentURL)
			if err != nil {
				fmt.Printf("  -> Warning: Could not fetch captcha: %v\n", err)
			} else if captchaData != nil {
				fmt.Printf("  -> Fetched captcha (key: %s)\n", captchaData.Key)
				step.CaptchaData = captchaData
			}
		}

		// Check for redirect
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			location := resp.Header.Get("Location")
			if location == "" {
				fmt.Printf("  -> Status %d but no Location header, stopping.\n", resp.StatusCode)
				steps = append(steps, step)
				break
			}

			step.RedirectLocation = location
			steps = append(steps, step)

			// Check if redirect location is /Page/Dashboard - stop tracking
			if strings.Contains(location, "/Page/Dashboard") {
				fmt.Printf("  -> Reached /Page/Dashboard, stopping redirect tracking.\n")
				break
			}

			// Handle relative URLs
			if strings.HasPrefix(location, "/") {
				// Relative URL - need to construct full URL
				req.URL.Path = location
				req.URL.RawQuery = ""
				if strings.Contains(location, "?") {
					parts := strings.SplitN(location, "?", 2)
					req.URL.Path = parts[0]
					req.URL.RawQuery = parts[1]
				}
				currentURL = req.URL.String()
			} else if strings.HasPrefix(location, "http") {
				// Absolute URL
				currentURL = location
			} else {
				// Relative URL without leading slash
				baseURL := req.URL.Scheme + "://" + req.URL.Host
				currentURL = baseURL + "/" + location
			}

			fmt.Printf("  -> Redirect to: %s\n", currentURL)
			stepNumber++
		} else {
			// Not a redirect, final response
			steps = append(steps, step)
			fmt.Printf("  -> Final response (status %d)\n", resp.StatusCode)
			break
		}
	}

	if stepNumber > config.MaxSteps {
		return steps, fmt.Errorf("exceeded maximum redirect steps (%d)", config.MaxSteps)
	}

	return steps, nil
}

func fetchCaptcha(client *http.Client, cookies []*http.Cookie, htmlBody string, refererURL string) (*CaptchaData, error) {
	// Extract CSRF token from HTML or use existing one
	csrfToken := extractCSRFToken(htmlBody)
	if csrfToken == "" && currentCaptcha != nil {
		// Reuse existing CSRF token if available
		csrfToken = currentCaptcha.CSRFToken
	}
	if csrfToken == "" {
		return nil, fmt.Errorf("could not extract CSRF token from HTML")
	}

	// Create captcha request
	captchaURL := "https://sso.my.gov.ir/client/v1/captcha"
	req, err := http.NewRequest("GET", captchaURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating captcha request: %w", err)
	}

	// Set headers based on the browser request
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,fa;q=0.8,ar;q=0.7")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("Referer", refererURL)
	req.Header.Set("sec-ch-ua", `"Chromium";v="142", "Google Chrome";v="142", "Not_A Brand";v="99"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"macOS"`)
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "same-origin")
	req.Header.Set("x-csrf-token", csrfToken)

	// Basic auth header
	authHeader := "Basic cHdhLnVzZXI6MDFhbnNraXludXd2czRzYnRvamE="
	req.Header.Set("Authorization", authHeader)

	// Add cookies
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	// Make request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making captcha request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bodyPreview, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("captcha request failed with status: %s, body preview: %s", resp.Status, string(bodyPreview[:min(200, len(bodyPreview))]))
	}

	// Check if response is gzip-compressed
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error creating gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	// Read response
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("error reading captcha response: %w", err)
	}

	// Parse JSON response
	var captchaResp struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}

	err = json.Unmarshal(body, &captchaResp)
	if err != nil {
		return nil, fmt.Errorf("error parsing captcha JSON: %w", err)
	}

	return &CaptchaData{
		Key:        captchaResp.Key,
		ImageData:  captchaResp.Value,
		CSRFToken:  csrfToken,
		AuthHeader: authHeader,
	}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func extractCSRFToken(html string) string {
	// Try multiple patterns to find CSRF token
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

func saveCaptchaImage(dataURL string, filename string) error {
	// Remove the data URL prefix (e.g., "data:image/png;base64,")
	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid data URL format")
	}

	// Decode base64
	imageData, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("error decoding base64: %w", err)
	}

	// Write to file
	err = os.WriteFile(filename, imageData, 0644)
	if err != nil {
		return fmt.Errorf("error writing file: %w", err)
	}

	return nil
}

func generateMarkdownFile(outputDir string, step RedirectStep) error {
	filename := fmt.Sprintf("%s/step_%02d.md", outputDir, step.StepNumber)

	var content strings.Builder

	content.WriteString(fmt.Sprintf("# Step %d: %s\n\n", step.StepNumber, step.Status))
	content.WriteString(fmt.Sprintf("**Timestamp:** %s\n\n", step.Timestamp.Format(time.RFC3339)))
	content.WriteString(fmt.Sprintf("**URL:** `%s`\n\n", step.URL))

	// Request Section
	content.WriteString("## Request\n\n")
	content.WriteString(fmt.Sprintf("**Method:** %s\n\n", step.Method))

	content.WriteString("### Request Headers\n\n")
	content.WriteString("```http\n")
	for key, values := range step.RequestHeaders {
		for _, value := range values {
			content.WriteString(fmt.Sprintf("%s: %s\n", key, value))
		}
	}
	content.WriteString("```\n\n")

	// Response Section
	content.WriteString("## Response\n\n")
	content.WriteString(fmt.Sprintf("**Status Code:** %d\n\n", step.StatusCode))
	content.WriteString(fmt.Sprintf("**Status:** %s\n\n", step.Status))

	if step.RedirectLocation != "" {
		content.WriteString(fmt.Sprintf("**Redirect Location:** `%s`\n\n", step.RedirectLocation))
	}

	content.WriteString("### Response Headers\n\n")
	content.WriteString("```http\n")
	for key, values := range step.ResponseHeaders {
		for _, value := range values {
			content.WriteString(fmt.Sprintf("%s: %s\n", key, value))
		}
	}
	content.WriteString("```\n\n")

	// Captcha Section (if available)
	if step.CaptchaData != nil {
		content.WriteString("### Captcha\n\n")
		content.WriteString(fmt.Sprintf("**Captcha Key:** `%s`\n\n", step.CaptchaData.Key))
		content.WriteString(fmt.Sprintf("**CSRF Token:** `%s`\n\n", step.CaptchaData.CSRFToken))
		content.WriteString(fmt.Sprintf("**Auth Header:** `%s`\n\n", step.CaptchaData.AuthHeader))

		// Save captcha image
		captchaFilename := fmt.Sprintf("%s/step_%02d_captcha.png", outputDir, step.StepNumber)
		err := saveCaptchaImage(step.CaptchaData.ImageData, captchaFilename)
		if err == nil {
			content.WriteString(fmt.Sprintf("**Captcha Image:** ![captcha](step_%02d_captcha.png)\n\n", step.StepNumber))
			content.WriteString(fmt.Sprintf("**Base64 Data (first 100 chars):** `%s...`\n\n", step.CaptchaData.ImageData[:100]))
		} else {
			content.WriteString(fmt.Sprintf("**Error saving captcha:** %v\n\n", err))
		}
	}

	content.WriteString("### Response Body\n\n")
	if len(step.ResponseBody) > 0 {
		body := step.ResponseBody

		// Try to detect content type
		contentType := step.ResponseHeaders.Get("Content-Type")
		if strings.Contains(contentType, "html") {
			content.WriteString("```html\n")
		} else if strings.Contains(contentType, "json") {
			content.WriteString("```json\n")
		} else if strings.Contains(contentType, "xml") {
			content.WriteString("```xml\n")
		} else {
			content.WriteString("```\n")
		}
		content.WriteString(body)
		content.WriteString("\n```\n\n")
	} else {
		content.WriteString("*(empty)*\n\n")
	}

	// Navigation
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

func generateSummaryFile(outputDir string, steps []RedirectStep) error {
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
