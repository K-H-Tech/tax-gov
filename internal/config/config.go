package config

import (
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all application configuration.
type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	HTTP        HTTPConfig        `mapstructure:"http"`
	Services    ServicesConfig    `mapstructure:"services"`
	Log         LogConfig         `mapstructure:"log"`
	FormOptions FormOptionsConfig `mapstructure:"form_options"`
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
}

// HTTPConfig holds HTTP client configuration.
type HTTPConfig struct {
	Timeout         time.Duration `mapstructure:"timeout"`
	MaxRedirects    int           `mapstructure:"max_redirects"`
	VerifyTLS       bool          `mapstructure:"verify_tls"`
	SaveCookies     bool          `mapstructure:"save_cookies"`
	FollowRedirects bool          `mapstructure:"follow_redirects"`
	OutputDir       string        `mapstructure:"output_dir"`
}

// ServicesConfig holds configuration for all services.
type ServicesConfig struct {
	MyGovAuth MyGovAuthConfig `mapstructure:"mygovauth"`
	MyTax     MyTaxConfig     `mapstructure:"mytax"`
	Enamad    EnamadConfig    `mapstructure:"enamad"`
	Mojavez   MojavezConfig   `mapstructure:"mojavez"`
}

// MyGovAuthConfig holds SSO/government authentication service configuration.
type MyGovAuthConfig struct {
	BaseURL      string `mapstructure:"base_url"`
	AuthURL      string `mapstructure:"auth_url"`
	LoginURL     string `mapstructure:"login_url"`
	LoggedInURL  string `mapstructure:"logged_in_url"`
	CaptchaURL   string `mapstructure:"captcha_url"`
	SendOTPURL   string `mapstructure:"send_otp_url"`
	VerifyOTPURL string `mapstructure:"verify_otp_url"`
	AuthHeader   string `mapstructure:"auth_header"`
}

// MyTaxConfig holds tax service configuration.
type MyTaxConfig struct {
	BaseURL         string `mapstructure:"base_url"`
	CallbackURL     string `mapstructure:"callback_url"`
	DashboardURL    string `mapstructure:"dashboard_url"`
	RegistrationURL string `mapstructure:"registration_url"`
	BasicInfoURL    string `mapstructure:"basic_info_url"`
	ClientID        string `mapstructure:"client_id"`
	RedirectURI     string `mapstructure:"redirect_uri"`
}

// EnamadConfig holds Enamad service configuration.
type EnamadConfig struct {
	BaseURL     string `mapstructure:"base_url"`
	CallbackURL string `mapstructure:"callback_url"`
	ClientID    string `mapstructure:"client_id"`
	RedirectURI string `mapstructure:"redirect_uri"`
}

// MojavezConfig holds Mojavez service configuration.
type MojavezConfig struct {
	BaseURL     string `mapstructure:"base_url"`
	CallbackURL string `mapstructure:"callback_url"`
	ClientID    string `mapstructure:"client_id"`
	RedirectURI string `mapstructure:"redirect_uri"`
}

// LogConfig holds logging configuration.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// DropdownOption represents a single option in a dropdown.
type DropdownOption struct {
	Value   string `mapstructure:"value" json:"value" yaml:"value"`
	Label   string `mapstructure:"label" json:"label" yaml:"label"`
	LabelFa string `mapstructure:"label_fa" json:"labelFa" yaml:"label_fa"`
}

// FormOptionsConfig holds dropdown options for forms.
type FormOptionsConfig struct {
	RegistrationReasons    []DropdownOption `mapstructure:"registration_reasons"`
	ActivityTypes          []DropdownOption `mapstructure:"activity_types"`
	EightCategoryJobs      []DropdownOption `mapstructure:"eight_category_jobs"`
	IndividualJobs         []DropdownOption `mapstructure:"individual_jobs"`
	ProfessionalGuilds     []DropdownOption `mapstructure:"professional_guilds"`
	ProfessionalAssemblies []DropdownOption `mapstructure:"professional_assemblies"`
	GuildUnions            []DropdownOption `mapstructure:"guild_unions"`
	BusinessLicenses       []DropdownOption `mapstructure:"business_licenses"`
	OwnershipTypes         []DropdownOption `mapstructure:"ownership_types"`
}

// Load reads configuration from file and environment.
func Load(configPath string) (*Config, error) {
	setDefaults()

	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("$HOME/.auto-tax-gov")
		viper.AddConfigPath("/etc/auto-tax-gov")
	}

	// Read config file (optional - don't fail if not found)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
		slog.Debug("no config file found, using defaults")
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Warn if SSO auth_header is not configured
	if cfg.Services.MyGovAuth.AuthHeader == "" {
		slog.Warn("SSO auth_header not configured - set via config file or AUTO_TAX_GOV_SERVICES_MYGOVAUTH_AUTH_HEADER env var")
	}

	return &cfg, nil
}

