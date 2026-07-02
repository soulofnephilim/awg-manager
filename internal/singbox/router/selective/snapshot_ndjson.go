package selective

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hoaxisr/awg-manager/internal/storage"
)

// DomainMatcherRecord is one matcher persisted after rebuild (no IP list).
type DomainMatcherRecord struct {
	Matcher    string   `json:"matcher"`
	Kind       string   `json:"kind"`
	QueryHosts []string `json:"queryHosts"`
	CDN        bool     `json:"cdn,omitempty"`
	Outbound   string   `json:"outbound,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// SnapshotSummary is the in-memory + on-disk metadata for the last rebuild.
type SnapshotSummary struct {
	RebuiltAt          string `json:"rebuiltAt"`
	EntryCount         int    `json:"entryCount"`
	StaticCIDRCount    int    `json:"staticCidrCount"`
	DomainMatcherCount int    `json:"domainMatcherCount"`
	LastCDNRefresh     string `json:"lastCDNRefresh,omitempty"`
}

const (
	snapshotMetaFile  = "selective-snapshot-meta"
	snapshotNDJSON    = "selective-snapshot.ndjson"
	snapshotNDJSONTmp = "selective-snapshot.ndjson.tmp"
)

func snapshotMetaPath(configDir string) string {
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, snapshotMetaFile)
}

func snapshotNDJSONPath(configDir string) string {
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, snapshotNDJSON)
}

// snapshotWriter streams matcher records to NDJSON during rebuild.
type snapshotWriter struct {
	configDir string
	file      *os.File
	enc       *json.Encoder
	mu        *sync.Mutex
}

func newSnapshotWriter(configDir string) (*snapshotWriter, error) {
	p := snapshotNDJSONPath(configDir)
	if p == "" {
		return &snapshotWriter{}, nil
	}
	tmp := filepath.Join(filepath.Dir(p), snapshotNDJSONTmp)
	f, err := os.Create(tmp)
	if err != nil {
		return nil, err
	}
	return &snapshotWriter{configDir: configDir, file: f, enc: json.NewEncoder(f), mu: &sync.Mutex{}}, nil
}

func (w *snapshotWriter) WriteRecord(rec DomainMatcherRecord) error {
	if w.enc == nil {
		return nil
	}
	if w.mu != nil {
		w.mu.Lock()
		defer w.mu.Unlock()
	}
	if rec.QueryHosts == nil {
		rec.QueryHosts = []string{}
	}
	return w.enc.Encode(rec)
}

func (w *snapshotWriter) CloseAndCommit(summary SnapshotSummary) error {
	if w.file == nil {
		writeSnapshotMeta(w.configDir, summary)
		return nil
	}
	if err := w.file.Close(); err != nil {
		return err
	}
	w.file = nil
	dst := snapshotNDJSONPath(w.configDir)
	tmp := filepath.Join(filepath.Dir(dst), snapshotNDJSONTmp)
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	writeSnapshotMeta(w.configDir, summary)
	RemoveLegacySnapshotJSON(w.configDir)
	return nil
}

func (w *snapshotWriter) Abort() {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	p := snapshotNDJSONPath(w.configDir)
	if p != "" {
		_ = os.Remove(filepath.Join(filepath.Dir(p), snapshotNDJSONTmp))
	}
}

func writeSnapshotMeta(configDir string, summary SnapshotSummary) {
	p := snapshotMetaPath(configDir)
	if p == "" {
		return
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return
	}
	// Atomic write: a torn meta file after power loss would make
	// readSnapshotSummary silently return nil and lose the rebuild history.
	_ = storage.AtomicWrite(p, data)
}

func readSnapshotSummary(configDir string) *SnapshotSummary {
	p := snapshotMetaPath(configDir)
	if p == "" {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var s SnapshotSummary
	if err := json.Unmarshal(data, &s); err != nil {
		return nil
	}
	return &s
}

// ReadSnapshotMatchers reads up to limit matcher records starting at offset.
func ReadSnapshotMatchers(configDir string, offset, limit int) ([]DomainMatcherRecord, int, error) {
	p := snapshotNDJSONPath(configDir)
	if p == "" {
		return nil, 0, nil
	}
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var total int
	var page []DomainMatcherRecord
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if total >= offset && len(page) < limit {
			var rec DomainMatcherRecord
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				continue
			}
			if rec.QueryHosts == nil {
				rec.QueryHosts = []string{}
			}
			page = append(page, rec)
		}
		total++
	}
	if err := sc.Err(); err != nil {
		return nil, 0, err
	}
	return page, total, nil
}

// ForEachCDNMatcher scans NDJSON and invokes fn for CDN-flagged matchers.
func ForEachCDNMatcher(configDir string, fn func(DomainMatcherRecord) error) error {
	p := snapshotNDJSONPath(configDir)
	if p == "" {
		return nil
	}
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec DomainMatcherRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if !rec.CDN || rec.Error != "" {
			continue
		}
		if err := fn(rec); err != nil {
			return err
		}
	}
	return sc.Err()
}
