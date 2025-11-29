package taxregister

// RegistrationRequest contains data for creating a new tax registration.
type RegistrationRequest struct {
	Type         string `json:"type"`         // "Multiple" or "Single"
	PostalCode   string `json:"postalCode"`   // 10-digit postal code
	BusinessName string `json:"businessName"` // URL-encoded Persian business name
}

// RegistrationResponse is the response from NewRegistration API.
type RegistrationResponse struct {
	Success bool   `json:"isSuccess"`
	Message string `json:"msg"` // Contains GUID on success
	GUID    string `json:"guid,omitempty"`
}

// SSOResponse is the response from SSODoc API.
type SSOResponse struct {
	IsLogin bool   `json:"isLogin"`
	URL     string `json:"url"` // Full URL to TokenLoginProcessWithSignout
}

// HomePageData contains parsed data from the HomePage.
type HomePageData struct {
	GUID          string            `json:"guid"`
	Status        string            `json:"status"`
	StatusMessage string            `json:"statusMessage"`
	Sections      []string          `json:"sections"` // List of completed sections
	RawHTML       string            `json:"-"`        // Raw HTML for debugging
	FormData      map[string]string `json:"formData"` // Any hidden form fields
}

// PublicDataForm contains the ASP.NET form state and fields from PublicData page.
type PublicDataForm struct {
	// ASP.NET hidden fields
	ViewState          string `json:"viewState"`
	ViewStateGenerator string `json:"viewStateGenerator"`
	EventValidation    string `json:"eventValidation"`

	// Registration GUID
	GUID string `json:"guid"`

	// Form field values (current state)
	Fields map[string]string `json:"fields"`

	// Available dropdown options
	DropdownOptions map[string][]DropdownOption `json:"dropdownOptions"`
}

// DropdownOption represents a single option in a dropdown.
type DropdownOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// PublicDataRequest contains form data for submitting the PublicData form.
type PublicDataRequest struct {
	// Required fields from step_09.raw
	RegistrationCause   string `json:"registrationCause"`   // DDLNewRegistrationCause
	IsTejari            string `json:"isTejari"`            // DDLIsTejari
	FinancialStartDate  string `json:"financialStartDate"`  // TextBoxFinantialStartDate (Jalali)
	BusinessName        string `json:"businessName"`        // TextBoxPDName
	GroupOneType        string `json:"groupOneType"`        // DDLGroupOneTypes
	LegalType           string `json:"legalType"`           // DDLPDLegalType
	NewLegalGroup       string `json:"newLegalGroup"`       // DDLPDNewLegalGroup
	NewLegalType        string `json:"newLegalType"`        // DDLPDNewLegalType
	HasJobLicense       string `json:"hasJobLicense"`       // DDLHasJobLicence
	Ownership           string `json:"ownership"`           // DDLPDOwnership
	FinancialDayStart   string `json:"financialDayStart"`   // DDLFinantialDayStart
	FinancialMonthStart string `json:"financialMonthStart"` // DDLFinantialMonthStart

	// Optional fields
	Website string `json:"website,omitempty"` // TextBoxAddressWebsite
	Email   string `json:"email,omitempty"`   // TextBoxAddressEmail
}

// AjaxPostbackRequest contains data for an AJAX partial postback.
type AjaxPostbackRequest struct {
	EventTarget string            // The control that triggered the postback (e.g., "ctl00$CPC$DDLPDNewLegalGroup")
	UpdatePanel string            // The update panel to refresh (e.g., "ctl00$CPC$UPGuildGroup")
	FormState   *PublicDataForm   // Current form state
	FieldValues map[string]string // Current field values to include
}

// AjaxPostbackResponse contains the result of an AJAX partial postback.
type AjaxPostbackResponse struct {
	// Updated ASP.NET state
	ViewState       string `json:"viewState"`
	EventValidation string `json:"eventValidation"`

	// Updated dropdown options (if any changed)
	UpdatedDropdowns map[string][]DropdownOption `json:"updatedDropdowns,omitempty"`

	// Raw response for debugging
	RawResponse string `json:"-"`
}

// RegistrationFlowRequest contains all data needed for the complete 11-step flow.
type RegistrationFlowRequest struct {
	// Step 1: New Registration
	Type         string `json:"type"`         // "Multiple" or "Single"
	PostalCode   string `json:"postalCode"`   // 10-digit postal code
	BusinessName string `json:"businessName"` // Business name

	// Steps 9-10: PublicData form submission
	PublicData *PublicDataRequest `json:"publicData,omitempty"`
}

// RegistrationFlowResponse contains the result of the complete registration flow.
type RegistrationFlowResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`

	// Registration details
	GUID         string `json:"guid,omitempty"`
	Status       string `json:"status,omitempty"`
	FinalPageURL string `json:"finalPageUrl,omitempty"`

	// Step-by-step results
	Steps []StepResult `json:"steps,omitempty"`
}

// StepResult contains the result of a single step in the flow.
type StepResult struct {
	Step    int    `json:"step"`
	Name    string `json:"name"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	URL     string `json:"url,omitempty"`
}
