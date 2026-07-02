package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/hoaxisr/awg-manager/internal/singbox/router/selective"
)

// enrichSelectiveRuleSetRefs attaches dat rule-set metadata from the live router
// config. Matchers are streamed from disk via OpenRuleSetJSON during rebuild —
// full rule-set JSON is not loaded into memory here.
func (s *ServiceImpl) enrichSelectiveRuleSetRefs(ctx context.Context, refs []selective.RuleSetRef, cfg *RouterConfig) []selective.RuleSetRef {
	if len(refs) == 0 || cfg == nil {
		return refs
	}
	byTag := make(map[string]RuleSet, len(cfg.Route.RuleSet))
	for _, rs := range cfg.Route.RuleSet {
		byTag[rs.Tag] = rs
	}
	out := make([]selective.RuleSetRef, len(refs))
	copy(out, refs)
	for i := range out {
		rs, ok := byTag[out[i].Tag]
		if !ok {
			continue
		}
		if kind, tags, ok := parseDatRuleSetURL(rs.URL); ok {
			out[i].DatKind = kind
			out[i].DatTags = tags
		}
		if out[i].URL == "" {
			out[i].URL = rs.URL
		}
		if out[i].Format == "" {
			out[i].Format = rs.Format
		}
		if len(rs.Rules) > 0 {
			out[i].Rules = append(out[i].Rules, rs.Rules...)
		}
	}
	return out
}

func (s *ServiceImpl) loadRuleSetSourceRules(ctx context.Context, rs RuleSet) ([]map[string]any, error) {
	if len(rs.Rules) > 0 {
		return rs.Rules, nil
	}
	raw, err := s.loadRuleSetSourceJSON(ctx, rs)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var src inlineRuleSetSource
	if err := json.Unmarshal(raw, &src); err != nil {
		return nil, fmt.Errorf("parse rule-set source: %w", err)
	}
	return src.Rules, nil
}

func (s *ServiceImpl) loadRuleSetSourceJSON(ctx context.Context, rs RuleSet) ([]byte, error) {
	configDir := s.singboxConfigDir()
	inlineDir := filepath.Join(configDir, "rule-sets", "inline")
	datDir := filepath.Join(configDir, "rule-sets", "dat")

	if rs.Type == "inline" {
		if p := filepath.Join(inlineDir, safeRuleSetFilename(rs.Tag)+".json"); fileReadable(p) {
			return os.ReadFile(p)
		}
	}

	if rs.Path != "" {
		if data, ok := readRuleSetJSONSibling(rs.Path); ok {
			return data, nil
		}
	}

	if rs.Type == "remote" && strings.TrimSpace(rs.URL) != "" {
		if kind, tags, ok := parseDatRuleSetURL(rs.URL); ok {
			return s.loadDatRuleSetSourceJSON(ctx, datDir, kind, tags)
		}
		return s.loadRemoteRuleSetSourceJSON(ctx, rs)
	}

	if rs.Type == "local" && rs.Path != "" {
		if data, ok := readRuleSetJSONSibling(rs.Path); ok {
			return data, nil
		}
	}

	if rs.Type == "remote" && datDir != "" && configDir != "" {
		p := filepath.Join(datDir, safeRuleSetFilename(rs.Tag)+".json")
		if fileReadable(p) {
			return os.ReadFile(p)
		}
	}

	return nil, nil
}

func (s *ServiceImpl) singboxConfigDir() string {
	if s.deps.Orch != nil {
		return s.deps.Orch.ConfigDir()
	}
	if s.deps.Singbox != nil {
		return s.deps.Singbox.ConfigDir()
	}
	return ""
}

func readRuleSetJSONSibling(path string) ([]byte, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		data, err := os.ReadFile(path)
		return data, err == nil
	}
	jsonPath := strings.TrimSuffix(path, ".srs") + ".json"
	data, err := os.ReadFile(jsonPath)
	return data, err == nil
}

func fileReadable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func parseDatRuleSetURL(rawURL string) (kind string, tags []string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u == nil {
		return "", nil, false
	}
	if !strings.HasSuffix(u.Path, "/dat-srs") && !strings.HasSuffix(u.Path, "dat-srs") {
		return "", nil, false
	}
	kind = strings.ToLower(strings.TrimSpace(u.Query().Get("kind")))
	if kind != "geosite" && kind != "geoip" {
		return "", nil, false
	}
	tags = dedupeStrings(u.Query()["tag"])
	if len(tags) == 0 {
		return "", nil, false
	}
	return kind, tags, true
}

func (s *ServiceImpl) loadDatRuleSetSourceJSON(ctx context.Context, datDir, kind string, tags []string) ([]byte, error) {
	if datDir == "" {
		return nil, fmt.Errorf("dat rule-set dir unavailable")
	}
	jsonPath := filepath.Join(datDir, datRuleSetBaseName(kind, tags)+".json")
	if fileReadable(jsonPath) {
		return os.ReadFile(jsonPath)
	}
	token, err := s.ensureDatRuleSetToken()
	if err != nil {
		return nil, err
	}
	if _, err := s.DatRuleSetFile(ctx, kind, tags, token); err != nil {
		return nil, err
	}
	return os.ReadFile(jsonPath)
}

func datRuleSetBaseName(kind string, tags []string) string {
	return safeRuleSetFilename(kind + "-" + strings.Join(tags, "-"))
}

func (s *ServiceImpl) loadRemoteRuleSetSourceJSON(ctx context.Context, rs RuleSet) ([]byte, error) {
	format := rs.Format
	if format == "" {
		format = inferFormat(rs.URL)
	}
	path, err := ruleSetDownload(ctx, rs.URL, format)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	if strings.HasSuffix(strings.ToLower(path), ".json") || format == "source" {
		return os.ReadFile(path)
	}
	return ruleSetDecompileExec(s.singboxBinary(), path)
}
