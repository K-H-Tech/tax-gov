# Code Quality Standards

## Commit Messages

### Format
```
<type>(<scope>): <subject>

<body>

<footer>
```

### Types
- `feat`: New feature
- `fix`: Bug fix
- `refactor`: Code change that neither fixes a bug nor adds a feature
- `docs`: Documentation only
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

### Rules
- Subject line: max 50 characters, imperative mood ("Add feature" not "Added feature")
- Body: wrap at 72 characters, explain what and why (not how)
- Reference issues: `Fixes #123` or `Relates to #456`

## Pull Requests

### Description Requirements
- Summary of changes (what and why)
- Testing done
- Screenshots for UI changes
- Breaking changes highlighted
- Related issues linked

### PR Size
- Prefer small, focused PRs (< 400 lines)
- Split large changes into logical commits
- One concern per PR

## Code Review Expectations

### Author Responsibilities
- Self-review before requesting review
- Respond to all comments
- Keep PR updated with main branch

### Reviewer Responsibilities
- Review within 24 hours
- Be constructive, not critical
- Approve when "good enough", not "perfect"
- Use suggestions for minor changes

## Documentation

### When to Document
- Public APIs (exported functions, types)
- Complex algorithms
- Non-obvious business logic
- Configuration options

### Format
```go
// FunctionName does X.
// It handles Y edge case by Z.
// Returns error if condition is not met.
func FunctionName(...) error {
```

## Error Messages

### Good Error Messages
- Include context (what failed)
- Include relevant values (IDs, names)
- Suggest resolution when possible

```go
// GOOD
return fmt.Errorf("failed to fetch user %d from database: %w", userID, err)

// BAD
return fmt.Errorf("database error")
```

## Code Organization

### File Naming
- Go files: `snake_case.go`
- Test files: `*_test.go`
- One primary type per file when practical

### Package Structure
- Keep packages focused and small
- Avoid circular dependencies
- Internal packages for implementation details

## Constants and Configuration

### Naming
```go
const (
    DefaultTimeout = 30 * time.Second
    MaxRetries     = 3
)
```

### Configuration
- Use environment variables for deployment config
- Use flags for CLI options
- Document all configuration options
