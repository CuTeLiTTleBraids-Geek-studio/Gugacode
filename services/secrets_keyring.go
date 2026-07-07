package services

// Shared keyring constants used by the darwin (macOS Keychain) and linux
// (libsecret) platform files. Defined here — without a build tag — so both
// platform files can reference them without duplication. On platforms where
// the keyring CLI path is never taken (windows, BSD), these constants exist
// harmlessly as unused package-level constants (Go allows unused constants).
const (
	// keyringServiceName is the namespace under which gugacode secrets are
	// stored in the platform keyring. On macOS this is the Keychain "service"
	// name; on Linux this becomes a libsecret attribute value.
	keyringServiceName = "gugacode"

	// keyringAccount is the account/label for the AI API key. gugacode
	// currently stores only one secret, so a fixed account name is sufficient.
	// If more secrets are added in the future, this should become a parameter.
	keyringAccount = "ai-api-key"
)
