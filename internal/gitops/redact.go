package gitops

import (
	"net/url"
	"regexp"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// userinfoPattern matches the userinfo section of a URL embedded anywhere in
// free text, e.g. "https://user:token@host/…" inside a go-git error message.
// The class is greedy and admits '@' so that, like url.Parse, the userinfo
// runs to the last '@' before the path; a password containing an unescaped
// '@' is therefore redacted in full. Whitespace is admitted too, so a
// password with an unescaped space is not cut short; the match stops at a
// path '/', a line break or a closing quote.
var userinfoPattern = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^/\r\n"]*@`)

// redactCredentials removes URL userinfo from arbitrary text.
func redactCredentials(s string) string {
	return userinfoPattern.ReplaceAllString(s, "${1}[REDACTED]@")
}

// redactedError wraps an error so that Error() never exposes URL credentials
// while errors.Is / errors.As still see the original chain.
type redactedError struct{ err error }

func (e *redactedError) Error() string { return redactCredentials(e.err.Error()) }
func (e *redactedError) Unwrap() error { return e.err }

// redactError returns err wrapped so its message has URL credentials removed.
// go-git includes the full request URL (userinfo included) in transport
// errors, so every error that can originate from a clone or fetch must pass
// through here before it is logged, returned, or recorded on a span.
func redactError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*redactedError); ok {
		return err
	}
	return &redactedError{err: err}
}

// splitHTTPCredentials removes userinfo from an http(s) repo URL and returns
// it as BasicAuth so the credential is never handed to go-git as part of the
// URL (where it would appear in errors and be persisted in .git/config).
// Non-HTTP URLs and URLs without userinfo are returned unchanged with nil auth.
func splitHTTPCredentials(raw string) (string, transport.AuthMethod) {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw, nil
	}
	if s := strings.ToLower(u.Scheme); s != "http" && s != "https" {
		return raw, nil
	}
	pass, _ := u.User.Password()
	auth := &http.BasicAuth{Username: u.User.Username(), Password: pass}
	u.User = nil
	return u.String(), auth
}

// scrubRemoteCredentials rewrites any http(s) remote URL in the repository
// config that carries userinfo, removing the credentials. Repositories cloned
// before credentials were split out of the URL have the token persisted in
// .git/config; leaving it there would expose it in go-git fetch errors and to
// anything that can read the file. It reports whether anything was changed,
// and returns the credentials removed from the origin remote's first URL so
// a caller with no other auth can keep fetching from it.
func scrubRemoteCredentials(repo *git.Repository) (originAuth transport.AuthMethod, scrubbed bool, err error) {
	cfg, err := repo.Config()
	if err != nil {
		return nil, false, err
	}
	for name, rc := range cfg.Remotes {
		for i, u := range rc.URLs {
			if clean, auth := splitHTTPCredentials(u); auth != nil {
				rc.URLs[i] = clean
				scrubbed = true
				if name == git.DefaultRemoteName && i == 0 {
					originAuth = auth
				}
			}
		}
	}
	if !scrubbed {
		return nil, false, nil
	}
	return originAuth, true, repo.SetConfig(cfg)
}
