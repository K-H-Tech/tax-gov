# API Design Standards

## RESTful Conventions

### Resource Naming
```
GET    /users           # List users
POST   /users           # Create user
GET    /users/{id}      # Get user
PUT    /users/{id}      # Update user (full)
PATCH  /users/{id}      # Update user (partial)
DELETE /users/{id}      # Delete user

# Nested resources
GET    /users/{id}/orders
POST   /users/{id}/orders
```

### Naming Rules
- Use nouns, not verbs (`/users` not `/getUsers`)
- Use plural forms (`/users` not `/user`)
- Use lowercase with hyphens (`/user-profiles` not `/userProfiles`)
- Avoid deep nesting (max 2 levels)

## HTTP Status Codes

### Success Codes
| Code | Meaning | When to Use |
|------|---------|-------------|
| 200 | OK | Successful GET, PUT, PATCH |
| 201 | Created | Successful POST that creates resource |
| 204 | No Content | Successful DELETE |

### Client Error Codes
| Code | Meaning | When to Use |
|------|---------|-------------|
| 400 | Bad Request | Invalid request body, missing required field |
| 401 | Unauthorized | Missing or invalid authentication |
| 403 | Forbidden | Authenticated but not authorized |
| 404 | Not Found | Resource doesn't exist |
| 409 | Conflict | Resource conflict (duplicate, version mismatch) |
| 422 | Unprocessable Entity | Valid syntax but semantic errors |

### Server Error Codes
| Code | Meaning | When to Use |
|------|---------|-------------|
| 500 | Internal Server Error | Unexpected server error |
| 502 | Bad Gateway | Upstream service error |
| 503 | Service Unavailable | Temporary unavailability |

## JSON Response Structure

### Success Response
```json
{
  "data": {
    "id": 123,
    "name": "Example",
    "createdAt": "2024-01-15T10:30:00Z"
  }
}
```

### List Response
```json
{
  "data": [
    {"id": 1, "name": "First"},
    {"id": 2, "name": "Second"}
  ],
  "meta": {
    "total": 100,
    "page": 1,
    "perPage": 20
  }
}
```

### Error Response
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid request data",
    "details": [
      {
        "field": "email",
        "message": "must be a valid email address"
      }
    ]
  }
}
```

## Go Implementation Patterns

### Handler Structure
```go
type Handler struct {
    store  Store
    logger *slog.Logger
}

func NewHandler(store Store, logger *slog.Logger) *Handler {
    return &Handler{store: store, logger: logger}
}

func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Parse path parameter
    idStr := r.PathValue("id") // Go 1.22+
    id, err := strconv.Atoi(idStr)
    if err != nil {
        h.writeError(w, http.StatusBadRequest, "INVALID_ID", "invalid user id")
        return
    }

    // Fetch resource
    user, err := h.store.GetUser(ctx, id)
    if errors.Is(err, ErrNotFound) {
        h.writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
        return
    }
    if err != nil {
        h.logger.Error("failed to get user", "error", err, "id", id)
        h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
        return
    }

    h.writeJSON(w, http.StatusOK, map[string]any{"data": user})
}
```

### JSON Helpers
```go
func (h *Handler) writeJSON(w http.ResponseWriter, status int, data any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(data)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, code, message string) {
    h.writeJSON(w, status, map[string]any{
        "error": map[string]any{
            "code":    code,
            "message": message,
        },
    })
}
```

## Request Validation

### Decode and Validate Pattern
```go
type CreateUserRequest struct {
    Email    string `json:"email"`
    Name     string `json:"name"`
    Password string `json:"password"`
}

func (r *CreateUserRequest) Validate() error {
    var errs []string

    if r.Email == "" {
        errs = append(errs, "email is required")
    } else if !isValidEmail(r.Email) {
        errs = append(errs, "email must be valid")
    }

    if r.Name == "" {
        errs = append(errs, "name is required")
    }

    if len(r.Password) < 8 {
        errs = append(errs, "password must be at least 8 characters")
    }

    if len(errs) > 0 {
        return &ValidationError{Errors: errs}
    }
    return nil
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
    var req CreateUserRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
        return
    }

    if err := req.Validate(); err != nil {
        h.writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
        return
    }

    // proceed with valid request
}
```

## Pagination

### Query Parameters
```
GET /users?page=2&per_page=20
GET /users?cursor=abc123&limit=20  # cursor-based
```

### Implementation
```go
type PaginationParams struct {
    Page    int
    PerPage int
}

func parsePagination(r *http.Request) PaginationParams {
    page, _ := strconv.Atoi(r.URL.Query().Get("page"))
    if page < 1 {
        page = 1
    }

    perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
    if perPage < 1 || perPage > 100 {
        perPage = 20
    }

    return PaginationParams{Page: page, PerPage: perPage}
}
```

## Timeouts and Context

### Always Use Context
```go
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Use context for database queries
    user, err := h.store.GetUser(ctx, id)

    // Use context for external API calls
    data, err := h.client.Fetch(ctx, url)
}
```

### Set Request Timeouts
```go
server := &http.Server{
    ReadTimeout:  5 * time.Second,
    WriteTimeout: 10 * time.Second,
    IdleTimeout:  120 * time.Second,
}
```

## Versioning

### URL Path Versioning (Recommended)
```
GET /api/v1/users
GET /api/v2/users
```

### Header Versioning (Alternative)
```
GET /api/users
Accept: application/vnd.myapp.v2+json
```

## CORS

```go
func CORS(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*") // Or specific origins
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }

        next.ServeHTTP(w, r)
    })
}
```
