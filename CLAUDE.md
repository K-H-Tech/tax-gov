# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A Go-based HTTP redirect tracker with a web UI for tracking OAuth/SSO authentication flows to Iranian government tax services (my.tax.gov.ir via sso.my.gov.ir). The application simulates browser-like behavior to track redirect chains, handle captcha verification, and perform OTP-based authentication.

## Running the Application

```bash
# Build the CLI
go build -o taxgov ./cmd/taxgov

# Start the web server (default port 8080)
./taxgov serve

# Start on a custom port
./taxgov serve --port 3000

# Run redirect tracking from CLI
./taxgov track

# Show version
./taxgov version

# Get help
./taxgov --help
```

### Configuration

Copy `config.example.yaml` to `config.yaml` and update with your values:
```bash
cp config.example.yaml config.yaml
```

Configuration can also be set via environment variables with `TAXGOV_` prefix:
```bash
TAXGOV_SERVER_PORT=3000 ./taxgov serve
```

## Project Structure

```
tax-gov/
├── cmd/taxgov/           # CLI entry points (Cobra)
│   ├── main.go           # Main entry point
│   ├── root.go           # Root command and config init
│   ├── serve.go          # Web server command
│   └── track.go          # CLI tracking command
├── internal/
│   ├── client/           # HTTP client factory
│   │   └── http.go       # Client creation, headers helpers
│   ├── config/           # Configuration (Viper)
│   │   └── config.go     # Config structs and loading
│   ├── models/           # Shared data types
│   │   └── types.go      # RedirectStep, CaptchaData, etc.
│   ├── server/           # HTTP server
│   │   ├── handlers.go   # API endpoint handlers
│   │   ├── server.go     # Server lifecycle
│   │   └── session.go    # Session state management
│   └── tracker/          # Core tracking logic
│       ├── auth.go       # OTP/authentication operations
│       ├── captcha.go    # Captcha fetching
│       └── tracker.go    # Redirect chain tracking
├── web/                  # Static web assets
│   └── index.html        # Persian UI
├── .claude/              # Development rules
│   ├── agents/           # AI agent configurations
│   └── rules/            # Code quality rules
├── config.example.yaml   # Example configuration
├── go.mod                # Go module definition
└── CLAUDE.md             # This file
```

## Architecture

### CLI Layer (cmd/taxgov/)
- **Cobra** for command-line interface with `serve` and `track` subcommands
- **Viper** for configuration management (YAML files + environment variables)
- Structured logging via `log/slog`

### API Endpoints (internal/server/)
- `GET /` - Serves the web UI
- `POST /api/start-tracker` - Initiates redirect tracking and fetches captcha
- `POST /api/refresh-captcha` - Fetches new captcha using existing session
- `POST /api/send-otp` - Sends OTP request with mobile + captcha
- `POST /api/verify-otp` - Verifies OTP code and completes login
- `POST /api/access-dashboard` - Accesses the tax dashboard
- `POST /api/start-tax-file` - Initiates tax file registration

### Core Components
- **Tracker** (`internal/tracker/`) - Follows redirect chains, extracts captcha/CSRF tokens
- **Session** (`internal/server/session.go`) - Thread-safe session state with `sync.RWMutex`
- **Client** (`internal/client/`) - HTTP client factory with no-redirect policy

### Frontend (web/index.html)
- Single-page RTL Persian UI with multi-step login flow
- Steps: Load captcha → Enter mobile + captcha → Receive OTP → Verify OTP → Access dashboard

## Key Implementation Details

- HTTP client configured to NOT auto-follow redirects (`http.ErrUseLastResponse`)
- Cookies maintained across requests via Session struct
- CSRF tokens extracted from HTML via regex patterns
- Captcha fetched from `/client/v1/captcha` with Basic auth header
- Response bodies may be gzip-compressed
- Dependency injection pattern for testability
- All handlers receive dependencies through Handler struct

## Development Standards

This project follows a hierarchical rule system. CLAUDE.md provides project context, detailed standards are in `.claude/` subdirectories.

### Rule Index

| Category | Location | When to Apply |
|----------|----------|---------------|
| **Go Engineering** | `.claude/agents/go-engineer.md` | All Go code changes |
| **Code Quality** | `.claude/rules/code-quality.md` | All code changes |
| **Security** | `.claude/rules/security.md` | Auth, input handling, secrets |
| **Testing** | `.claude/rules/testing.md` | Writing/modifying tests |
| **API Design** | `.claude/rules/api-design.md` | HTTP endpoints, JSON responses |

### Before Implementation

1. Review this file for project architecture
2. Check `.claude/agents/go-engineer.md` for Go standards
3. Apply relevant rules from `.claude/rules/`
4. Run `go test -race` and linters before committing

## Implementation Checklist

Before writing or modifying code:

- [ ] Understand the relevant architecture section above
- [ ] Review applicable rules in `.claude/agents/` or `.claude/rules/`
- [ ] For Go: Apply go-engineer agent standards
- [ ] Run tests and linters before committing
