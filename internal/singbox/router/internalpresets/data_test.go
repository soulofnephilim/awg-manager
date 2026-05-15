// Regression guard: ensures the preset catalog structure stays consistent.
// Catches silent ID drift, missing categories, and invalid category values.
package internalpresets

import "testing"

func TestAll_ExpectedIDs(t *testing.T) {
	expected := map[string]bool{
		"youtube":        true,
		"google":         true,
		"discord":        true,
		"telegram":       true,
		"x":              true,
		"facebook":       true,
		"instagram":      true,
		"tiktok":         true,
		"whatsapp":       true,
		"signal":         true,
		"reddit":         true,
		"netflix":        true,
		"twitch":         true,
		"spotify":        true,
		"disney":         true,
		"hbo":            true,
		"wikimedia":      true,
		"bbc":            true,
		"category-media": true,
		"openai":         true,
		"anthropic":      true,
		"gemini":         true,
		"copilot":        true,
		"grok":           true,
		"perplexity":     true,
		"category-ai":    true,
		"github":         true,
		"gitlab":         true,
		"docker":         true,
		"linkedin":       true,
		"cloudflare":     true,
		"akamai":         true,
		"aws":            true,
		"apple":          true,
		"microsoft":      true,
		"category-games": true,
		"steam":          true,
		"playstation":    true,
		"xbox":           true,
		"roblox":         true,
		"nintendo":       true,
		"ads":            true,
		"rkn":            true,
		"unavailable-in-russia": true,
		"porn":           true,
	}

	seen := make(map[string]bool, len(expected))
	for _, p := range All() {
		seen[p.ID] = true
		if !expected[p.ID] {
			t.Errorf("unexpected preset ID %q", p.ID)
		}
	}
	for id := range expected {
		if !seen[id] {
			t.Errorf("missing preset ID %q", id)
		}
	}
}

func TestAll_NonFeaturedHasCategory(t *testing.T) {
	for _, p := range All() {
		if p.Featured || p.Sensitive {
			continue
		}
		if p.Category == "" {
			t.Errorf("preset %q: not Featured and not Sensitive but Category is empty", p.ID)
		}
	}
}

func TestAll_FeaturedNoCategory(t *testing.T) {
	for _, p := range All() {
		if !p.Featured {
			continue
		}
		if p.Category != "" {
			t.Errorf("preset %q: Featured=true but Category=%q (expected empty)", p.ID, p.Category)
		}
	}
}

func TestAll_CategoryConstantsOnly(t *testing.T) {
	valid := map[string]bool{
		CatSocial:    true,
		CatMedia:     true,
		CatAI:        true,
		CatDeveloper: true,
		CatCloud:     true,
		CatGaming:    true,
		CatBlock:     true,
	}
	for _, p := range All() {
		if p.Category == "" {
			continue
		}
		if !valid[p.Category] {
			t.Errorf("preset %q: Category=%q is not one of the seven exported constants", p.ID, p.Category)
		}
	}
}

func TestAll_NewPresets(t *testing.T) {
	newPresets := map[string]struct {
		file     string
		category string
		tag      string
	}{
		"roblox":                {"roblox", CatGaming, "geosite-roblox"},
		"nintendo":              {"nintendo", CatGaming, "geosite-nintendo"},
		"linkedin":              {"linkedin", CatDeveloper, "geosite-linkedin"},
		"copilot":               {"copilot", CatAI, "geosite-copilot"},
		"gemini":                {"gemini", CatAI, "geosite-google-gemini"},
		"grok":                  {"grok", CatAI, "geosite-grok"},
		"rkn":                   {"rkn", CatBlock, "geosite-rkn"},
		"unavailable-in-russia": {"unavailable-in-russia", CatBlock, "geosite-unavailable-in-russia"},
	}
	presets := All()
	byID := make(map[string]Preset, len(presets))
	for _, p := range presets {
		byID[p.ID] = p
	}
	for id, want := range newPresets {
		p, ok := byID[id]
		if !ok {
			t.Errorf("new preset %q missing from All()", id)
			continue
		}
		if p.Category != want.category {
			t.Errorf("preset %q category: got %s, want %s", id, p.Category, want.category)
		}
		wantURL := vernetteSRSRoot + want.file + ".srs"
		if len(p.RuleSets) == 0 || p.RuleSets[0].URL != wantURL {
			t.Errorf("preset %q URL: got %v, want %s", id, p.RuleSets, wantURL)
		}
		if len(p.RuleSets) == 0 {
			t.Errorf("preset %q has no RuleSets", id)
			continue
		}
		if p.RuleSets[0].Tag != want.tag {
			t.Errorf("preset %q tag: got %s, want %s", id, p.RuleSets[0].Tag, want.tag)
		}
	}
}

func TestAll_VernetteURLs(t *testing.T) {
	wantVernette := map[string]string{
		"telegram":  "telegram.srs",
		"whatsapp":  "whatsapp.srs",
		"discord":   "discord-full.srs",
		"youtube":   "youtube.srs",
		"x":         "x.srs",
		"tiktok":    "tiktok.srs",
		"instagram": "instagram.srs",
		"openai":    "openai.srs",
		"netflix":   "netflix.srs",
		"anthropic": "claude.srs",
	}
	presets := All()
	byID := make(map[string]Preset, len(presets))
	for _, p := range presets {
		byID[p.ID] = p
	}
	for id, file := range wantVernette {
		p, ok := byID[id]
		if !ok {
			t.Errorf("preset %q missing from All()", id)
			continue
		}
		if len(p.RuleSets) == 0 {
			t.Errorf("preset %q has no RuleSets", id)
			continue
		}
		wantURL := vernetteSRSRoot + file
		if p.RuleSets[0].URL != wantURL {
			t.Errorf("preset %q URL mismatch: got %s, want %s", id, p.RuleSets[0].URL, wantURL)
		}
		// Tag must stay "geosite-<slug>" for backward compatibility with 20-router.json.
		wantTag := "geosite-" + id
		if p.RuleSets[0].Tag != wantTag {
			t.Errorf("preset %q tag changed: got %s, want %s", id, p.RuleSets[0].Tag, wantTag)
		}
	}
}
