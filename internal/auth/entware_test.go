package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hoaxisr/awg-manager/internal/auth/shadowcrypt"
)

// writeShadow writes a shadow fixture and returns a verifier pointed at it.
func writeShadow(t *testing.T, content string) *EntwareVerifier {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shadow")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write shadow fixture: %v", err)
	}
	return &EntwareVerifier{ShadowPath: path}
}

// fixture hash: SHA-256-crypt of "Hello world!" — official vector from
// Drepper's SHA-crypt specification (also glibc-verified, see shadowcrypt
// tests for provenance).
const shadowFixture = `# comment line
root:$5$saltstring$5B8vYYiY.CVt1RlTTf8KbXBH3hsxY/GNooZaBBGWEc5:19000:0:99999:7:::
locked:!:19000:0:99999:7:::
locked2:!$5$whatever$hash:19000:0:99999:7:::
star:*:19000::::::
empty::19000::::::
bcryptuser:$2b$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy:19000::::::

malformedline
`

func TestEntwareVerify_Success(t *testing.T) {
	v := writeShadow(t, shadowFixture)
	if err := v.Verify("root", "Hello world!"); err != nil {
		t.Fatalf("Verify(root, correct) = %v, want nil", err)
	}
}

func TestEntwareVerify_WrongPassword(t *testing.T) {
	v := writeShadow(t, shadowFixture)
	if err := v.Verify("root", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Verify(root, wrong) = %v, want ErrInvalidCredentials", err)
	}
}

func TestEntwareVerify_UserNotFound(t *testing.T) {
	v := writeShadow(t, shadowFixture)
	if err := v.Verify("nobody", "x"); !errors.Is(err, ErrEntwareUserNotFound) {
		t.Fatalf("Verify(nobody) = %v, want ErrEntwareUserNotFound", err)
	}
}

func TestEntwareVerify_LockedAccounts(t *testing.T) {
	v := writeShadow(t, shadowFixture)
	for _, login := range []string{"locked", "locked2", "star", "empty"} {
		if err := v.Verify(login, "x"); !errors.Is(err, ErrEntwareAccountLocked) {
			t.Errorf("Verify(%s) = %v, want ErrEntwareAccountLocked", login, err)
		}
	}
}

func TestEntwareVerify_UnsupportedHash(t *testing.T) {
	v := writeShadow(t, shadowFixture)
	if err := v.Verify("bcryptuser", "x"); !errors.Is(err, ErrUnsupportedHash) {
		t.Fatalf("Verify(bcryptuser) = %v, want ErrUnsupportedHash", err)
	}
}

func TestEntwareVerify_MissingShadowFile(t *testing.T) {
	v := &EntwareVerifier{ShadowPath: filepath.Join(t.TempDir(), "does-not-exist")}
	if err := v.Verify("root", "x"); !errors.Is(err, ErrEntwareUnavailable) {
		t.Fatalf("Verify(missing file) = %v, want ErrEntwareUnavailable", err)
	}
}

// TestEntwareVerify_DummyKDFRunsForAbsentUser pins the anti-enumeration fix:
// a not-found or locked account still burns a full KDF (against the fixed
// dummy hash) so its verification time matches the wrong-password path. The
// discarded result lands in dummyKDFSink, which both proves the KDF ran and
// prevents the compiler from optimizing the equalizing work away.
func TestEntwareVerify_DummyKDFRunsForAbsentUser(t *testing.T) {
	v := writeShadow(t, shadowFixture)
	for _, tc := range []struct {
		login   string
		wantErr error
	}{
		{"nobody", ErrEntwareUserNotFound},
		{"locked", ErrEntwareAccountLocked},
	} {
		dummyKDFSink = nil
		if err := v.Verify(tc.login, "some-password"); !errors.Is(err, tc.wantErr) {
			t.Fatalf("Verify(%s) = %v, want %v", tc.login, err, tc.wantErr)
		}
		// The dummy KDF must have executed (wrong password vs the fixed $6$
		// hash → ErrMismatch), proving it was not skipped/optimized away.
		if !errors.Is(dummyKDFSink, shadowcrypt.ErrMismatch) {
			t.Fatalf("Verify(%s): dummyKDFSink = %v, want a completed KDF (ErrMismatch)", tc.login, dummyKDFSink)
		}
	}
}

// TestEntwareVerify_AbsentAndWrongPasswordTakeComparableTime is a coarse,
// non-flaky guard that the absent-user path is no longer a fast early return:
// it must take a KDF-sized amount of time, not microseconds. A wide margin
// keeps it stable under -race / loaded CI.
func TestEntwareVerify_AbsentAndWrongPasswordTakeComparableTime(t *testing.T) {
	v := writeShadow(t, shadowFixture)

	measure := func(login, pw string) time.Duration {
		start := time.Now()
		_ = v.Verify(login, pw)
		return time.Since(start)
	}
	// Warm up (file cache, allocator) so the first call's cost is not blamed
	// on the KDF.
	_ = v.Verify("root", "warmup")

	wrong := measure("root", "definitely-wrong") // full real KDF
	absent := measure("nobody", "definitely-wrong")

	// The absent path must spend at least a small fraction of the real-KDF
	// time; a bare not-found early return would be orders of magnitude faster.
	if absent < wrong/8 {
		t.Fatalf("absent-user path too fast: %v vs wrong-password %v (timing channel not closed)", absent, wrong)
	}
}
