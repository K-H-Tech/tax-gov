# Security Standards

## Input Validation

### All External Input Must Be Validated
- HTTP request parameters
- User-provided data
- File uploads
- Configuration from environment

### Validation Rules
```go
// Validate early, fail fast
func HandleRequest(w http.ResponseWriter, r *http.Request) {
    userID := r.URL.Query().Get("user_id")
    if userID == "" {
        http.Error(w, "user_id required", http.StatusBadRequest)
        return
    }

    id, err := strconv.Atoi(userID)
    if err != nil || id <= 0 {
        http.Error(w, "invalid user_id", http.StatusBadRequest)
        return
    }
    // proceed with validated input
}
```

## Injection Prevention

### SQL Injection
```go
// ALWAYS use parameterized queries
// GOOD
db.Query("SELECT * FROM users WHERE id = $1", userID)

// NEVER concatenate user input
// BAD - vulnerable to SQL injection
db.Query("SELECT * FROM users WHERE id = " + userID)
```

### Command Injection
```go
// NEVER pass user input to shell commands
// If unavoidable, use exec.Command with separate args
cmd := exec.Command("program", "--flag", sanitizedInput)

// NEVER use shell=true or string concatenation
// BAD
exec.Command("sh", "-c", "program " + userInput)
```

### Path Traversal
```go
// Validate file paths
func SafeFilePath(basePath, userInput string) (string, error) {
    cleanPath := filepath.Clean(userInput)
    fullPath := filepath.Join(basePath, cleanPath)

    // Ensure result is within basePath
    if !strings.HasPrefix(fullPath, basePath) {
        return "", errors.New("invalid path: directory traversal detected")
    }
    return fullPath, nil
}
```

## Secret Management

### Never Hardcode Secrets
```go
// BAD
const apiKey = "sk-1234567890abcdef"

// GOOD - use environment variables
apiKey := os.Getenv("API_KEY")
if apiKey == "" {
    return errors.New("API_KEY environment variable required")
}
```

### Secrets Checklist
- API keys, tokens, passwords: environment variables only
- Database credentials: environment variables or secret manager
- Never commit `.env` files with real secrets
- Use `.env.example` for documentation

### Logging Secrets
```go
// NEVER log sensitive data
// BAD
log.Printf("Authenticating with token: %s", token)

// GOOD - log without revealing secrets
log.Printf("Authenticating user: %s", userID)
```

## HTTPS/TLS

### Enforce HTTPS
- All production traffic over HTTPS
- Redirect HTTP to HTTPS
- Use `http.Server` with TLS config

### TLS Configuration
```go
server := &http.Server{
    TLSConfig: &tls.Config{
        MinVersion: tls.VersionTLS12,
        // Prefer server cipher suites
        PreferServerCipherSuites: true,
    },
}
```

## Authentication & Authorization

### Password Handling
```go
// Use bcrypt for password hashing
import "golang.org/x/crypto/bcrypt"

hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
err := bcrypt.CompareHashAndPassword(hash, []byte(password))
```

### Session Management
- Use secure, random session IDs
- Set appropriate cookie flags: `Secure`, `HttpOnly`, `SameSite`
- Implement session timeout
- Invalidate sessions on logout

### Authorization Checks
```go
// Check authorization on every request
func (h *Handler) UpdateResource(w http.ResponseWriter, r *http.Request) {
    user := auth.UserFromContext(r.Context())
    resource := getResource(r)

    if !user.CanEdit(resource) {
        http.Error(w, "forbidden", http.StatusForbidden)
        return
    }
    // proceed
}
```

## OWASP Top 10 Awareness

### Key Vulnerabilities to Prevent
1. **Injection** - Parameterized queries, input validation
2. **Broken Authentication** - Strong password policies, MFA
3. **Sensitive Data Exposure** - Encrypt at rest and in transit
4. **XML External Entities** - Disable DTD processing
5. **Broken Access Control** - Deny by default, check on every request
6. **Security Misconfiguration** - Minimal permissions, update dependencies
7. **XSS** - Escape output, Content-Security-Policy headers
8. **Insecure Deserialization** - Validate before deserializing
9. **Using Components with Known Vulnerabilities** - Run `govulncheck`
10. **Insufficient Logging** - Log security events, monitor anomalies

## Security Headers

```go
func SecurityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-XSS-Protection", "1; mode=block")
        w.Header().Set("Content-Security-Policy", "default-src 'self'")
        next.ServeHTTP(w, r)
    })
}
```
