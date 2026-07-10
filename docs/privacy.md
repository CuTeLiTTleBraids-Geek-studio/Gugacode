# Privacy Statement

This document explains what gugacode collects, how secrets are stored, and where your data goes. gugacode is a local-first desktop IDE: the short version is **gugacode collects no telemetry, your API key is encrypted at rest and never returned to the frontend, and AI requests go directly from the app to the AI provider URL you configured — gugacode runs no intermediary server.**

This statement is the companion to the security details in [`../SECURITY.md`](../SECURITY.md) and the extension sandbox details in [`extension-security.md`](extension-security.md).

## 1. API Key Storage (G-SEC-07)

The AI API key you enter in Settings is encrypted at rest before it is written to disk. It is **never** stored in plaintext and **never** returned to the frontend as plaintext.

### Encryption method by platform
| Platform | Method | Location | Notes |
|---|---|---|---|
| **Windows** | DPAPI (`CryptProtectData`) | `settings.json` as a `"dpapi:"`-prefixed blob | Machine-bound: the ciphertext can only be decrypted by the same Windows user account on the same machine. |
| **macOS** | AES-256-GCM (per-install key file), or Keychain via `security` CLI | `settings.json` as an `"aes:"` blob, or a `"keyring:"` marker pointing to a Keychain entry | The AES key file is `~/.config/gugacode/secret.key` with `0600` permissions, generated on first use with `crypto/rand`. |
| **Linux** | AES-256-GCM (per-install key file), or libsecret via `secret-tool` CLI | Same as macOS, but the keyring backend is libsecret (GNOME Keyring / KDE Wallet). | Falls back to AES when the keyring CLI is unavailable. |

Implementation: `services/secrets.go` (dispatcher), `services/secrets_aes.go` (AES-256-GCM), `services/secrets_windows.go` (DPAPI), `services/secrets_darwin.go`, `services/secrets_linux.go`, `services/secrets_keyring.go`.

### Never returned to the frontend
`SettingsService.LoadSettings` clears `AIApiKey` to `""` before returning across the Wails binding, so the plaintext key never lives in the frontend JavaScript heap (where it would be vulnerable to XSS). The frontend receives only:
- `aiApiKeyConfigured` (`bool`) — whether a key is stored.
- `aiApiKeyStorageMethod` (`"dpapi"` / `"aes"` / `"keyring"` / `"plain"` / `"none"`) — how it is stored.

The decrypted key is available to the **backend only**, via `SettingsService.GetDecryptedAPIKey`, which the AI service uses to authenticate requests. Legacy plaintext keys (from older gugacode versions) are auto-migrated to encrypted form on the first `LoadSettings`.

### Atomic, mode-`0600` writes
The settings file is written atomically (temp file → `fsync` → `rename`, G-SEC-09) with filesystem permissions `0600`, so only your user account can read it and a crash mid-write cannot corrupt it.

### Not logged
The API key is never written to log files and never included in error messages. AI error responses are parsed with the body capped at 64 KB (G-SEC-08) and only the provider's structured error message is surfaced.

## 2. AI Requests Go Directly to Your Configured BaseURL

When you send a chat message, generate a conversation title, request inline completion, or list models, gugacode's backend (`services/ai_service.go`) constructs an HTTP `POST` (or `GET` for `/v1/models`) **directly to `cfg.BaseURL + "/v1/chat/completions"` or `cfg.BaseURL + "/v1/messages"`** — the URL you configured in Settings.

- **No intermediary server.** gugacode does not operate a proxy, relay, or "AI gateway" that your requests pass through. The request goes from your machine straight to your AI provider (OpenAI, Anthropic, Azure OpenAI, OpenRouter, Ollama, LM Studio, llama.cpp, etc.).
- **No redirect following.** The AI HTTP client sets `CheckRedirect: noRedirectPolicy` (`http.ErrUseLastResponse`), so a malicious or compromised provider cannot redirect your API key to a different host.
- **BaseURL is validated** (G-SEC-01, `services/ai_urlsec.go`) before it is stored: scheme must be `http`/`https`, no embedded userinfo, and non-loopback hosts must use `https`. Loopback hosts (`localhost`, `127.0.0.0/8`, `::1`, `*.localhost`) may use plain `http` so you can point gugacode at a local LLM server.
- **Only the AI provider sees the key.** The `Authorization: Bearer <key>` (OpenAI protocol) or `x-api-key: <key>` (Anthropic protocol) header is sent only to your configured BaseURL.

The only other external network requests the app makes are link clicks in the Help menu (opened in your external browser, not inside the app).

## 3. Telemetry: None

