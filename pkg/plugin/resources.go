package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// ---------------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------------

func (a *App) handleTokenCreate(w http.ResponseWriter, req *http.Request) {
	pCtx := backend.PluginConfigFromContext(req.Context())
	gCfg := backend.GrafanaConfigFromContext(req.Context())
	user := backend.UserFromContext(req.Context())

	var body struct {
		Name          string `json:"name"`
		SecondsToLive *int64 `json:"secondsToLive,omitempty"`
	}
	if err := json.NewDecoder(io.LimitReader(req.Body, 64<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: %v", err)
		return
	}

	// Token name validation.
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "token name is required")
		return
	}
	if len(body.Name) > maxTokenNameLength {
		writeError(w, http.StatusBadRequest, "token name must be %d characters or fewer", maxTokenNameLength)
		return
	}

	// Reject TTLs that are zero, negative, or larger than the maximum allowed.
	maxTTL := a.maxTokenTTL()
	if body.SecondsToLive != nil && (*body.SecondsToLive <= 0 || *body.SecondsToLive > maxTTL) {
		writeError(w, http.StatusBadRequest, "secondsToLive must be an integer between 1 (1s) and %d (%s)", maxTTL, (time.Duration(maxTTL) * time.Second).String())
		return
	}

	// If the user requested a specific secondsToLive, use it; otherwise, use the plugin's default.
	ttl := maxTTL
	if body.SecondsToLive != nil {
		ttl = *body.SecondsToLive
	}

	sa, err := a.findOrCreateServiceAccount(req.Context(), gCfg, pCtx, user)
	if err != nil {
		writeAPIError(w, err)
		return
	}

	// Reject if the user has already reached their max per-user token limit.
	if maxTokensPerUser := a.maxTokensPerUser(); maxTokensPerUser > 0 {
		tokens, err := a.listTokens(req.Context(), gCfg, pCtx, sa)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		activeUserTokens := int64(0)
		for _, t := range tokens {
			if !t.HasExpired {
				activeUserTokens++
			}
		}
		if activeUserTokens >= maxTokensPerUser {
			writeError(w, http.StatusTooManyRequests,
				"token limit reached (used %d of %d allowed active tokens); delete an existing token before creating a new one", activeUserTokens, maxTokensPerUser)
			return
		}
	}

	t, err := a.createToken(req.Context(), gCfg, pCtx, sa, body.Name, ttl)
	if err != nil {
		writeAPIError(w, err)
		return
	}

	backend.Logger.Info("Token created",
		"orgId", pCtx.OrgID, "userLogin", user.Login,
		"serviceAccountId", sa.ID, "serviceAccountName", sa.Name,
		"tokenId", t.ID, "tokenName", t.Name,
		"secondsToLive", fmt.Sprintf("%d", ttl))

	writeJSON(w, http.StatusOK, t)
}

func (a *App) handleTokenList(w http.ResponseWriter, req *http.Request) {
	pCtx := backend.PluginConfigFromContext(req.Context())
	gCfg := backend.GrafanaConfigFromContext(req.Context())
	user := backend.UserFromContext(req.Context())

	sa, err := a.findOrCreateServiceAccount(req.Context(), gCfg, pCtx, user)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	tokens, err := a.listTokens(req.Context(), gCfg, pCtx, sa)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tokens)
}

func (a *App) handleTokenDelete(w http.ResponseWriter, req *http.Request) {
	pCtx := backend.PluginConfigFromContext(req.Context())
	gCfg := backend.GrafanaConfigFromContext(req.Context())
	user := backend.UserFromContext(req.Context())

	tokenID, err := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err != nil || tokenID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid token ID")
		return
	}

	sa, err := a.findOrCreateServiceAccount(req.Context(), gCfg, pCtx, user)
	if err != nil {
		writeAPIError(w, err)
		return
	}

	// Confirm the token actually belongs to this user's SA before deleting.
	tokens, err := a.listTokens(req.Context(), gCfg, pCtx, sa)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	found := false
	for _, t := range tokens {
		if t.ID == tokenID {
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}

	if err := a.deleteToken(req.Context(), gCfg, pCtx, sa, tokenID); err != nil {
		writeAPIError(w, err)
		return
	}
	backend.Logger.Info("Token deleted",
		"orgId", pCtx.OrgID, "userLogin", user.Login,
		"serviceAccountId", sa.ID, "serviceAccountName", sa.Name,
		"tokenId", tokenID)

	tokens, err = a.listTokens(req.Context(), gCfg, pCtx, sa)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tokens)
}

