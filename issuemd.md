please fix all this issue :

1- In cmd/taxgov/root.go around line 58, the viper environment prefix uses hyphens
("auto-tax-gov") which produces invalid environment variable names; replace
hyphens with underscores (e.g., "auto_tax_gov" or "AUTOTAXGOV" as desired) in
the viper.SetEnvPrefix call and update any documentation or code that references
the old prefix so environment variables use valid names like
AUTO_TAX_GOV_SERVER_PORT.
2- In cmd/taxgov/root.go around lines 89 to 97, the switch unconditionally applies
the logLevel string (which defaults to "info") and thus can overwrite a
config-provided level; only apply the switch when the flag was explicitly set by
the user. Modify the logic to first check whether the log-level flag was changed
(e.g. Flags().Changed("log-level") / pflag) and only then run the switch to map
"debug"/"warn"/"error" to slog levels; otherwise leave the level from config
intact.

3-  func runTrack(cmd *cobra.Command, args []string) error {
 	cfg, err := config.Load(cfgFile)
 	if err != nil {
 		return fmt.Errorf("failed to load config: %w", err)
 	}

+	// Override output directory if flag was set
+	if cmd.Flags().Changed("output") {
+		cfg.Tracker.OutputDir = outputDir
+	}
+
 	t := tracker.New(cfg, logger)

4- In internal/config/config.go around line 118, the code sets a default
sso.auth_header to a Base64 credential string which hardcodes secrets; remove
that default and stop seeding credentials in source. Change to not call
viper.SetDefault for sso.auth_header (or set it to an empty string), require the
value to be provided via a config file or environment variable, and add
validation after loading config to error/exit (or log a clear error) if
sso.auth_header is empty so deployments must supply credentials via secure
config or env vars.

5- In internal/config/config.go around lines 145 to 149, the GetStartURL method
builds the OAuth2 authorization URL by string concatenation which does not
URL-encode query parameter values; change it to construct the URL properly
(parse the base AuthURL into a url.URL or use url.Values), set/query-escape the
parameters (response_type, scope, client_id, state, redirect_uri) using
url.QueryEscape or by assigning values to url.Values and encoding them, then
append the encoded RawQuery to the base URL so client_id and redirect_uri (and
other params) are safely encoded.
6- // RedirectStep represents a single step in the redirect chain.
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
type CaptchaData struct {
	Key        string `json:"key"`
	ImageData  string `json:"imageData"`
	CSRFToken  string `json:"csrfToken"`
	AuthHeader string `json:"authHeader,omitempty"`
}
@coderabbitai
coderabbitai bot
yesterday
⚠️ Potential issue | 🟠 Major

Be deliberate about persisting headers/bodies and AuthHeader (PII/secrets risk).

RedirectStep plus CaptchaData can contain highly sensitive material: full request/response bodies, cookies and auth headers in RequestHeaders/ResponseHeaders, and AuthHeader on the captcha. If steps are ever serialized to disk (e.g., via tracker.SaveResults) or logged, this can inadvertently store PII and credentials (session cookies, Authorization headers, phone numbers in forms, etc.).

If this tool is ever run on anything other than a tightly-controlled local machine, please consider:

Redacting or omitting sensitive headers (Authorization, Cookie, etc.) and body fields before persisting.
Avoiding persistence of AuthHeader entirely, or encrypting it at rest and never returning it to the client.
Making persistence of raw headers/bodies opt-in via config and clearly documented.
7- In internal/server/handlers.go around lines 182 and 229, the code logs full
mobile numbers at Info level which exposes PII; change the logging to avoid
storing raw phone numbers by either removing the mobile field from Info logs or
masking it and lowering the level — implement masking that replaces all but the
last 2–3 digits with asterisks (e.g. "***-***-1234") and log that masked value
instead, or log the event without the mobile at Info and emit the masked mobile
only at Debug/Trace for restricted diagnostics.

8- internal/tracker/auth.go around lines 14 to 31 (also apply same change to lines
~56-75): the function currently treats responses containing obvious failure text
("خطا" or "error") as a non-error by setting result.Message but still returning
a nil error; change this to return an actual error when such failure indicators
are detected (e.g., construct and return fmt.Errorf("OTP request failed: %s",
responseBody) and return nil for the result), so callers/handlers can propagate
a failure; do the same for the other similar 56-75 block and include the
response body (or a trimmed excerpt) in the error for debugging while preserving
successful-path behavior.
9- In internal/tracker/auth.go around lines 30-31 (and also lines 93-94), the code
logs full mobile numbers and the full OTP URL at Info level which exposes PII;
replace these logs to avoid leaking sensitive data by either omitting the mobile
or logging a masked version (e.g., keep last 3–4 digits) via a small maskMobile
helper, and log only the endpoint/path or hostname rather than the full OTP URL
(strip query params). Update the Info calls to use the masked mobile or no
mobile and a sanitized URL, and apply the same changes at the other referenced
lines.
10- internal/tracker/captcha.go around lines 61-66 (and similarly 109-114): the
current return populates AuthHeader into models.CaptchaData which risks leaking
SSO credentials to clients/logs; stop assigning t.cfg.SSO.AuthHeader into the
CaptchaData returned by FetchCaptcha and RefreshCaptcha so the struct only
contains Key, ImageData and CSRFToken, and keep using t.cfg.SSO.AuthHeader
privately when making upstream requests; if you must retain AuthHeader on the
struct for internal use, ensure it is omitted/zeroed before any JSON marshaling
or before returning to callers.
11- internal/tracker/tracker.go around lines 198 to 219: SaveResults and
generateSummaryFile can panic when passed an empty steps slice because
generateSummaryFile assumes steps[0] exists; add a guard at the top of
SaveResults that checks if len(steps) == 0 and either return nil (or still
create an empty summary file) to avoid calling generateSummaryFile with no
entries, and update generateSummaryFile (around lines 296-307) to defensively
handle len(steps) == 0 by writing a sensible placeholder (e.g., “No steps
recorded”) or returning gracefully instead of indexing steps[0]; adjust logging
to reflect the empty-case behavior.
12- In internal/service/mygovauth/otp.go around lines 14 to 15, the code builds the
SendOTPURL by directly interpolating mobile, captchaKey, and captchaCode into
the query string which can break or enable injection when these values contain
special characters; fix it by URL-encoding the parameters — parse the base
SendOTPURL with net/url, create url.Values (or call url.QueryEscape on each
value), set "mobile", "captcha_key", and "captcha_value" into the query values,
assign the encoded query to the URL and use url.String() (or the encoded base +
"?" + values.Encode()) to produce the final loginURL so all parameters are
safely encoded.
13- In internal/service/mygovauth/otp.go around line 29, the logger currently
records the full mobile number which exposes PII; update the logging to avoid
storing the raw mobile number by either omitting the mobile field entirely or
replacing it with a masked version (e.g., keep last 2–4 digits and replace the
rest with asterisks) before passing to s.logger.Info; ensure the log still
provides useful context (e.g., "sending OTP request", "mobile_masked",
maskedMobile, "url", loginURL) and apply the same masking/omission pattern
wherever mobile numbers are logged.
14- In internal/service/mygovauth/otp.go around line 85, the formData is built with
fmt.Sprintf and doesn’t URL-encode otpCode, csrfToken or mobile, which will
break the request if they contain special chars; replace the sprintf
construction with a proper application/x-www-form-urlencoded encoder—create a
url.Values, set "password", "_csrf" and "username" to the respective variables,
then use values.Encode() as the request body and ensure the Content-Type header
is "application/x-www-form-urlencoded".
15- internal/service/mygovauth/otp.go around line 96: the logger currently records
the full mobile number in the "verifying OTP" Info call, which exposes PII;
change the log to either omit the mobile field or pass a masked version (e.g.,
replace all but the last 2–4 digits with asterisks) using a shared maskMobile
helper so logs stay useful but do not contain raw phone numbers; implement or
call maskMobile(mobile) and use that in the logger (or remove the mobile arg
entirely) and apply the same pattern consistently where SendOTP logs the mobile.
16- In internal/service/mygovauth/service.go around lines 159 to 196, the function
FollowRedirectChain hardcodes maxRedirects := 10; change it to use the
configured value (e.g. maxRedirects := s.cfg.HTTP.MaxRedirects) and guard it
with a sensible fallback if the config is zero/negative (for example default to
10) so the method honors the service configuration; ensure the variable type
matches and update any related logic/comments accordingly.
17- -        <div class="status" id="status"></div>
+        <div class="status" id="status" role="alert" aria-live="polite" aria-atomic="true"></div>
18- In web/index.html around line 370 (and also apply the same changes to lines
500–514), the status banner div lacks ARIA attributes so screen readers won’t
announce dynamic updates; update the status container to include role="status",
aria-live="polite" and aria-atomic="true" (or use aria-live="assertive" for
critical error messages) so changes are announced, and ensure messages are set
as textContent (not innerHTML) to avoid unexpected markup; additionally, when a
button triggers a loading state set aria-busy="true" on the button (and add
disabled while loading) and remove/reset aria-busy when the action completes.
19- In internal/service/mygovauth/service.go around lines 100 to 104, the call to
httputil.DumpRequestOut(req, true) stores the full raw request (including
cookies and auth headers) into RedirectStep.RequestBody which can leak sensitive
headers; modify the code to avoid persisting sensitive data by either (a)
removing sensitive headers from a cloned request before dumping (e.g. delete
Authorization, Cookie, Set-Cookie, or other sensitive header keys) and then
dumping the sanitized request, or (b) only populate RequestBody when a
debug/config flag is enabled; implement one of these fixes and ensure
RequestHeaders still uses req.Header.Clone() if needed but without including
secrets in RequestBody.
20 -   const mobile = document.getElementById('mobile').value;
  const captchaCode = document.getElementById('captchaCode').value;
  const loginBtn = document.getElementById('loginBtn');

  if (!captchaData) {
      showStatus('لطفاً منتظر بارگذاری کپچا بمانید', 'error');
      return;
  }
+
+            if (!/^09\d{9}$/.test(mobile)) {
+                showStatus('شماره موبایل نامعتبر است', 'error');
+                return;
+            }
+
+            if (!captchaCode || captchaCode.length !== 6 || !/^\d+$/.test(captchaCode)) {
+                showStatus('کد امنیتی باید ۶ رقم باشد', 'error');
+                return;
+            }
21-   const mobile = document.getElementById('mobile').value;
  const captchaCode = document.getElementById('captchaCode').value;
  const loginBtn = document.getElementById('loginBtn');

  if (!captchaData) {
      showStatus('لطفاً منتظر بارگذاری کپچا بمانید', 'error');
      return;
  }
+
+            if (!/^09\d{9}$/.test(mobile)) {
+                showStatus('شماره موبایل نامعتبر است', 'error');
+                return;
+            }
+
+            if (!captchaCode || captchaCode.length !== 6 || !/^\d+$/.test(captchaCode)) {
+                showStatus('کد امنیتی باید ۶ رقم باشد', 'error');
+                return;
+            }
22-   document.getElementById('otpForm').addEventListener('submit', async (e) => {
      e.preventDefault();

      const otpCode = document.getElementById('otpCode').value;
      const otpBtn = document.getElementById('otpBtn');

+            if (!otpCode || otpCode.length !== 6 || !/^\d+$/.test(otpCode)) {
+                showStatus('کد تأیید باید ۶ رقم باشد', 'error');
+                return;
+            }
+
      if (!captchaData || !userMobile) {
          showStatus('خطا: لطفاً صفحه را رفرش کنید', 'error');
          return;
      }
23- internal/service/mytax/validation.go around lines 31-101: ValidateBasicInfo
currently dereferences opts fields without checking for nil and uses
len(req.UnitTitle) which counts bytes not UTF-8 runes; add an early nil guard
that returns a ValidationResult with an appropriate error when opts is nil (or
alternatively panic/return an explicit error per project convention), and
replace all uses of len(req.UnitTitle) with
utf8.RuneCountInString(req.UnitTitle) so min/max checks operate on characters
(import "unicode/utf8" if not present); keep the existing error messages but
base their conditions on the rune count.
24-