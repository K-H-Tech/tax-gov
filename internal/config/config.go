package config

import (
	"log/slog"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all application configuration.
type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	Tracker TrackerConfig `mapstructure:"tracker"`
	SSO     SSOConfig     `mapstructure:"sso"`
	Tax     TaxConfig     `mapstructure:"tax"`
	Log     LogConfig     `mapstructure:"log"`
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
}

// TrackerConfig holds redirect tracker configuration.
type TrackerConfig struct {
	Timeout         time.Duration `mapstructure:"timeout"`
	MaxSteps        int           `mapstructure:"max_steps"`
	VerifyTLS       bool          `mapstructure:"verify_tls"`
	SaveCookies     bool          `mapstructure:"save_cookies"`
	FollowRedirects bool          `mapstructure:"follow_redirects"`
	OutputDir       string        `mapstructure:"output_dir"`
}

// SSOConfig holds SSO service configuration.
type SSOConfig struct {
	BaseURL      string `mapstructure:"base_url"`
	AuthURL      string `mapstructure:"auth_url"`
	LoginURL     string `mapstructure:"login_url"`
	CaptchaURL   string `mapstructure:"captcha_url"`
	SendOTPURL   string `mapstructure:"send_otp_url"`
	VerifyOTPURL string `mapstructure:"verify_otp_url"`
	ClientID     string `mapstructure:"client_id"`
	RedirectURI  string `mapstructure:"redirect_uri"`
	AuthHeader   string `mapstructure:"auth_header"`
}

// TaxConfig holds tax service configuration.
type TaxConfig struct {
	DashboardURL    string `mapstructure:"dashboard_url"`
	RegistrationURL string `mapstructure:"registration_url"`
}

// LogConfig holds logging configuration.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
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
		viper.AddConfigPath("$HOME/.taxgov")
		viper.AddConfigPath("/etc/taxgov")
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

	return &cfg, nil
}

// setDefaults sets default configuration values.
func setDefaults() {
	// Server defaults
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.read_timeout", "5s")
	viper.SetDefault("server.write_timeout", "10s")
	viper.SetDefault("server.idle_timeout", "120s")

	// Tracker defaults
	viper.SetDefault("tracker.timeout", "60s")
	viper.SetDefault("tracker.max_steps", 20)
	viper.SetDefault("tracker.verify_tls", true)
	viper.SetDefault("tracker.save_cookies", true)
	viper.SetDefault("tracker.follow_redirects", true)
	viper.SetDefault("tracker.output_dir", "redirect_steps")

	// SSO defaults
	viper.SetDefault("sso.base_url", "https://sso.my.gov.ir")
	viper.SetDefault("sso.auth_url", "https://sso.my.gov.ir/oauth2/authorize")
	viper.SetDefault("sso.login_url", "https://sso.my.gov.ir/login")
	viper.SetDefault("sso.captcha_url", "https://sso.my.gov.ir/client/v1/captcha")
	viper.SetDefault("sso.send_otp_url", "https://sso.my.gov.ir/sendOtp")
	viper.SetDefault("sso.verify_otp_url", "https://sso.my.gov.ir/login_otp")
	viper.SetDefault("sso.client_id", "my.tax")
	viper.SetDefault("sso.redirect_uri", "https://my.tax.gov.ir/myiran/sso")
	viper.SetDefault("sso.auth_header", "Basic cHdhLnVzZXI6MDFhbnNraXludXd2czRzYnRvamE=")

	// Tax defaults
	viper.SetDefault("tax.dashboard_url", "https://my.tax.gov.ir/Page/Dashboard")
	viper.SetDefault("tax.registration_url", "https://my.tax.gov.ir/Page/NewRegistration/")

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

// GetStartURL returns the OAuth2 authorization URL with parameters.
func (c *Config) GetStartURL() string {
	return c.SSO.AuthURL + "?response_type=code&scope=openid%20profile&client_id=" +
		c.SSO.ClientID + "&state=state1&redirect_uri=" + c.SSO.RedirectURI
}