// registerRoutes registers the plugin's resource HTTP handlers.
func (a *App) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /token", a.handleTokenCreate)
	mux.HandleFunc("GET /tokens", a.handleTokenList)
	mux.HandleFunc("DELETE /tokens/{id}", a.handleTokenDelete)
}

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

type serviceAccount struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Login      string `json:"login"`
	Role       string `json:"role"`
	OrgID      int64  `json:"orgId"`
	IsDisabled bool   `json:"isDisabled"`
}

type token struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Key        string  `json:"key,omitempty"`
	Created    string  `json:"created,omitempty"`
	Expiration *string `json:"expiration,omitempty"`
	HasExpired bool    `json:"hasExpired"`
	LastUsedAt *string `json:"lastUsedAt,omitempty"`
}

// ---------------------------------------------------------------------------
// Service account + token operations
// ---------------------------------------------------------------------------

func serviceAccountName(login string) string {
	return serviceAccountPrefix + login
}

func (a *App) findOrCreateServiceAccount(ctx context.Context, gCfg *backend.GrafanaCfg, pCtx backend.PluginContext, user *backend.User) (*serviceAccount, error) {
	if user == nil || user.Login == "" {
		return nil, httpError{status: http.StatusUnauthorized, err: errors.New("this feature is unavailable for anonymous users; please sign in with a user account and try again")}
	}
	name := serviceAccountName(user.Login)

	sa, err := a.findServiceAccountByName(ctx, gCfg, pCtx, name)
	if err != nil {
		return nil, err
	}

	desired := map[string]any{
		"name":       name,
		"role":       user.Role,
		"isDisabled": false,
	}

	if sa != nil {
		if sa.Role == user.Role && !sa.IsDisabled {
			return sa, nil
		}
		// Role changed or SA was disabled: patch it back to the desired
		// state. Tokens issued by a service account inherit the SA's
		// current permissions on each request, so a role downgrade
		// takes effect immediately for new requests using existing
		// tokens. (Grafana evaluates permissions at request time, not
		// at token issue time, so no token revocation is necessary.)
		patchReq := &apiRequest{
			Method: http.MethodPatch,
			Path:   fmt.Sprintf("/api/serviceaccounts/%d", sa.ID),
			Body:   desired,
		}
		if err := a.grafanaAPIRequestWithOrgSwitch(ctx, gCfg, pCtx, patchReq, sa); err != nil {
			return nil, fmt.Errorf("updating service account: %w", err)
		}
		backend.Logger.Info("Service account reconciled",
			"orgId", pCtx.OrgID, "userLogin", user.Login,
			"serviceAccountId", sa.ID, "serviceAccountName", sa.Name,
			"role", sa.Role)
		return sa, nil
	}

	// Create a new SA for the user.
	createBody := map[string]any{
		"name":       name,
		"role":       user.Role,
		"isDisabled": false,
	}
	createReq := &apiRequest{
		Method: http.MethodPost,
		Path:   "/api/serviceaccounts",
		Body:   createBody,
	}
	var created serviceAccount
	if err := a.grafanaAPIRequestWithOrgSwitch(ctx, gCfg, pCtx, createReq, &created); err != nil {
		// Race: another concurrent request may have created the SA
		// after our search returned empty. Retry the lookup once.
		if existing, lookupErr := a.findServiceAccountByName(ctx, gCfg, pCtx, name); lookupErr == nil && existing != nil {
			return existing, nil
		}
		return nil, fmt.Errorf("creating service account: %w", err)
	}
	backend.Logger.Info("Service account created",
		"orgId", pCtx.OrgID, "userLogin", user.Login,
		"serviceAccountId", created.ID, "serviceAccountName", created.Name,
		"role", created.Role)
	return &created, nil
}

