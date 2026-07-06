package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoaxisr/awg-manager/internal/singbox/orchestrator"
)

// newEditorHandler строит хендлер над реальным оркестратором в tmpdir
// (validator nil — sing-box check пропускается, кросс-слот валидация
// работает полностью). Регистрируются router (системный источник тегов)
// и user; router включается через файл в active/.
func newEditorHandler(t *testing.T) (*SingboxConfigEditorHandler, *orchestrator.Orchestrator, string) {
	t.Helper()
	dir := t.TempDir()
	o := orchestrator.New(dir, nil)
	for _, meta := range orchestrator.KnownSlots() {
		if meta.Slot == orchestrator.SlotRouter || meta.Slot == orchestrator.SlotUser {
			if err := o.Register(meta); err != nil {
				t.Fatalf("register %s: %v", meta.Slot, err)
			}
		}
	}
	if err := o.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return NewSingboxConfigEditorHandler(o), o, dir
}

func decodeEnvelope(t *testing.T, body []byte, data any) {
	t.Helper()
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, body)
	}
	if !env.Success {
		t.Fatalf("expected success=true: %s", body)
	}
	if data != nil {
		if err := json.Unmarshal(env.Data, data); err != nil {
			t.Fatalf("decode data: %v\n%s", err, env.Data)
		}
	}
}

