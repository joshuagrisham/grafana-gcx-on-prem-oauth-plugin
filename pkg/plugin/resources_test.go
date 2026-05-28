package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// ---------------------------------------------------------------------------
// fakeGrafana is an in-process stub of the subset of the Grafana HTTP API
// that this plugin's backend calls. Each test instance gets its own server
// and its own in-memory state so tests are isolated.
// ---------------------------------------------------------------------------

type fakeServiceAccount struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Login      string `json:"login"`
	Role       string `json:"role"`
	OrgID      int64  `json:"orgId"`
	IsDisabled bool   `json:"isDisabled"`
}

type fakeToken struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Key        string  `json:"key,omitempty"`
	Created    string  `json:"created,omitempty"`
	Expiration *string `json:"expiration,omitempty"`
	HasExpired bool    `json:"hasExpired"`
	LastUsedAt *string `json:"lastUsedAt,omitempty"`

	saID int64
}

type fakeGrafana struct {
	t      *testing.T
	mu     sync.Mutex
	server *httptest.Server

	nextSAID    int64
	nextTokenID int64

	serviceAccounts map[int64]*fakeServiceAccount
	tokens          map[int64]*fakeToken
	orgUsers        map[int64][]grafanaOrgUser

	requestLog []string

	wantBasicUser string
	wantBasicPass string
	wantBearer    string
}

func newFakeGrafana(t *testing.T) *fakeGrafana {
	t.Helper()
	f := &fakeGrafana{
		t:               t,
		serviceAccounts: map[int64]*fakeServiceAccount{},
		tokens:          map[int64]*fakeToken{},
		orgUsers:        map[int64][]grafanaOrgUser{},
		wantBasicUser:   "admin",
		wantBasicPass:   "admin",
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeGrafana) URL() string { return f.server.URL }

func (f *fakeGrafana) addServiceAccount(name, role string, orgID int64, disabled bool) *fakeServiceAccount {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextSAID++
	sa := &fakeServiceAccount{
		ID:         f.nextSAID,
		Name:       name,
		Login:      "sa-" + strconv.FormatInt(f.nextSAID, 10),
		Role:       role,
		OrgID:      orgID,
		IsDisabled: disabled,
	}
	f.serviceAccounts[sa.ID] = sa
	return sa
}

func (f *fakeGrafana) addToken(saID int64, name string, expiresIn time.Duration) *fakeToken {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextTokenID++
	exp := time.Now().Add(expiresIn).UTC().Format(time.RFC3339)
	tok := &fakeToken{
		ID:         f.nextTokenID,
		Name:       name,
		Created:    time.Now().UTC().Format(time.RFC3339),
		Expiration: &exp,
		HasExpired: expiresIn <= 0,
		saID:       saID,
	}
	f.tokens[tok.ID] = tok
	return tok
}

func (f *fakeGrafana) addOrgUser(orgID int64, login, role string, disabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.orgUsers[orgID] = append(f.orgUsers[orgID], grafanaOrgUser{
		OrgID: orgID, UserID: int64(len(f.orgUsers[orgID]) + 1),
		Login: login, Email: login + "@example.com", Role: role, IsDisabled: disabled,
	})
}

func (f *fakeGrafana) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	if f.wantBearer != "" {
		if r.Header.Get("Authorization") != "Bearer "+f.wantBearer {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return false
		}
		return true
	}
	if f.wantBasicUser != "" {
		u, p, ok := r.BasicAuth()
		if !ok || u != f.wantBasicUser || p != f.wantBasicPass {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return false
		}
	}
	return true
}