// findServiceAccountByName paginates through /api/serviceaccounts/search
// looking for an exact (Name, OrgID) match. Returns (nil, nil) when not
// found.
func (a *App) findServiceAccountByName(ctx context.Context, gCfg *backend.GrafanaCfg, pCtx backend.PluginContext, name string) (*serviceAccount, error) {
	const perPage = 100
	for page := 1; page <= 100; page++ {
		searchReq := &apiRequest{
			Method: http.MethodGet,
			Path:   "/api/serviceaccounts/search",
			Query: map[string]string{
				"perpage": strconv.Itoa(perPage),
				"page":    strconv.Itoa(page),
				// query is fuzzy/substring; we re-verify exact match below
				"query": name,
			},
		}
		var resp struct {
			ServiceAccounts []serviceAccount `json:"serviceAccounts"`
			TotalCount      int              `json:"totalCount"`
			Page            int              `json:"page"`
			PerPage         int              `json:"perPage"`
		}
		if err := a.grafanaAPIRequestWithOrgSwitch(ctx, gCfg, pCtx, searchReq, &resp); err != nil {
			return nil, fmt.Errorf("searching service accounts: %w", err)
		}
		for i := range resp.ServiceAccounts {
			sa := &resp.ServiceAccounts[i]
			if sa.Name == name && sa.OrgID == pCtx.OrgID {
				return sa, nil
			}
		}
		if len(resp.ServiceAccounts) < perPage {
			return nil, nil
		}
	}
	return nil, errors.New("service account search exceeded pagination limit")
}

func (a *App) createToken(ctx context.Context, gCfg *backend.GrafanaCfg, pCtx backend.PluginContext, sa *serviceAccount, name string, secondsToLive int64) (*token, error) {
	body := map[string]any{"name": name, "secondsToLive": secondsToLive}
	req := &apiRequest{
		Method: http.MethodPost,
		Path:   fmt.Sprintf("/api/serviceaccounts/%d/tokens", sa.ID),
		Body:   body,
	}
	var t token
	if err := a.grafanaAPIRequest(ctx, gCfg, pCtx, req, &t); err != nil {
		return nil, fmt.Errorf("creating token: %w", err)
	}
	return &t, nil
}

func (a *App) listTokens(ctx context.Context, gCfg *backend.GrafanaCfg, pCtx backend.PluginContext, sa *serviceAccount) ([]token, error) {
	req := &apiRequest{
		Method: http.MethodGet,
		Path:   fmt.Sprintf("/api/serviceaccounts/%d/tokens", sa.ID),
	}
	var out []token
	if err := a.grafanaAPIRequest(ctx, gCfg, pCtx, req, &out); err != nil {
		return nil, fmt.Errorf("listing tokens: %w", err)
	}
	return out, nil
}

func (a *App) deleteToken(ctx context.Context, gCfg *backend.GrafanaCfg, pCtx backend.PluginContext, sa *serviceAccount, tokenID int64) error {
	req := &apiRequest{
		Method: http.MethodDelete,
		Path:   fmt.Sprintf("/api/serviceaccounts/%d/tokens/%d", sa.ID, tokenID),
	}
	if err := a.grafanaAPIRequest(ctx, gCfg, pCtx, req, nil); err != nil {
		return fmt.Errorf("deleting token: %w", err)
	}
	return nil
}

