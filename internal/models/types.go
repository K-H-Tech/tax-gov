package models

import (
	"net/http"
	"time"
)

// RedirectStep represents a single step in the redirect chain.
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

// CaptchaData holds captcha information from the SSO login page.
// WARNING: AuthHeader contains sensitive credentials - never expose in logs or API responses.
type CaptchaData struct {
	Key        string `json:"key"`
	ImageData  string `json:"imageData"`
	CSRFToken  string `json:"csrfToken"`
	AuthHeader string `json:"-"` // Excluded from JSON to prevent credential exposure
}

// LoginResult represents the result of a login operation.
type LoginResult struct {
	Message string
	Data    map[string]any
}

// OTPRequest represents the request to send an OTP.
type OTPRequest struct {
	Mobile      string `json:"mobile"`
	CaptchaCode string `json:"captchaCode"`
	CaptchaKey  string `json:"captchaKey"`
	CSRFToken   string `json:"csrfToken"`
}

// OTPVerifyRequest represents the request to verify an OTP.
type OTPVerifyRequest struct {
	Mobile    string `json:"mobile"`
	OTPCode   string `json:"otpCode"`
	CSRFToken string `json:"csrfToken"`
}

// APIResponse is a generic API response structure.
type APIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// CaptchaResponse is the response for captcha-related endpoints.
type CaptchaResponse struct {
	Success bool `json:"success"`
	Captcha *struct {
		Key       string `json:"key"`
		ImageData string `json:"imageData"`
		CSRFToken string `json:"csrfToken"`
	} `json:"captcha,omitempty"`
	Error string `json:"error,omitempty"`
}

// TaxRegistrationRequest represents the tax file registration request.
type TaxRegistrationRequest struct {
	PostalCode   string `json:"postalCode"`
	BusinessName string `json:"businessName"`
	Type         string `json:"type"` // "Single" or "Company"
}

// DropdownOption represents a single option in a dropdown.
type DropdownOption struct {
	Value   string `json:"value" yaml:"value"`
	Label   string `json:"label" yaml:"label"`
	LabelFa string `json:"labelFa" yaml:"label_fa"`
}

// BasicInfoRequest represents Step 2 basic information form submission.
type BasicInfoRequest struct {
	RegistrationReason   string `json:"registrationReason"`   // دلیل ثبت‌نام
	ActivityType         string `json:"activityType"`         // نوع فعالیت
	StartDate            string `json:"startDate"`            // تاریخ شروع فعالیت
	UnitTitle            string `json:"unitTitle"`            // عنوان واحد
	EightCategoryJob     string `json:"eightCategoryJob"`     // مشاغل هشتگانه
	IndividualJob        string `json:"individualJob"`        // مشاغل انفرادی
	ProfessionalGuild    string `json:"professionalGuild"`    // مجامع صنفی
	ProfessionalAssembly string `json:"professionalAssembly"` // مجامع صنفی (dropdown)
	GuildUnion           string `json:"guildUnion"`           // اتحادیه صنفی
	BusinessLicense      string `json:"businessLicense"`      // پروانه کسب
	OwnershipType        string `json:"ownershipType"`        // نوع مالکیت
	Website              string `json:"website,omitempty"`    // پایگاه اینترنتی (optional)
}