// setDefaults sets default configuration values.
func setDefaults() {
	// Server defaults
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.read_timeout", "5s")
	viper.SetDefault("server.write_timeout", "10s")
	viper.SetDefault("server.idle_timeout", "120s")

	// HTTP client defaults
	viper.SetDefault("http.timeout", "60s")
	viper.SetDefault("http.max_redirects", 20)
	viper.SetDefault("http.verify_tls", true)
	viper.SetDefault("http.save_cookies", true)
	viper.SetDefault("http.follow_redirects", true)
	viper.SetDefault("http.output_dir", "redirect_steps")

	// MyGovAuth service defaults (SSO)
	viper.SetDefault("services.mygovauth.base_url", "https://sso.my.gov.ir")
	viper.SetDefault("services.mygovauth.auth_url", "https://sso.my.gov.ir/oauth2/authorize")
	viper.SetDefault("services.mygovauth.login_url", "https://sso.my.gov.ir/login")
	viper.SetDefault("services.mygovauth.logged_in_url", "https://sso.my.gov.ir/logged-in")
	viper.SetDefault("services.mygovauth.captcha_url", "https://sso.my.gov.ir/client/v1/captcha")
	viper.SetDefault("services.mygovauth.send_otp_url", "https://sso.my.gov.ir/sendOtp")
	viper.SetDefault("services.mygovauth.verify_otp_url", "https://sso.my.gov.ir/login_otp")
	viper.SetDefault("services.mygovauth.auth_header", "") // Must be set via config file or AUTO_TAX_GOV_SERVICES_MYGOVAUTH_AUTH_HEADER env var

	// MyTax service defaults
	viper.SetDefault("services.mytax.base_url", "https://my.tax.gov.ir")
	viper.SetDefault("services.mytax.callback_url", "https://my.tax.gov.ir/myiran/sso")
	viper.SetDefault("services.mytax.dashboard_url", "https://my.tax.gov.ir/Page/Dashboard")
	viper.SetDefault("services.mytax.registration_url", "https://my.tax.gov.ir/Page/NewRegistration/")
	viper.SetDefault("services.mytax.basic_info_url", "https://my.tax.gov.ir/Page/BasicInfo/")
	viper.SetDefault("services.mytax.client_id", "my.tax")
	viper.SetDefault("services.mytax.redirect_uri", "https://my.tax.gov.ir/myiran/sso")

	// Enamad service defaults
	viper.SetDefault("services.enamad.base_url", "https://reg2.enamad.ir")
	viper.SetDefault("services.enamad.callback_url", "https://reg2.enamad.ir/callback")
	viper.SetDefault("services.enamad.client_id", "enamad")
	viper.SetDefault("services.enamad.redirect_uri", "https://reg2.enamad.ir/callback")

	// Mojavez service defaults
	viper.SetDefault("services.mojavez.base_url", "https://mojavez.ir")
	viper.SetDefault("services.mojavez.callback_url", "https://mojavez.ir/callback")
	viper.SetDefault("services.mojavez.client_id", "mojavez")
	viper.SetDefault("services.mojavez.redirect_uri", "https://mojavez.ir/callback")

	// Log defaults
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "text")
}

// GetLogLevel returns the slog level from config.
func (c *Config) GetLogLevel() slog.Level {
	switch strings.ToLower(c.Log.Level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// GetMyTaxAuthURL returns the OAuth2 authorization URL for MyTax service.
func (c *Config) GetMyTaxAuthURL() string {
	u, _ := url.Parse(c.Services.MyGovAuth.AuthURL)
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("scope", "openid profile")
	q.Set("client_id", c.Services.MyTax.ClientID)
	q.Set("state", "state1")
	q.Set("redirect_uri", c.Services.MyTax.RedirectURI)
	u.RawQuery = q.Encode()
	return u.String()
}

// GetEnamadAuthURL returns the OAuth2 authorization URL for Enamad service.
func (c *Config) GetEnamadAuthURL() string {
	u, _ := url.Parse(c.Services.MyGovAuth.AuthURL)
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("scope", "openid profile")
	q.Set("client_id", c.Services.Enamad.ClientID)
	q.Set("state", "state1")
	q.Set("redirect_uri", c.Services.Enamad.RedirectURI)
	u.RawQuery = q.Encode()
	return u.String()
}

// GetMojavezAuthURL returns the OAuth2 authorization URL for Mojavez service.
func (c *Config) GetMojavezAuthURL() string {
	u, _ := url.Parse(c.Services.MyGovAuth.AuthURL)
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("scope", "openid profile")
	q.Set("client_id", c.Services.Mojavez.ClientID)
	q.Set("state", "state1")
	q.Set("redirect_uri", c.Services.Mojavez.RedirectURI)
	u.RawQuery = q.Encode()
	return u.String()
}
