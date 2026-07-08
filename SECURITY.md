# Security Policy

## Supported Versions

| Version | Supported |
|---|---|
| 1.0.x | ✅ |
| < 1.0 | ❌ |

## Reporting a Vulnerability

If you discover a security vulnerability in gugacode, please report it responsibly:

1. **Do NOT open a public GitHub issue** for security vulnerabilities.
2. Email security@gugacode.dev with a description of the vulnerability, steps to reproduce, and potential impact.
3. You will receive an acknowledgment within 48 hours.
4. We will investigate and provide a fix timeline within 7 days.

Please include:
- Description of the vulnerability
- Steps to reproduce
- Affected components (backend service, frontend component, etc.)
- Potential impact
- Suggested fix (if any)

## Security Measures

### Path Sandboxing
All file operations are sandboxed to the workspace root. `FileService.validatePath()` prevents directory traversal attacks by checking that the resolved path is within the workspace root. Terminal sessions validate their working directory similarly.

### Input Validation
- Project IDs are validated as hex strings to prevent path traversal via filenames.
- AI API responses are checked for non-2xx status codes and parsed for structured error messages.
- HTTP clients disable redirects to prevent SSRF.

### XSS Prevention
- Markdown rendering uses DOMPurify to sanitize HTML before rendering.
- All user input displayed in the UI is escaped by Vue's template engine by default.

### API Key Storage
- API keys are stored in the local settings file (XDG config directory) and never transmitted to any server except the configured AI provider.
- API keys are not logged or included in error messages.

### Dependency Security
- Run `govulncheck ./...` to scan Go dependencies for known vulnerabilities.
- Run `npm audit` in the frontend directory to check npm dependencies.
- Both should be run in CI before releases.

## Security Headers

The Wails v3 webview does not make external network requests except:
- AI provider API calls (user-configured base URL)
- Link clicks in the Help menu (opens in external browser)

No CSRF, ClickJacking, or CORS protections are needed since the app runs in a desktop webview, not a browser.

## Disclosure Timeline

- **Day 0**: Vulnerability reported
- **Day 1-2**: Acknowledgment and initial assessment
- **Day 3-7**: Fix development and testing
- **Day 7-14**: Patch release (severity-dependent)
- **Day 30**: Public disclosure (if applicable)

## Contact

- Security email: security@gugacode.dev
- General issues: [GitHub Issues](https://github.com/gugacode/gugacode/issues)
