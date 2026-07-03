package auth

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/hoaxisr/awg-manager/internal/auth/shadowcrypt"
)

// defaultShadowPath is the Entware shadow database on Keenetic routers.
const defaultShadowPath = "/opt/etc/shadow"

var (
	// ErrEntwareUnavailable means /opt/etc/shadow is missing or unreadable —
	// Entware credential verification cannot be performed at all.
	ErrEntwareUnavailable = errors.New("entware shadow database unavailable")
	// ErrEntwareUserNotFound means the login has no shadow entry.
	ErrEntwareUserNotFound = errors.New("entware user not found")
	// ErrEntwareAccountLocked means the entry exists but its hash field is
	// empty or locked ("!", "*") — the account cannot be used for login.
	ErrEntwareAccountLocked = errors.New("entware account locked")
	// ErrUnsupportedHash means the stored hash uses a scheme shadowcrypt
	// cannot verify (yescrypt, bcrypt, DES, ...). Toward the HTTP client
	// this is indistinguishable from invalid credentials.
	ErrUnsupportedHash = errors.New("неподдерживаемый формат хэша")
)

// EntwareVerifier verifies logins against the Entware shadow database
// entirely locally — no NDMS /auth call, hence no router-side
// authentication notifications (issue #441).
type EntwareVerifier struct {
	// ShadowPath is variable so tests can point at a fixture file.
	ShadowPath string
}

// NewEntwareVerifier returns a verifier reading /opt/etc/shadow.
func NewEntwareVerifier() *EntwareVerifier {
	return &EntwareVerifier{ShadowPath: defaultShadowPath}
}

// dummyShadowHash is a fixed, well-formed $6$ SHA-512-crypt hash with the
// default 5000 rounds (the official Drepper "Hello world!" vector). It exists
// only to run a full KDF of comparable cost when the account is absent or
// locked, so those cases take about the same time as a wrong password against
// a real $6$ entry — closing the user-enumeration timing channel (issue #441).
// No password ever matches it; the result is intentionally discarded.
const dummyShadowHash = "$6$saltstring$svn8UoSVapNtMuq1ukKS4tPQd8iKwSMHWjl/O817G3uBnIFNjnQJuesI68u4OTLiBFdcbYEdFCoEOfaS35inz1"

// dummyKDFSink receives the discarded dummy-KDF result. Writing to a
// package-level variable prevents the compiler from proving the call dead and
// optimizing the equalizing work away.
var dummyKDFSink error

// runDummyKDF performs a full SHA-512-crypt computation whose cost matches the
// wrong-password path, then throws the result away. See dummyShadowHash.
func runDummyKDF(password string) {
	dummyKDFSink = shadowcrypt.Verify(password, dummyShadowHash)
}

// Verify checks login/password against the shadow database. Returns nil on
// success. Failures are distinguished internally (ErrEntwareUnavailable,
// ErrEntwareUserNotFound, ErrEntwareAccountLocked, ErrUnsupportedHash,
// ErrInvalidCredentials) so the caller can log the reason — but the HTTP
// layer must always collapse them into the same 401 invalid-credentials
// response to avoid user enumeration.
func (v *EntwareVerifier) Verify(login, password string) error {
	hash, err := v.lookupHash(login)
	if err != nil {
		// The account is absent or locked, so there is no stored hash to
		// verify against — but a wrong password for an EXISTING user runs the
		// full 5000-round KDF. Run an equal-cost dummy KDF here so response
		// latency does not reveal which logins exist. The internal error
		// distinction is preserved for logging.
		if errors.Is(err, ErrEntwareUserNotFound) || errors.Is(err, ErrEntwareAccountLocked) {
			runDummyKDF(password)
		}
		return err
	}
	switch err := shadowcrypt.Verify(password, hash); {
	case err == nil:
		return nil
	case errors.Is(err, shadowcrypt.ErrMismatch):
		return ErrInvalidCredentials
	case errors.Is(err, shadowcrypt.ErrUnsupported):
		return fmt.Errorf("%w (login %q)", ErrUnsupportedHash, login)
	default:
		// Malformed hash — treat like an unsupported entry.
		return fmt.Errorf("%w: %v", ErrUnsupportedHash, err)
	}
}

// lookupHash finds the crypt hash for login in the shadow file.
func (v *EntwareVerifier) lookupHash(login string) (string, error) {
	data, err := os.ReadFile(v.ShadowPath)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrEntwareUnavailable, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 2 || fields[0] != login {
			continue
		}
		hash := fields[1]
		if hash == "" || strings.HasPrefix(hash, "!") || strings.HasPrefix(hash, "*") {
			return "", ErrEntwareAccountLocked
		}
		return hash, nil
	}
	return "", ErrEntwareUserNotFound
}
