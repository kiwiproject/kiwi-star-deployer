// Package gpg makes sure gpg-agent can sign without prompting, which the
// release's non-interactive Maven build requires.
package gpg

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
)

// warmTimeout bounds the warm-up. It is normally instant (pinentry-mac
// answers from the Keychain), but must leave time for someone to type the
// passphrase into a pinentry dialog when the key is not saved there.
const warmTimeout = 2 * time.Minute

// Warm signs an empty message so gpg-agent caches the signing passphrase.
//
// The release runs mvn with -B, which makes maven-gpg-plugin pass
// --pinentry-mode error: gpg-agent is then forbidden from launching pinentry
// and fails with "No pinentry" unless the passphrase is already cached. These
// flags deliberately omit that option, so pinentry may run here (pinentry-mac
// answers silently from the Keychain) and fill the cache for the later signing.
func Warm(r runner.Runner) error {
	// Stdin is deliberately left unset: os/exec then connects the child's stdin
	// to the null device, so gpg signs an empty message instead of waiting for
	// input from the terminal.
	res, err := r.Run(runner.Options{
		Command: "gpg",
		Args:    []string{"--armor", "--detach-sign", "--output", os.DevNull},
		Timeout: warmTimeout,
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return errors.New("gpg not found on PATH")
	}

	msg := fmt.Sprintf("gpg could not sign: %v", err)
	if res != nil {
		if detail := strings.Join(strings.Fields(strings.ReplaceAll(strings.TrimSpace(res.Stderr), "\n", "; ")), " "); detail != "" {
			msg += " (" + detail + ")"
		}
	}
	return fmt.Errorf("%s; the release runs mvn -B, which cannot prompt for a passphrase, so gpg-agent must be able to unlock the key first: check that gpg and pinentry work, e.g. echo test | gpg --clearsign, and consider raising default-cache-ttl in ~/.gnupg/gpg-agent.conf", msg)
}
