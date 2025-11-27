# Redirect Tracker

A Go program that tracks HTTP redirects and generates detailed markdown documentation for each step in the redirect chain.

## Features

- Tracks all HTTP redirects from a starting URL
- Captures complete request and response details at each step
- Maintains cookies across redirects
- Generates individual markdown files for each step
- Creates a summary file with the complete redirect chain
- Configurable timeout, TLS verification, and max steps

## Usage

### Basic Usage

```bash
go run redirect_tracker.go
```

### Configuration

Edit the `Config` struct in the `main()` function to customize behavior:

```go
config := Config{
    Timeout:         60 * time.Second, // Request timeout
    MaxSteps:        20,                // Maximum redirect steps to follow
    FollowRedirects: true,              // Follow redirects
    VerifyTLS:       true,              // Verify TLS certificates
    SaveCookies:     true,              // Maintain cookies across requests
}
```

### Output

The program creates a `redirect_steps/` directory containing:

- `step_01.md`, `step_02.md`, etc. - Detailed info for each redirect step
- `summary.md` - Overview of the entire redirect chain

Each step file includes:
- Request URL and headers
- Response status, headers, and body
- Redirect location (if applicable)
- Navigation links to other steps

## Troubleshooting

### Connection Timeouts

If you're experiencing connection timeouts:

1. **Increase the timeout**: Modify the `Timeout` value in the config
2. **Check network connectivity**: Ensure you can reach the target domain
3. **Use a VPN**: Some servers may block requests from certain regions
4. **Disable TLS verification** (for testing only): Set `VerifyTLS: false`

### Testing with cURL

You can test the URL manually first:

```bash
# Test with verbose output
curl -v -L "https://sso.my.gov.ir/oauth2/authorize?response_type=code&scope=openid%20profile&client_id=my.tax&state=state1&redirect_uri=https://my.tax.gov.ir/myiran/sso"

# Test without following redirects
curl -v "https://sso.my.gov.ir/oauth2/authorize?response_type=code&scope=openid%20profile&client_id=my.tax&state=state1&redirect_uri=https://my.tax.gov.ir/myiran/sso"
```

### Using Browser Developer Tools

As an alternative, you can:
1. Open the URL in a browser
2. Open Developer Tools (F12) → Network tab
3. Check "Preserve log"
4. Navigate to the URL
5. View all requests in the Network tab

## Example Output Structure

```
redirect_steps/
├── summary.md          # Overview of all steps
├── step_01.md         # First request/response
├── step_02.md         # Second request/response
└── ...
```

## Target URL

The program is configured to track redirects from:

```
https://sso.my.gov.ir/oauth2/authorize?response_type=code&scope=openid%20profile&client_id=my.tax&state=state1&redirect_uri=https://my.tax.gov.ir/myiran/sso
```

To change the URL, edit the `startURL` variable in the `main()` function.

## Requirements

- Go 1.16 or later
- Network access to the target URLs

## Notes

- The program automatically handles relative and absolute redirect URLs
- Cookies are maintained across requests to simulate browser behavior
- Response bodies are truncated at 50KB in the markdown files to avoid extremely large files
- The program will stop after 20 redirects by default to prevent infinite loops