**gugacode collects no telemetry.** Specifically:
- **No usage analytics.** We do not track which features you use, how often, or for how long.
- **No crash reports.** There is no crash-reporting SDK (no Sentry, Crashlytics, Bugsnag, etc.) bundled in the app. Crashes are written only to your local log file (`<configDir>/gugacode/logs/`).
- **No error reports.** Errors are logged locally; nothing is sent to gugacode.
- **No update pings to gugacode.** The app does not "phone home" to check for updates or report its version. (Update checks, if any, are done by the platform's package manager or by you manually.)
- **No unique device/user IDs.** No machine ID, install ID, or anonymous client ID is generated or transmitted.

A codebase-wide search for `telemetry`, `analytics`, `sentry`, `posthog`, `mixpanel`, `amplitude`, `gtag`, `google-analytics` returns no matches in either the Go backend or the Vue frontend. If a future release adds optional telemetry, it will be **opt-in, off by default, and documented here**.

## 4. Local Data

All user data is stored **locally** on your machine in your XDG config directory (typically `~/.config/gugacode/` on Linux/macOS, `%APPDATA%\gugacode\` on Windows). Nothing is synced to gugacode's servers because gugacode has no servers for your data.

| Data | Path | Notes |
|---|---|---|
| Settings (including the encrypted API key) | `<configDir>/gugacode/settings.json` | Atomic write, `0600`. Profile-aware: each profile has its own `settings.json`. |
| AES key file (macOS/Linux fallback) | `<configDir>/gugacode/secret.key` | `0600`, 32 random bytes. |
| Conversations | `<configDir>/gugacode/conversations/` | One JSON file per conversation. |
| Presets / profiles / rules | `<configDir>/gugacode/presets/`, `profiles/`, `rules/` | JSON files. |
| Extension security state | `<configDir>/gugacode/extension-security.json` | Per-extension classification, permissions, SHA-256, enabled state. |
| Extension blacklist (user-added) | `<configDir>/gugacode/extension-blacklist.json` | User-added malicious-extension IDs. |
| Logs | `<configDir>/gugacode/logs/` | Local slog output; no remote shipping. |

`<configDir>` resolves via the `github.com/adrg/xdg` library (`xdg.ConfigHome`). To find the exact path on your machine, check the Settings → General panel, which shows the active profile's config path.

To remove all gugacode data, delete the `<configDir>/gugacode/` directory. On Windows, also be aware DPAPI-encrypted blobs are tied to your Windows user account — deleting the directory removes the ciphertext; the key cannot be recovered.

## 5. Extensions Are Sandboxed

VS Code-style extensions and native plugins run in a restricted environment. Their network access is governed by their declared permission level:

| Extension level | Network access |
|---|---|
| **Trusted** | None. |
| **Reviewed** | None (file write + terminal only). |
| **Restricted** | Allowed, but each `network.request` requires interactive confirmation, and the BaseURL the extension reaches is the extension's own — it does **not** inherit your AI API key. Extensions never receive `appState.aiApiKey` (it is empty in the frontend state; G-SEC-07). |

Extension webviews render in an `<iframe sandbox="allow-scripts">` with **no `allow-same-origin`**, so they cannot access the parent's DOM, `localStorage`, cookies, or `window.go` bindings. See [`extension-security.md`](extension-security.md) for the full model.

If you do not trust an extension, leave it disabled (the default) or remove it. Newly installed extensions start disabled + pending review (G-SEC-12).

## 6. Offline Behavior

All features except live AI calls work **without an internet connection**:
- Editor, file tree, search, git operations, terminal, multi-tab terminal.
- Settings, profiles, presets, rules, conversations (locally cached).
- LSP language servers (run locally).
- Inline completion, chat, conversation-title generation, model listing — **require** network access to your configured AI BaseURL. If you point gugacode at a local LLM server (Ollama, LM Studio, llama.cpp) on `localhost`, even "AI" features work fully offline.

The app does not require network access to launch and does not perform any startup "phone home" call.

## 7. Summary

| Question | Answer |
|---|---|
| Does gugacode collect telemetry? | **No.** No usage data, no crash reports, no error reports, no device IDs. |
| Is my API key encrypted? | **Yes.** DPAPI on Windows, AES-256-GCM or Keychain/libsecret elsewhere. |
| Is my API key sent to gugacode? | **No.** It is sent only to your configured AI BaseURL. |
| Is my API key returned to the frontend? | **No.** Only a `configured` boolean and a `storageMethod` label cross the binding. |
| Do AI requests pass through a gugacode server? | **No.** They go directly from the app to your BaseURL. |
| Where is my data stored? | **Locally**, in your XDG config directory. |
| Does gugacode work offline? | **Yes**, except for live AI calls (unless you use a local LLM). |

For security vulnerability reporting, see [`../SECURITY.md`](../SECURITY.md). For the extension permission model and sandbox boundaries, see [`extension-security.md`](extension-security.md).
