package singbox

import (
	"net/netip"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	ipv4WithOptionalPortRe   = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}(?::\d{1,5})?\b`)
	ipv6BracketWithPortRe    = regexp.MustCompile(`\[[0-9A-Fa-f:]+\](?::\d{1,5})?`)
	ipv6PlainRe              = regexp.MustCompile(`\b[0-9A-Fa-f:]*:[0-9A-Fa-f:]+\b`)
	domainWithOptionalPortRe = regexp.MustCompile(`(?i)\b(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{1,62}(?::\d{1,5})?\b`)
)

func sanitizeSingboxLogText(s string) string {
	if s == "" {
		return s
	}
	s = ipv6BracketWithPortRe.ReplaceAllStringFunc(s, redactIPv6WithOptionalPort)
	s = ipv6PlainRe.ReplaceAllStringFunc(s, redactIPv6WithOptionalPort)
	s = ipv4WithOptionalPortRe.ReplaceAllStringFunc(s, redactIPWithOptionalPort)
	s = domainWithOptionalPortRe.ReplaceAllStringFunc(s, redactDomainWithOptionalPort)
	return s
}

func redactIPWithOptionalPort(s string) string {
	host, port := splitHostPortOptional(s)
	if host == "" {
		return s
	}
	addr, err := netip.ParseAddr(host)
	if err != nil || !addr.Is4() {
		return s
	}
	return redactSensitiveToken(host) + port
}

func redactDomainWithOptionalPort(s string) string {
	host, port := splitHostPortOptional(s)
	if host == "" || !strings.Contains(host, ".") {
		return s
	}
	return redactSensitiveToken(host) + port
}

func redactIPv6WithOptionalPort(s string) string {
	hostPart := s
	port := ""
	bracketed := false
	if strings.HasPrefix(hostPart, "[") {
		bracketed = true
		end := strings.IndexByte(hostPart, ']')
		if end <= 1 {
			return s
		}
		hostPart = hostPart[1:end]
		rest := s[end+1:]
		if rest != "" {
			if !strings.HasPrefix(rest, ":") {
				return s
			}
			for _, ch := range rest[1:] {
				if ch < '0' || ch > '9' {
					return s
				}
			}
			port = rest
		}
	} else {
		hostPart, port = splitHostPortOptionalIPv6(s)
	}

	addr, err := netip.ParseAddr(hostPart)
	if err != nil || !addr.Is6() {
		return s
	}

	masked := redactSensitiveToken(hostPart)
	if bracketed {
		return "[" + masked + "]" + port
	}
	return masked + port
}

func splitHostPortOptional(s string) (host, port string) {
	i := strings.LastIndexByte(s, ':')
	if i <= 0 || i == len(s)-1 {
		return s, ""
	}
	for _, ch := range s[i+1:] {
		if ch < '0' || ch > '9' {
			return s, ""
		}
	}
	return s[:i], s[i:]
}

func splitHostPortOptionalIPv6(s string) (host, port string) {
	// For plain IPv6 we only treat ":<digits>" as port when it's unambiguously a port suffix.
	i := strings.LastIndexByte(s, ':')
	if i <= 0 || i == len(s)-1 {
		return s, ""
	}
	for _, ch := range s[i+1:] {
		if ch < '0' || ch > '9' {
			return s, ""
		}
	}
	candidateHost := s[:i]
	if addr, err := netip.ParseAddr(candidateHost); err == nil && addr.Is6() {
		return candidateHost, s[i:]
	}
	return s, ""
}

func redactSensitiveToken(s string) string {
	n := utf8.RuneCountInString(s)
	if n <= 4 {
		return strings.Repeat("*", n)
	}
	r := []rune(s)
	return string(r[:2]) + strings.Repeat("*", n-4) + string(r[n-2:])
}
