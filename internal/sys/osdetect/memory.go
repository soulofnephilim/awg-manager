package osdetect

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"sync"
)

// LowMemoryThresholdMB is the threshold below which GC optimizations are applied.
// Routers with 256MB RAM report ~248MB, so we use 200MB to avoid false positives.
const LowMemoryThresholdMB = 200

// MidMemoryThresholdMB is the upper bound of the mid-memory GC tier.
// Keenetic devices with 256MB (~248 reported) and 512MB (~500 reported) RAM
// fall into 200..700MB and get a soft GOMEMLIMIT; 1GB models report ~950MB+
// and stay untuned. 700 leaves a wide margin above real 512MB readings
// without catching 1GB devices.
const MidMemoryThresholdMB = 700

var (
	totalMemoryMB   int
	totalMemoryOnce sync.Once
)

// GetTotalMemoryMB returns total system RAM in megabytes.
// The value is cached after first call.
// Returns 0 if unable to determine.
func GetTotalMemoryMB() int {
	totalMemoryOnce.Do(func() {
		totalMemoryMB = detectTotalMemory()
	})
	return totalMemoryMB
}

// IsLowMemoryDevice returns true if the device has less than LowMemoryThresholdMB RAM.
func IsLowMemoryDevice() bool {
	mem := GetTotalMemoryMB()
	return mem > 0 && mem < LowMemoryThresholdMB
}

// GetGCEnv returns environment variables for Go GC tuning.
// If disableMemorySaving is true, returns soft mode (GOGC=100 only).
// If disableMemorySaving is false (default), applies the auto tier table.
// Returns nil for devices with sufficient RAM (>= MidMemoryThresholdMB) in
// auto mode.
func GetGCEnv(disableMemorySaving bool) []string {
	if disableMemorySaving {
		return []string{"GOGC=100"}
	}
	return gcEnvForTotalMemoryMB(GetTotalMemoryMB())
}

// gcEnvForTotalMemoryMB is the GC tier table, split from GetGCEnv so it is
// testable without faking /proc/meminfo.
//
// The <200MB tiers are the historical ones (unchanged). The 200–700MB tier
// exists because 256–512MB routers previously ran with GOGC=100 and NO
// GOMEMLIMIT at all: a selective-ipset rebuild over a huge rule list could
// balloon the heap until the kernel OOM killer fired (observed in the field:
// anon-rss 311MB on a 512MB device). GOMEMLIMIT=96MiB is a SOFT limit chosen
// well above the daemon's ~30-60MB idle heap — normal operation never
// GC-thrashes — while forcing aggressive collection during rebuild spikes
// instead of unbounded growth. GOGC=50 keeps steady-state growth modest
// between spikes. Explicit GOGC/GOMEMLIMIT environment variables always win:
// applyGoMemoryLimits (cmd/awg-manager) skips any knob already present in
// the environment.
func gcEnvForTotalMemoryMB(mem int) []string {
	if mem <= 0 || mem >= MidMemoryThresholdMB {
		return nil
	}

	var memLimit string
	switch {
	case mem < 50:
		memLimit = "16MiB"
	case mem < 100:
		memLimit = "24MiB"
	case mem < LowMemoryThresholdMB:
		memLimit = "32MiB"
	default:
		memLimit = "96MiB"
	}

	return []string{
		"GOGC=50",
		"GOMEMLIMIT=" + memLimit,
	}
}

// detectTotalMemory reads /proc/meminfo and extracts MemTotal.
func detectTotalMemory() int {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.Atoi(fields[1])
				if err != nil {
					return 0
				}
				return kb / 1024
			}
		}
	}
	return 0
}