func (a *App) deleteServiceAccount(ctx context.Context, gCfg *backend.GrafanaCfg, pCtx backend.PluginContext, saID int64) error {
	req := &apiRequest{
		Method: http.MethodDelete,
		Path:   fmt.Sprintf("/api/serviceaccounts/%d", saID),
	}
	if err := a.grafanaAPIRequest(ctx, gCfg, pCtx, req, nil); err != nil {
		return fmt.Errorf("deleting service account: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Grafana API client
// ---------------------------------------------------------------------------

type apiRequest struct {
	Method string
	Path   string
	Query  map[string]string
	Body   any
}

func (a *App) grafanaAPIRequest(ctx context.Context, gCfg *backend.GrafanaCfg, pCtx backend.PluginContext, attrs *apiRequest, out any) error {
	if gCfg == nil {
		return errors.New("missing Grafana config in context")
	}
	if pCtx.PluginID == "" {
		pCtx = a.pluginCtx
	}

	appURL, err := gCfg.AppURL()
	if err != nil {
		return err
	}
	reqURL, err := url.JoinPath(appURL, attrs.Path)
	if err != nil {
		return err
	}

	var bodyReader io.Reader
	if attrs.Body != nil {
		raw, err := json.Marshal(attrs.Body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, attrs.Method, reqURL, bodyReader)
	if err != nil {
		return err
	}
	if attrs.Query != nil {
		q := req.URL.Query()
		for k, v := range attrs.Query {
			q.Add(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}
	req.Header.Set("Accept", "application/json")
	if attrs.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := a.setGrafanaAuthHeader(pCtx, req); err != nil {
		return err
	}

	backend.Logger.Debug("Grafana API request",
		"method", attrs.Method, "url", reqURL, "orgId", pCtx.OrgID)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	const maxBody = 10 << 20
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return httpError{status: resp.StatusCode, err: fmt.Errorf("%s %s: %s", attrs.Method, attrs.Path, strings.TrimSpace(string(raw)))}
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("parsing %s response: %w", attrs.Path, err)
	}
	return nil

}

// grafanaAPIRequestWithOrgSwitch wraps grafanaAPIRequest with a locking Basic Auth user org switch
// when necessary to ensure that the request is executed in the correct org context. This is
// required for any actions that involve searching, creating, or updating Service Accounts, as the
// Service Account API unfortunately does not respect the X-Grafana-Org-Id request header.
// See https://grafana.com/docs/grafana/latest/developer-resources/api-reference/http-api/examples/create-api-tokens-for-org/
// for more information.
func (a *App) grafanaAPIRequestWithOrgSwitch(ctx context.Context, gCfg *backend.GrafanaCfg, pCtx backend.PluginContext, attrs *apiRequest, out any) error {
	if gCfg == nil {
		return errors.New("missing Grafana config in context")
	}
	if pCtx.PluginID == "" {
		pCtx = a.pluginCtx
	}

	requiresSwitch := true
	if pCtx.AppInstanceSettings != nil {
		if t, ok := pCtx.AppInstanceSettings.DecryptedSecureJSONData["token"]; ok && t != "" {
			// Org-scoped SA token is already pinned to its org.
			requiresSwitch = false
		}
	} else {
		if _, _, ok := a.backendBasicAuth(); !ok {
			return fmt.Errorf("missing authentication credentials for orgId %d; please contact an administrator", pCtx.OrgID)
		}
	}

	// If the plugin's auth mode does not require us to switch the Basic Auth
	// user's session, we can just make the request directly without locking.
	if !requiresSwitch {
		return a.grafanaAPIRequest(ctx, gCfg, pCtx, attrs, out)
	}

	// Otherwise, we need to wrap this request in a lock, switch the Basic Auth
	// user's session, execute the request, and then release the lock.
	// This will help to prevent race conditions with any other requests that
	// might be executed by other users at the same time.

	basicAuthOrgSwitchMutex.Lock()
	defer basicAuthOrgSwitchMutex.Unlock()

	req := &apiRequest{
		Method: http.MethodPost,
		Path:   fmt.Sprintf("/api/user/using/%d", pCtx.OrgID),
	}
	if err := a.grafanaAPIRequest(ctx, gCfg, pCtx, req, nil); err != nil {
		return fmt.Errorf("switching user org context to %d: %w", pCtx.OrgID, err)
	}

	return a.grafanaAPIRequest(ctx, gCfg, pCtx, attrs, out)
}

func (a *App) setGrafanaAuthHeader(pCtx backend.PluginContext, req *http.Request) error {
	// Org-scoped service account token configured via the plugin's settings page.
	if pCtx.AppInstanceSettings != nil {
		if t, ok := pCtx.AppInstanceSettings.DecryptedSecureJSONData["token"]; ok && t != "" {
			req.Header.Set("Authorization", "Bearer "+t)
			return nil
		}
	}

	// Basic Auth (GrafanaAdmin) fallback.
	if username, password, ok := a.backendBasicAuth(); ok {
		req.SetBasicAuth(username, password)
		return nil
	}

	return fmt.Errorf("no Grafana API credentials configured for this plugin instance in orgId %d", pCtx.OrgID)
}

// ---------------------------------------------------------------------------
// Response helpers
// ---------------------------------------------------------------------------

// httpError is an internal error type that lets handlers map upstream
// Grafana status codes through to the client cleanly.
type httpError struct {
	status int
	err    error
}

func (e httpError) Error() string { return e.err.Error() }
func (e httpError) Unwrap() error { return e.err }

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		backend.Logger.Warn("Failed to encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, format string, args ...any) {
	writeJSON(w, status, map[string]string{"error": fmt.Sprintf(format, args...)})
}

func writeAPIError(w http.ResponseWriter, err error) {
	var he httpError
	if errors.As(err, &he) {
		writeError(w, he.status, "%s", he.err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, "%s", err.Error())
}
