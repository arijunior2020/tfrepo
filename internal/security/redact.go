package security

import (
	"regexp"
	"sort"
	"strings"
)

// authURLPattern matches the credentials portion of a URL such as
// "https://oauth2:TOKEN@gitlab.com/..." — anything between "://" and "@"
// that contains no whitespace or "/". It cannot catch a bare token that
// appears outside of a URL; pass known secrets explicitly for that.
var authURLPattern = regexp.MustCompile(`(https?://)[^@\s/]+@`)

// Redact replaces credentials embedded in authenticated URLs with "***" and
// replaces any provided secret values wherever they appear. Empty secrets
// are ignored. Longer secrets are replaced first so that one secret being a
// prefix of another doesn't leave a partial value behind.
func Redact(value string, secrets ...string) string {
	result := authURLPattern.ReplaceAllString(value, "${1}***@")

	nonEmpty := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		if secret != "" {
			nonEmpty = append(nonEmpty, secret)
		}
	}

	sort.Slice(nonEmpty, func(i, j int) bool { return len(nonEmpty[i]) > len(nonEmpty[j]) })

	for _, secret := range nonEmpty {
		result = strings.ReplaceAll(result, secret, "***")
	}

	return result
}
