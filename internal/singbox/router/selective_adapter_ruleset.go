package router

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hoaxisr/awg-manager/internal/singbox/router/selective"
)

// openSelectiveRuleSetJSON returns a streamable JSON file path for a rule-set ref.
// SRS sources are decompiled to a temp file; callers must invoke cleanup when done.
func (s *ServiceImpl) openSelectiveRuleSetJSON(ctx context.Context, ref selective.RuleSetRef) (string, func(), error) {
	noop := func() {}

	if jsonPath := selectiveJSONPath(ref); jsonPath != "" {
		if _, err := os.Stat(jsonPath); err == nil {
			return jsonPath, noop, nil
		}
	}

	rs := ruleSetFromRef(ref)

	if rs.Type == "inline" && ref.InlineDir != "" {
		p := filepath.Join(ref.InlineDir, safeRuleSetFilename(rs.Tag)+".json")
		if fileReadable(p) {
			return p, noop, nil
		}
	}

	if path := strings.TrimSpace(rs.Path); path != "" {
		if strings.HasSuffix(strings.ToLower(path), ".json") && fileReadable(path) {
			return path, noop, nil
		}
		if strings.HasSuffix(strings.ToLower(path), ".srs") {
			return s.decompileSRSToTemp(s.singboxBinary(), path)
		}
	}

	if rs.Type == "remote" && strings.TrimSpace(rs.URL) != "" {
		if kind, tags, ok := parseDatRuleSetURL(rs.URL); ok {
			jsonPath, err := s.ensureDatRuleSetJSONPath(ctx, ref.DatDir, kind, tags)
			return jsonPath, noop, err
		}
		format := rs.Format
		if format == "" {
			format = inferFormat(rs.URL)
		}
		localPath, err := ruleSetDownload(ctx, rs.URL, format)
		if err != nil {
			return "", noop, fmt.Errorf("download: %w", err)
		}
		if strings.HasSuffix(strings.ToLower(localPath), ".json") || format == "source" {
			return localPath, noop, nil
		}
		return s.decompileSRSToTemp(s.singboxBinary(), localPath)
	}

	if rs.Type == "remote" && ref.DatDir != "" {
		p := filepath.Join(ref.DatDir, safeRuleSetFilename(rs.Tag)+".json")
		if fileReadable(p) {
			return p, noop, nil
		}
	}

	return "", noop, nil
}

func (s *ServiceImpl) decompileSRSToTemp(binary, srsPath string) (string, func(), error) {
	path, err := ruleSetDecompileToFile(binary, srsPath)
	if err != nil {
		return "", func() {}, err
	}
	return path, func() { _ = os.Remove(path) }, nil
}

func (s *ServiceImpl) ensureDatRuleSetJSONPath(ctx context.Context, datDir, kind string, tags []string) (string, error) {
	if datDir == "" {
		return "", fmt.Errorf("dat rule-set dir unavailable")
	}
	jsonPath := filepath.Join(datDir, datRuleSetBaseName(kind, tags)+".json")
	if fileReadable(jsonPath) {
		return jsonPath, nil
	}
	token, err := s.ensureDatRuleSetToken()
	if err != nil {
		return "", err
	}
	if _, err := s.DatRuleSetFile(ctx, kind, tags, token); err != nil {
		return "", err
	}
	if !fileReadable(jsonPath) {
		return "", fmt.Errorf("dat rule-set json missing after materialize: %s", jsonPath)
	}
	return jsonPath, nil
}

func ruleSetFromRef(ref selective.RuleSetRef) RuleSet {
	return RuleSet{
		Tag:    ref.Tag,
		Type:   ref.Type,
		Path:   ref.Path,
		URL:    ref.URL,
		Format: ref.Format,
	}
}
