package routerclock

import (
	"os"
	"strconv"
	"strings"
	"time"
)

var tzCandidates = []string{"/etc/TZ", "/var/TZ"}

// Info describes router local clock info derived from /etc/TZ or /var/TZ.
type Info struct {
	Now           time.Time
	ZoneName      string
	OffsetMinutes int
	Location      *time.Location
	Source        string
	RawTZ         string
}

// Get returns router clock info. Primary source is Keenetic-style TZ files
// (/etc/TZ then /var/TZ). Fallback is Go runtime zone.
func Get() Info {
	if tz, source, ok := readRouterTZ(); ok {
		if zoneName, offsetMinutes, ok := parsePOSIXTZ(tz); ok {
			loc := time.FixedZone(zoneName, offsetMinutes*60)
			now := time.Now().In(loc)
			return Info{
				Now:           now,
				ZoneName:      zoneName,
				OffsetMinutes: offsetMinutes,
				Location:      loc,
				Source:        source,
				RawTZ:         tz,
			}
		}
	}

	now := time.Now()
	zoneName, zoneOffsetSeconds := now.Zone()
	return Info{
		Now:           now,
		ZoneName:      zoneName,
		OffsetMinutes: zoneOffsetSeconds / 60,
		Location:      now.Location(),
		Source:        "runtime",
	}
}

// Convert converts t to router local time location.
func Convert(t time.Time) time.Time {
	return t.In(Get().Location)
}

func readRouterTZ() (string, string, bool) {
	for _, p := range tzCandidates {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		s := strings.TrimSpace(string(b))
		if s == "" {
			continue
		}
		return s, p, true
	}
	return "", "", false
}

// Test hook: override TZ candidate file list.
func setTZCandidatesForTest(paths []string) func() {
	prev := tzCandidates
	tzCandidates = append([]string(nil), paths...)
	return func() { tzCandidates = prev }
}

// parsePOSIXTZ parses minimal POSIX TZ prefix (e.g. MSK-3, UTC0, EST5EDT).
// POSIX sign is inverted vs UTC offset:
//
//	MSK-3 -> UTC+3, EST5 -> UTC-5.
func parsePOSIXTZ(s string) (zoneName string, offsetMinutes int, ok bool) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return "", 0, false
	}

	i := 0
	for i < len(raw) {
		c := raw[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			i++
			continue
		}
		break
	}
	if i == 0 {
		return "", 0, false
	}
	zoneName = raw[:i]
	if i >= len(raw) {
		return "", 0, false
	}

	j := i
	sign := 1
	if raw[j] == '+' {
		j++
	} else if raw[j] == '-' {
		sign = -1
		j++
	}

	startHours := j
	for j < len(raw) && raw[j] >= '0' && raw[j] <= '9' {
		j++
	}
	if startHours == j {
		return "", 0, false
	}

	hours, err := strconv.Atoi(raw[startHours:j])
	if err != nil {
		return "", 0, false
	}

	minutes := 0
	if j < len(raw) && raw[j] == ':' {
		j++
		startMin := j
		for j < len(raw) && raw[j] >= '0' && raw[j] <= '9' {
			j++
		}
		if startMin == j {
			return "", 0, false
		}
		minutes, err = strconv.Atoi(raw[startMin:j])
		if err != nil || minutes < 0 || minutes > 59 {
			return "", 0, false
		}
	}

	totalPOSIXMinutes := sign * (hours*60 + minutes)
	offsetMinutes = -totalPOSIXMinutes
	return zoneName, offsetMinutes, true
}
