package domainutil

import (
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// RegistrableDomain returns the eTLD+1 for a host or URL when the public suffix
// list can identify one. It falls back to the normalized host for private or
// development hosts so callers do not lose usable evidence.
func RegistrableDomain(value string) string {
	value = strings.Trim(strings.TrimSpace(value), `"'()[]{}<>.,;`)
	if value == "" {
		return ""
	}
	host := value
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil {
			return ""
		}
		host = parsed.Hostname()
	}
	host = strings.Trim(strings.ToLower(host), ".")
	host = strings.TrimPrefix(host, "www.")
	if host == "" {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil {
		return host
	}
	if registrable, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil {
		return registrable
	}
	if strings.Contains(host, ".") {
		return host
	}
	return ""
}

func SameRegistrableDomain(a, b string) bool {
	left := RegistrableDomain(a)
	right := RegistrableDomain(b)
	return left != "" && right != "" && left == right
}
