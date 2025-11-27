# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A Go-based HTTP redirect tracker with a web UI for tracking OAuth/SSO authentication flows to Iranian government tax services (my.tax.gov.ir via sso.my.gov.ir). The application simulates browser-like behavior to track redirect chains, handle captcha verification, and perform OTP-based authentication.

## Running the Application

```bash
# Start the web server on port 8080
go run redirect_tracker.go

# Open in browser
http://localhost:8080
```

## Architecture

### Backend (redirect_tracker.go)
- HTTP server with REST API endpoints for authentication flow:
  - `/api/start-tracker` - Initiates redirect tracking and fetches initial captcha
  - `/api/refresh-captcha` - Fetches new captcha using existing session
  - `/api/send-otp` - Sends OTP request with mobile + captcha
  - `/api/verify-otp` - Verifies OTP code and completes login
  - `/api/access-dashboard` - Accesses the tax dashboard post-authentication
  - `/api/start-tax-file` - Initiates tax file registration
- Maintains session state via global `currentClient`, `currentCookies`, and `currentCaptcha` variables
- Generates markdown documentation of redirect steps in `redirect_steps/` directory

### Frontend (index.html)
- Single-page RTL Persian UI with multi-step login flow
- Steps: Load captcha → Enter mobile + captcha → Receive OTP → Verify OTP → Access dashboard → Start tax file

### Browser Helper (browser_capture.js)
- Console script for capturing redirect data directly from browser DevTools
- Alternative approach when programmatic tracking fails

## Key Implementation Details

- HTTP client configured to NOT auto-follow redirects (`http.ErrUseLastResponse`)
- Cookies are maintained across requests to simulate browser sessions
- CSRF tokens extracted from HTML responses via regex patterns
- Captcha fetched from `/client/v1/captcha` endpoint with Basic auth header
- Response bodies may be gzip-compressed and need decompression

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