func TestConfigEditor_ListSlots_Shape(t *testing.T) {
	h, o, dir := newEditorHandler(t)
	// router: применённый файл; user: только черновик.
	if err := os.WriteFile(filepath.Join(dir, "20-router.json"), []byte(`{"route":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := o.SetEnabled(orchestrator.SlotRouter, true); err != nil {
		t.Fatal(err)
	}
	if err := o.SaveDraft(orchestrator.SlotUser, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.ListSlots(rec, httptest.NewRequest(http.MethodGet, "/api/singbox/config/slots", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var data ConfigSlotsResponse
	decodeEnvelope(t, rec.Body.Bytes(), &data)
	if len(data.Slots) != 2 {
		t.Fatalf("slots = %d, want 2 (registered only)", len(data.Slots))
	}
	byName := map[string]ConfigSlotInfo{}
	for _, s := range data.Slots {
		byName[s.Slot] = s
	}
	router := byName["router"]
	if router.Ownership != "system" || router.Filename != "20-router.json" || !router.Enabled || router.HasDraft {
		t.Errorf("router slot info: %+v", router)
	}
	if router.Size == 0 || router.MTime == "" {
		t.Errorf("router size/mtime not populated: %+v", router)
	}
	user := byName["user"]
	if user.Ownership != "user" || user.Filename != "90-user.json" || !user.HasDraft {
		t.Errorf("user slot info: %+v", user)
	}
}

func TestConfigEditor_GetSlot_ReadsEffectiveAndState(t *testing.T) {
	h, o, dir := newEditorHandler(t)
	if err := os.WriteFile(filepath.Join(dir, "20-router.json"), []byte(`{"route":{"final":"direct"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := o.SetEnabled(orchestrator.SlotRouter, true); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.GetSlot(rec, httptest.NewRequest(http.MethodGet, "/api/singbox/config/slot?name=router", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var data ConfigSlotContentResponse
	decodeEnvelope(t, rec.Body.Bytes(), &data)
	if data.State != "active" || !strings.Contains(data.Content, `"final"`) || data.HasDraft {
		t.Errorf("slot content: %+v", data)
	}

	// Неизвестный слот → 404.
	rec = httptest.NewRecorder()
	h.GetSlot(rec, httptest.NewRequest(http.MethodGet, "/api/singbox/config/slot?name=nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown slot status = %d, want 404", rec.Code)
	}

	// Несконфигурированный user-слот → absent, пустой content.
	rec = httptest.NewRecorder()
	h.GetSlot(rec, httptest.NewRequest(http.MethodGet, "/api/singbox/config/slot?name=user", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("user slot status %d: %s", rec.Code, rec.Body.String())
	}
	decodeEnvelope(t, rec.Body.Bytes(), &data)
	if data.State != "absent" || data.Content != "" {
		t.Errorf("user slot content: %+v", data)
	}
}

func TestConfigEditor_PutUser_InvalidJSON400(t *testing.T) {
	h, _, _ := newEditorHandler(t)
	for name, body := range map[string]string{
		"malformed":  `{"outbounds":`,
		"not-object": `[1,2,3]`,
		"empty":      ``,
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/singbox/config/user", strings.NewReader(body))
		h.PutUserConfig(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400 (%s)", name, rec.Code, rec.Body.String())
		}
	}
}

func TestConfigEditor_PutAndApply_HappyPath(t *testing.T) {
	h, o, dir := newEditorHandler(t)
	body := `{"outbounds":[{"type":"direct","tag":"user-direct"}]}`

	rec := httptest.NewRecorder()
	h.PutUserConfig(rec, httptest.NewRequest(http.MethodPut, "/api/singbox/config/user", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status %d: %s", rec.Code, rec.Body.String())
	}
	if !o.HasDraft(orchestrator.SlotUser) {
		t.Fatal("draft missing after PUT")
	}
	// Applied-файла ещё нет — черновик инертен.
	if _, err := os.Stat(filepath.Join(dir, "90-user.json")); !os.IsNotExist(err) {
		t.Fatalf("active file must not exist before apply: %v", err)
	}

	rec = httptest.NewRecorder()
	h.ApplyUserConfig(rec, httptest.NewRequest(http.MethodPost, "/api/singbox/config/user/apply", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply status %d: %s", rec.Code, rec.Body.String())
	}
	got, err := os.ReadFile(filepath.Join(dir, "90-user.json"))
	if err != nil {
		t.Fatalf("active file after apply: %v", err)
	}
	if string(got) != body {
		t.Errorf("active content: %s", got)
	}
	if o.HasDraft(orchestrator.SlotUser) {
		t.Error("draft survived apply")
	}
}

func TestConfigEditor_Apply_NoDraft409(t *testing.T) {
	h, _, _ := newEditorHandler(t)
	rec := httptest.NewRecorder()
	h.ApplyUserConfig(rec, httptest.NewRequest(http.MethodPost, "/api/singbox/config/user/apply", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "NO_DRAFT") {
		t.Errorf("expected NO_DRAFT code: %s", rec.Body.String())
	}
}

func TestConfigEditor_Apply_ValidationFailure422(t *testing.T) {
	h, o, _ := newEditorHandler(t)
	if err := o.SaveDraft(orchestrator.SlotUser,
		[]byte(`{"route":{"rules":[{"outbound":"ghost-tag"}]}}`)); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ApplyUserConfig(rec, httptest.NewRequest(http.MethodPost, "/api/singbox/config/user/apply", nil))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", rec.Code, rec.Body.String())
	}
	var body RouterStagingValidationError
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Validation == nil || len(body.Validation.Errors) == 0 {
		t.Fatalf("expected validation errors: %s", rec.Body.String())
	}
	e := body.Validation.Errors[0]
	if e.Slot != "user" || e.Kind != "unknown-outbound" || e.Tag != "ghost-tag" {
		t.Errorf("error attribution: %+v", e)
	}
	// Черновик сохранён для дальнейшего редактирования.
	if !o.HasDraft(orchestrator.SlotUser) {
		t.Error("draft must survive failed apply")
	}
}

func TestConfigEditor_Check_BodyVsDraftVs409(t *testing.T) {
	h, o, _ := newEditorHandler(t)

	// Ни тела, ни черновика → 409.
	rec := httptest.NewRecorder()
	h.CheckUserConfig(rec, httptest.NewRequest(http.MethodPost, "/api/singbox/config/user/check", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("no body no draft: status = %d, want 409", rec.Code)
	}

	// Тело задано → проверяем его (провал = 200 {ok:false}, не 422).
	rec = httptest.NewRecorder()
	h.CheckUserConfig(rec, httptest.NewRequest(http.MethodPost, "/api/singbox/config/user/check",
		strings.NewReader(`{"route":{"rules":[{"outbound":"ghost-tag"}]}}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("check body: status = %d: %s", rec.Code, rec.Body.String())
	}
	var data UserConfigCheckResponse
	decodeEnvelope(t, rec.Body.Bytes(), &data)
	if data.Ok || len(data.Errors) == 0 || data.Errors[0].Tag != "ghost-tag" {
		t.Errorf("check body result: %+v", data)
	}

	// Некорректное тело → 400.
	rec = httptest.NewRecorder()
	h.CheckUserConfig(rec, httptest.NewRequest(http.MethodPost, "/api/singbox/config/user/check",
		strings.NewReader(`[]`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("check invalid body: status = %d, want 400", rec.Code)
	}

	// Без тела с черновиком → проверяем черновик.
	if err := o.SaveDraft(orchestrator.SlotUser, []byte(`{"outbounds":[{"type":"direct","tag":"ok-tag"}]}`)); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	h.CheckUserConfig(rec, httptest.NewRequest(http.MethodPost, "/api/singbox/config/user/check", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("check draft: status = %d: %s", rec.Code, rec.Body.String())
	}
	data = UserConfigCheckResponse{} // omitempty: не наследовать errors прошлого ответа
	decodeEnvelope(t, rec.Body.Bytes(), &data)
	if !data.Ok || len(data.Errors) != 0 {
		t.Errorf("check draft result: %+v", data)
	}
}

// Дубликат тега между user-слотом и системным слотом всплывает в check.
func TestConfigEditor_Check_CollisionWithSystemSlot(t *testing.T) {
	h, o, dir := newEditorHandler(t)
	if err := os.WriteFile(filepath.Join(dir, "20-router.json"),
		[]byte(`{"outbounds":[{"type":"direct","tag":"shared-tag"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := o.SetEnabled(orchestrator.SlotRouter, true); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.CheckUserConfig(rec, httptest.NewRequest(http.MethodPost, "/api/singbox/config/user/check",
		strings.NewReader(`{"outbounds":[{"type":"direct","tag":"shared-tag"}]}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var data UserConfigCheckResponse
	decodeEnvelope(t, rec.Body.Bytes(), &data)
	if data.Ok {
		t.Fatal("expected collision failure")
	}
	found := false
	for _, e := range data.Errors {
		if e.Kind == "duplicate-outbound" && e.Tag == "shared-tag" {
			found = true
		}
	}
	if !found {
		t.Errorf("duplicate-outbound not surfaced: %+v", data.Errors)
	}
}

func TestConfigEditor_EnableToggle(t *testing.T) {
	h, o, dir := newEditorHandler(t)
	// Применённый user-файл.
	if err := o.SaveDraft(orchestrator.SlotUser, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ApplyUserConfig(rec, httptest.NewRequest(http.MethodPost, "/api/singbox/config/user/apply", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body.String())
	}

	// Выключение паркует файл в disabled/.
	rec = httptest.NewRecorder()
	h.EnableUserConfig(rec, httptest.NewRequest(http.MethodPost, "/api/singbox/config/user/enable",
		strings.NewReader(`{"enabled":false}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "disabled", "90-user.json")); err != nil {
		t.Errorf("parked file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "90-user.json")); !os.IsNotExist(err) {
		t.Errorf("active file must be gone: %v", err)
	}

	// Обратно.
	rec = httptest.NewRecorder()
	h.EnableUserConfig(rec, httptest.NewRequest(http.MethodPost, "/api/singbox/config/user/enable",
		strings.NewReader(`{"enabled":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("enable: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "90-user.json")); err != nil {
		t.Errorf("active file missing after enable: %v", err)
	}
}

// Применение к выключенному слоту включает его (иначе файл в active/ при
// enabled=false — validate слот пропустит, а sing-box файл прочитает).
func TestConfigEditor_Apply_EnablesParkedSlot(t *testing.T) {
	h, o, dir := newEditorHandler(t)
	if err := o.SaveDraft(orchestrator.SlotUser, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ApplyUserConfig(rec, httptest.NewRequest(http.MethodPost, "/api/singbox/config/user/apply", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.EnableUserConfig(rec, httptest.NewRequest(http.MethodPost, "/api/singbox/config/user/enable",
		strings.NewReader(`{"enabled":false}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("disable: %d", rec.Code)
	}

	// Новый черновик поверх припаркованного слота → apply включает слот.
	if err := o.SaveDraft(orchestrator.SlotUser, []byte(`{"outbounds":[]}`)); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	h.ApplyUserConfig(rec, httptest.NewRequest(http.MethodPost, "/api/singbox/config/user/apply", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("re-apply: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "90-user.json")); err != nil {
		t.Errorf("active file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "disabled", "90-user.json")); !os.IsNotExist(err) {
		t.Errorf("stale disabled copy: %v", err)
	}
}

func TestConfigEditor_Discard(t *testing.T) {
	h, o, _ := newEditorHandler(t)
	if err := o.SaveDraft(orchestrator.SlotUser, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.DiscardUserConfig(rec, httptest.NewRequest(http.MethodPost, "/api/singbox/config/user/discard", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("discard: %d %s", rec.Code, rec.Body.String())
	}
	if o.HasDraft(orchestrator.SlotUser) {
		t.Error("draft survived discard")
	}
	// Идемпотентно.
	rec = httptest.NewRecorder()
	h.DiscardUserConfig(rec, httptest.NewRequest(http.MethodPost, "/api/singbox/config/user/discard", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("second discard: %d", rec.Code)
	}
}

func TestConfigEditor_MethodChecks(t *testing.T) {
	h, _, _ := newEditorHandler(t)
	cases := []struct {
		name string
		fn   http.HandlerFunc
		bad  string
	}{
		{"slots", h.ListSlots, http.MethodPost},
		{"slot", h.GetSlot, http.MethodPost},
		{"put-user", h.PutUserConfig, http.MethodGet},
		{"check", h.CheckUserConfig, http.MethodGet},
		{"apply", h.ApplyUserConfig, http.MethodGet},
		{"discard", h.DiscardUserConfig, http.MethodGet},
		{"enable", h.EnableUserConfig, http.MethodGet},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		c.fn(rec, httptest.NewRequest(c.bad, "/x", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want 405", c.name, rec.Code)
		}
	}
}
