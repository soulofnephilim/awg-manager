package selective

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"

	sysexec "github.com/hoaxisr/awg-manager/internal/sys/exec"
)

// StagingSetName is the scratch ipset populated during rebuild and atomically
// swapped with SetName when the pipeline completes.
const StagingSetName = "AWGM-SELECTIVE-STG"

// IpsetChunkSize is how many entries are batched into one ipset restore call.
const IpsetChunkSize = 512

var ipsetChunkPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, IpsetChunkSize*32)
		return &b
	},
}

// EnsureStagingSet creates the staging set (empty) if missing.
func EnsureStagingSet(ctx context.Context) error {
	return createNamedSet(ctx, StagingSetName)
}

// FlushStagingSet removes all members from the staging set.
func FlushStagingSet(ctx context.Context) error {
	return flushNamedSet(ctx, StagingSetName)
}

// SwapWithStaging atomically exchanges live and staging ipset contents.
func SwapWithStaging(ctx context.Context) error {
	bin, err := ipsetBin()
	if err != nil {
		return err
	}
	if err := EnsureStagingSet(ctx); err != nil {
		return err
	}
	if err := CreateSet(ctx); err != nil {
		return err
	}
	res, err := sysexec.Run(ctx, bin, "swap", SetName, StagingSetName)
	if err != nil {
		return sysexec.FormatError(res, fmt.Errorf("ipset swap: %w", err))
	}
	return nil
}

// ChunkedAddStaging appends entries to the staging set in restore batches.
func ChunkedAddStaging(ctx context.Context, cidrs []string) error {
	return chunkedAddToSet(ctx, StagingSetName, cidrs)
}

// ChunkedAddLive appends entries to the live set (CDN refresh path).
func ChunkedAddLive(ctx context.Context, cidrs []string) error {
	return chunkedAddToSet(ctx, SetName, cidrs)
}

func chunkedAddToSet(ctx context.Context, setName string, cidrs []string) error {
	if len(cidrs) == 0 {
		return nil
	}
	for i := 0; i < len(cidrs); i += IpsetChunkSize {
		end := i + IpsetChunkSize
		if end > len(cidrs) {
			end = len(cidrs)
		}
		if err := addEntriesToSet(ctx, setName, cidrs[i:end]); err != nil {
			return err
		}
	}
	return nil
}

func addEntriesToSet(ctx context.Context, setName string, cidrs []string) error {
	if len(cidrs) == 0 {
		return nil
	}
	bin, err := ipsetBin()
	if err != nil {
		return err
	}
	bufPtr := ipsetChunkPool.Get().(*[]byte)
	b := bytes.NewBuffer((*bufPtr)[:0])
	defer func() {
		*bufPtr = b.Bytes()[:0]
		ipsetChunkPool.Put(bufPtr)
	}()

	valid := 0
	for _, raw := range cidrs {
		entry := normalizeEntry(raw)
		if entry == "" {
			continue
		}
		fmt.Fprintf(b, "add %s %s\n", setName, entry)
		valid++
	}
	if valid == 0 {
		return nil
	}
	res, err := sysexec.RunWithOptions(ctx, bin, []string{"restore", "-exist"},
		sysexec.Options{Stdin: b, Timeout: 60e9})
	if err != nil {
		return sysexec.FormatError(res, fmt.Errorf("ipset restore: %w", err))
	}
	return nil
}

func createNamedSet(ctx context.Context, name string) error {
	bin, err := ipsetBin()
	if err != nil {
		return err
	}
	res, err := sysexec.Run(ctx, bin,
		"create", name, "hash:net",
		"maxelem", fmt.Sprintf("%d", setMaxElem),
		"family", "inet",
	)
	if err != nil {
		combined := ""
		if res != nil {
			combined = res.Stdout + res.Stderr
		}
		if strings.Contains(combined, "already exists") {
			return nil
		}
		return sysexec.FormatError(res, fmt.Errorf("ipset create %s: %w", name, err))
	}
	return nil
}

func flushNamedSet(ctx context.Context, name string) error {
	bin, err := ipsetBin()
	if err != nil {
		return err
	}
	res, err := sysexec.Run(ctx, bin, "flush", name)
	if err != nil {
		combined := ""
		if res != nil {
			combined = res.Stdout + res.Stderr
		}
		if strings.Contains(combined, "does not exist") || strings.Contains(combined, "not found") {
			return createNamedSet(ctx, name)
		}
		return sysexec.FormatError(res, fmt.Errorf("ipset flush %s: %w", name, err))
	}
	return nil
}