func (f *fakeGrafana) handle(w http.ResponseWriter, r *http.Request) {
	if !f.checkAuth(w, r) {
		return
	}
	f.mu.Lock()
	f.requestLog = append(f.requestLog, r.Method+" "+r.URL.Path)
	f.mu.Unlock()

	switch {
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/user/using/"):
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet && r.URL.Path == "/api/serviceaccounts/search":
		f.handleSearchSAs(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/serviceaccounts":
		f.handleCreateSA(w, r)
	case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/serviceaccounts/") &&
		!strings.Contains(strings.TrimPrefix(r.URL.Path, "/api/serviceaccounts/"), "/"):
		f.handlePatchSA(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/serviceaccounts/") &&
		!strings.Contains(strings.TrimPrefix(r.URL.Path, "/api/serviceaccounts/"), "/"):
		f.handleDeleteSA(w, r)
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/tokens") &&
		strings.HasPrefix(r.URL.Path, "/api/serviceaccounts/"):
		f.handleListTokens(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/tokens") &&
		strings.HasPrefix(r.URL.Path, "/api/serviceaccounts/"):
		f.handleCreateToken(w, r)
	case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/tokens/"):
		f.handleDeleteToken(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/org/users":
		f.handleListOrgUsers(w, r)
	default:
		http.Error(w, "fake-grafana: not implemented: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}
}

func (f *fakeGrafana) handleSearchSAs(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	query := strings.ToLower(r.URL.Query().Get("query"))
	var matches []fakeServiceAccount
	for _, sa := range f.serviceAccounts {
		if query == "" || strings.Contains(strings.ToLower(sa.Name), query) {
			matches = append(matches, *sa)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"serviceAccounts": matches, "totalCount": len(matches),
		"page": 1, "perPage": len(matches),
	})
}

func (f *fakeGrafana) handleCreateSA(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string `json:"name"`
		Role       string `json:"role"`
		IsDisabled bool   `json:"isDisabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	orgID := int64(1)
	if v := r.Header.Get("X-Test-Org-ID"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			orgID = n
		}
	}
	sa := f.addServiceAccount(body.Name, body.Role, orgID, body.IsDisabled)
	writeJSON(w, http.StatusOK, sa)
}

func (f *fakeGrafana) handlePatchSA(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/serviceaccounts/")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	var body struct {
		Name       string `json:"name"`
		Role       string `json:"role"`
		IsDisabled bool   `json:"isDisabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	sa, ok := f.serviceAccounts[id]
	if !ok {
		f.mu.Unlock()
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if body.Role != "" {
		sa.Role = body.Role
	}
	sa.IsDisabled = body.IsDisabled
	out := *sa
	f.mu.Unlock()
	writeJSON(w, http.StatusOK, out)
}

func (f *fakeGrafana) handleDeleteSA(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/serviceaccounts/")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.serviceAccounts[id]; !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	delete(f.serviceAccounts, id)
	for tID, t := range f.tokens {
		if t.saID == id {
			delete(f.tokens, tID)
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (f *fakeGrafana) handleListTokens(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/serviceaccounts/"), "/")
	saID, _ := strconv.ParseInt(parts[0], 10, 64)
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []fakeToken{}
	for _, t := range f.tokens {
		if t.saID == saID {
			out = append(out, *t)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (f *fakeGrafana) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/serviceaccounts/"), "/")
	saID, _ := strconv.ParseInt(parts[0], 10, 64)
	var body struct {
		Name          string `json:"name"`
		SecondsToLive int64  `json:"secondsToLive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.nextTokenID++
	exp := time.Now().Add(time.Duration(body.SecondsToLive) * time.Second).UTC().Format(time.RFC3339)
	tok := &fakeToken{
		ID: f.nextTokenID, Name: body.Name,
		Key:        fmt.Sprintf("glsa_test_%d", f.nextTokenID),
		Created:    time.Now().UTC().Format(time.RFC3339),
		Expiration: &exp, HasExpired: false, saID: saID,
	}
	f.tokens[tok.ID] = tok
	out := *tok
	f.mu.Unlock()
	writeJSON(w, http.StatusOK, out)
}

func (f *fakeGrafana) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	tokenID, _ := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.tokens[tokenID]; !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	delete(f.tokens, tokenID)
	w.WriteHeader(http.StatusOK)
}

func (f *fakeGrafana) handleListOrgUsers(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	orgID := int64(1)
	if v := r.Header.Get("X-Test-Org-ID"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			orgID = n
		}
	}
	users := f.orgUsers[orgID]
	if users == nil {
		users = []grafanaOrgUser{}
	}
	writeJSON(w, http.StatusOK, users)
}

// ---------------------------------------------------------------------------
// Harness for invoking *App handlers in-process.
// ---------------------------------------------------------------------------

const testPluginID = "joshuagrisham-gcxonpremoauth-app"

type testHarness struct {
	app  *App
	mux  *http.ServeMux
	fake *fakeGrafana
}

func newHarness(t *testing.T) *testHarness {
	t.Helper()
	fake := newFakeGrafana(t)
	setBasicAuthEnv(t, "admin", "admin")
	app := &App{
		httpClient: fake.server.Client(),
		pluginCtx: backend.PluginContext{
			PluginID: testPluginID,
			OrgID:    1,
		},
	}
	mux := http.NewServeMux()
	app.registerRoutes(mux)
	return &testHarness{app: app, mux: mux, fake: fake}
}

func setBasicAuthEnv(t *testing.T, user, pass string) {
	t.Helper()
	envName := "GF_PLUGIN_" + strings.ToUpper(strings.ReplaceAll(testPluginID, "-", "_")) + "_"
	t.Setenv(envName+"BACKEND_USERNAME", user)
	t.Setenv(envName+"BACKEND_PASSWORD", pass)
}

func pluginEnv(suffix string) string {
	return "GF_PLUGIN_" + strings.ToUpper(strings.ReplaceAll(testPluginID, "-", "_")) + "_" + suffix
}

func (h *testHarness) request(t *testing.T, method, path string, body any, user *backend.User) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	ctx := context.Background()
	ctx = backend.WithPluginContext(ctx, backend.PluginContext{
		PluginID: testPluginID, OrgID: 1,
	})
	ctx = backend.WithGrafanaConfig(ctx, backend.NewGrafanaCfg(map[string]string{
		"GF_APP_URL": h.fake.URL(),
	}))
	ctx = backend.WithUser(ctx, user)
	req := httptest.NewRequestWithContext(ctx, method, path, reader)
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, req)
	return w
}

func findFakeSAByName(f *fakeGrafana, name string) *fakeServiceAccount {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, sa := range f.serviceAccounts {
		if sa.Name == name {
			out := *sa
			return &out
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestHandleTokenCreate_CreatesServiceAccountAndToken(t *testing.T) {
	h := newHarness(t)
	w := h.request(t, http.MethodPost, "/token",
		map[string]any{"name": "my-token", "secondsToLive": 3600},
		&backend.User{Login: "alice", Role: "Editor"})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var tok token
	if err := json.Unmarshal(w.Body.Bytes(), &tok); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if tok.Key == "" {
		t.Fatalf("expected token key, got %+v", tok)
	}
	if sa := findFakeSAByName(h.fake, serviceAccountName("alice")); sa == nil || sa.Role != "Editor" {
		t.Fatalf("service account not created as expected: %+v", sa)
	}
}

func TestHandleTokenCreate_RejectsEmptyName(t *testing.T) {
	h := newHarness(t)
	w := h.request(t, http.MethodPost, "/token",
		map[string]any{"name": "   "},
		&backend.User{Login: "alice", Role: "Viewer"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
}

func TestHandleTokenCreate_RejectsTTLOverMax(t *testing.T) {
	t.Setenv(pluginEnv("TOKEN_MAX_SECONDS_TO_LIVE"), "60")
	h := newHarness(t)
	w := h.request(t, http.MethodPost, "/token",
		map[string]any{"name": "n", "secondsToLive": 120},
		&backend.User{Login: "alice", Role: "Viewer"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
}

func TestHandleTokenCreate_RejectsZeroOrNegativeTTL(t *testing.T) {
	h := newHarness(t)
	for _, ttl := range []int64{0, -1} {
		w := h.request(t, http.MethodPost, "/token",
			map[string]any{"name": "n", "secondsToLive": ttl},
			&backend.User{Login: "alice", Role: "Viewer"})
		if w.Code != http.StatusBadRequest {
			t.Errorf("ttl %d: expected 400, got %d", ttl, w.Code)
		}
	}
}

func TestHandleTokenCreate_RejectsAnonymousUser(t *testing.T) {
	h := newHarness(t)
	w := h.request(t, http.MethodPost, "/token",
		map[string]any{"name": "n", "secondsToLive": 60}, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
}

func TestHandleTokenCreate_EnforcesMaxTokensPerUser(t *testing.T) {
	t.Setenv(pluginEnv("MAX_TOKENS_PER_USER"), "1")
	h := newHarness(t)
	user := &backend.User{Login: "alice", Role: "Viewer"}

	if w := h.request(t, http.MethodPost, "/token",
		map[string]any{"name": "t1", "secondsToLive": 3600}, user); w.Code != http.StatusOK {
		t.Fatalf("first create: %d %s", w.Code, w.Body.String())
	}
	w := h.request(t, http.MethodPost, "/token",
		map[string]any{"name": "t2", "secondsToLive": 3600}, user)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d %s", w.Code, w.Body.String())
	}
}

func TestHandleTokenCreate_RejectsLongName(t *testing.T) {
	h := newHarness(t)
	w := h.request(t, http.MethodPost, "/token",
		map[string]any{"name": strings.Repeat("a", maxTokenNameLength+1)},
		&backend.User{Login: "alice", Role: "Viewer"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d %s", w.Code, w.Body.String())
	}
}

func TestHandleTokenList(t *testing.T) {
	h := newHarness(t)
	sa := h.fake.addServiceAccount(serviceAccountName("alice"), "Viewer", 1, false)
	h.fake.addToken(sa.ID, "t1", time.Hour)
	h.fake.addToken(sa.ID, "t2", -time.Hour)

	w := h.request(t, http.MethodGet, "/tokens", nil,
		&backend.User{Login: "alice", Role: "Viewer"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var got []token
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 tokens, got %d (%+v)", len(got), got)
	}
}

func TestHandleTokenDelete_RejectsForeignToken(t *testing.T) {
	h := newHarness(t)
	_ = h.fake.addServiceAccount(serviceAccountName("alice"), "Viewer", 1, false)
	saBob := h.fake.addServiceAccount(serviceAccountName("bob"), "Viewer", 1, false)
	bobTok := h.fake.addToken(saBob.ID, "bobs-token", time.Hour)

	w := h.request(t, http.MethodDelete, fmt.Sprintf("/tokens/%d", bobTok.ID), nil,
		&backend.User{Login: "alice", Role: "Viewer"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d %s", w.Code, w.Body.String())
	}
}

func TestHandleTokenDelete_Success(t *testing.T) {
	h := newHarness(t)
	sa := h.fake.addServiceAccount(serviceAccountName("alice"), "Viewer", 1, false)
	tok := h.fake.addToken(sa.ID, "t1", time.Hour)

	w := h.request(t, http.MethodDelete, fmt.Sprintf("/tokens/%d", tok.ID), nil,
		&backend.User{Login: "alice", Role: "Viewer"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if _, exists := h.fake.tokens[tok.ID]; exists {
		t.Errorf("token should have been deleted from fake server")
	}
}

func TestHandleTokenDelete_RejectsInvalidID(t *testing.T) {
	h := newHarness(t)
	w := h.request(t, http.MethodDelete, "/tokens/-5", nil,
		&backend.User{Login: "alice", Role: "Viewer"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d %s", w.Code, w.Body.String())
	}
}

func TestFindOrCreateServiceAccount_ReconcilesRoleChange(t *testing.T) {
	h := newHarness(t)
	h.fake.addServiceAccount(serviceAccountName("alice"), "Viewer", 1, false)

	w := h.request(t, http.MethodGet, "/tokens", nil,
		&backend.User{Login: "alice", Role: "Editor"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	sa := findFakeSAByName(h.fake, serviceAccountName("alice"))
	if sa == nil || sa.Role != "Editor" {
		t.Fatalf("expected role reconciled to Editor, got %+v", sa)
	}
}
