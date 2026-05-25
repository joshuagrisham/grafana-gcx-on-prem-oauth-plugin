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
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

func (a *App) handleTokenCreate(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Name          string `json:"name"`
		SecondsToLive *int64 `json:"secondsToLive,omitempty"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pcfg := backend.PluginConfigFromContext(req.Context())

	// Enforce max token TTL from environment variable; defaults to 30 days
	maxSecondsToLive, exists := os.LookupEnv("GF_PLUGIN_" + strings.ToUpper(strings.ReplaceAll(pcfg.PluginID, "-", "_")) + "_TOKEN_MAX_SECONDS_TO_LIVE")
	if !exists {
		maxSecondsToLive = "2592000" // 30 days
	}
	maxTTL, err := strconv.ParseInt(maxSecondsToLive, 10, 64)
	if err != nil {
		http.Error(w, fmt.Sprintf("error parsing max seconds to live: %v", err), http.StatusInternalServerError)
		return
	}

	// Use the requested TTL, or the max if not specified
	ttl := maxTTL
	if body.SecondsToLive != nil && *body.SecondsToLive < maxTTL {
		ttl = *body.SecondsToLive
	}

	// Get the requesting user's Service Account
	userServiceAccount, err := a.findOrCreateServiceAccount(req.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Clean up any expired tokens before doing anything else
	tokens, err := a.listTokens(req.Context(), userServiceAccount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.cleanUpExpiredTokens(req.Context(), userServiceAccount, tokens); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create and return the token with the validated TTL
	newToken, err := a.createToken(req.Context(), userServiceAccount, body.Name, ttl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(newToken); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *App) handleTokenList(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userServiceAccount, err := a.findOrCreateServiceAccount(req.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Clean up any expired tokens before doing anything else
	tokens, err := a.listTokens(req.Context(), userServiceAccount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.cleanUpExpiredTokens(req.Context(), userServiceAccount, tokens); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Now fetch the list of tokens again and return the results
	tokens, err = a.listTokens(req.Context(), userServiceAccount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(tokens); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *App) handleTokenDelete(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Make sure that the ID path parameter is a valid integer
	tokenID, err := strconv.Atoi(req.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid token ID", http.StatusBadRequest)
		return
	}

	// Get the requesting user's Service Account
	userServiceAccount, err := a.findOrCreateServiceAccount(req.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get the current list of tokens to make sure that the requested token ID is actually one of the user's tokens
	tokens, err := a.listTokens(req.Context(), userServiceAccount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	found := false
	for _, t := range tokens {
		if t.ID == int64(tokenID) {
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "token not found in list of user's tokens", http.StatusNotFound)
		return
	}

	// If we got a match, delete the token
	if err := a.deleteToken(req.Context(), userServiceAccount, int64(tokenID)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// And return the updated list of tokens after deletion
	tokens, err = a.listTokens(req.Context(), userServiceAccount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(tokens); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

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

func (a *App) createToken(ctx context.Context, sa *serviceAccount, name string, secondsToLive int64) (*token, error) {
	type body struct {
		Name          string `json:"name"`
		SecondsToLive int64  `json:"secondsToLive"`
	}
	createRequest := &apiRequest{
		Method: http.MethodPost,
		Path:   fmt.Sprintf("/api/serviceaccounts/%d/tokens", sa.ID),
		Body: body{
			Name:          name,
			SecondsToLive: secondsToLive,
		},
	}
	var newToken token
	if err := a.grafanaApiRequest(ctx, createRequest, &newToken); err != nil {
		return nil, fmt.Errorf("error creating token: %w", err)
	}
	return &newToken, nil
}

func (a *App) listTokens(ctx context.Context, sa *serviceAccount) ([]token, error) {
	listRequest := &apiRequest{
		Method: http.MethodGet,
		Path:   fmt.Sprintf("/api/serviceaccounts/%d/tokens", sa.ID),
	}
	var listResponse []token
	if err := a.grafanaApiRequest(ctx, listRequest, &listResponse); err != nil {
		return nil, fmt.Errorf("error listing tokens: %w", err)
	}
	return listResponse, nil
}

func (a *App) deleteToken(ctx context.Context, sa *serviceAccount, tokenID int64) error {
	deleteRequest := &apiRequest{
		Method: http.MethodDelete,
		Path:   fmt.Sprintf("/api/serviceaccounts/%d/tokens/%d", sa.ID, tokenID),
	}
	if err := a.grafanaApiRequest(ctx, deleteRequest, nil); err != nil {
		return fmt.Errorf("error deleting token: %w", err)
	}
	return nil
}

func (a *App) cleanUpExpiredTokens(ctx context.Context, sa *serviceAccount, tokens []token) error {

	pcfg := backend.PluginConfigFromContext(ctx)

	// Parse the desired cleanup grace period from an environment variable; defaults to 72 hours
	gracePeriod, exists := os.LookupEnv("GF_PLUGIN_" + strings.ToUpper(strings.ReplaceAll(pcfg.PluginID, "-", "_")) + "_TOKEN_CLEANUP_GRACE_PERIOD")
	if !exists {
		gracePeriod = "72h"
	}
	gracePeriodDuration, err := time.ParseDuration(gracePeriod)
	if err != nil {
		return fmt.Errorf("error parsing cleanup expired tokens duration: %w", err)
	}

	// Loop through each token and delete those that are expired for longer than the cleanup grace period
	for _, t := range tokens {
		if t.HasExpired {
			// If expiration is nil but token is marked as expired, just delete it
			if t.Expiration == nil {
				backend.Logger.Debug("deleting expired token with no expiration time", "orgId", sa.OrgID, "serviceAccount", sa.Name, "tokenId", t.ID)
				if err := a.deleteToken(ctx, sa, t.ID); err != nil {
					backend.Logger.Warn("error deleting expired token", "orgId", sa.OrgID, "serviceAccount", sa.Name, "tokenId", t.ID, "error", err)
				}
				continue
			}
			// Otherwise delete any that have been expired longer than the grace period
			expirationTime, err := time.Parse(time.RFC3339, *t.Expiration)
			if err != nil {
				backend.Logger.Warn("error parsing token expiration time, skipping token cleanup", "orgId", sa.OrgID, "serviceAccount", sa.Name, "tokenId", t.ID, "error", err)
				continue
			}
			if time.Since(expirationTime) > gracePeriodDuration {
				backend.Logger.Debug("deleting expired token", "orgId", sa.OrgID, "serviceAccount", sa.Name, "tokenId", t.ID)
				if err := a.deleteToken(ctx, sa, t.ID); err != nil {
					backend.Logger.Warn("error deleting expired token", "orgId", sa.OrgID, "serviceAccount", sa.Name, "tokenId", t.ID, "error", err)
				}
			}
		}
	}

	return nil
}

func (a *App) findOrCreateServiceAccount(ctx context.Context) (*serviceAccount, error) {

	pcfg := backend.PluginConfigFromContext(ctx)
	user := backend.UserFromContext(ctx)

	if user == nil || user.Login == "" {
		return nil, errors.New("this feature is unavailable for anonymous users; please sign in with a user account and try again")
	}

	serviceAccountName := fmt.Sprintf("user:%s", user.Login)

	// First try to search if the user's Service Account already exists

	searchRequest := &apiRequest{
		Method: http.MethodGet,
		Path:   "/api/serviceaccounts/search",
		Query: map[string]string{
			"perpage": "100000",
			"query":   serviceAccountName,
		},
	}

	var searchResponse struct {
		ServiceAccounts []serviceAccount `json:"serviceAccounts"`
		TotalCount      int              `json:"totalCount"`
	}

	if err := a.grafanaApiRequest(ctx, searchRequest, &searchResponse); err != nil {
		return nil, fmt.Errorf("error searching service accounts: %w", err)
	}

	saAttrs := map[string]any{
		"name":       serviceAccountName,
		"role":       user.Role,
		"orgId":      pcfg.OrgID,
		"isDisabled": false,
	}

	// Loop through all of the results try to find a match for the user (exact Login and OrgID of the requestor)
	for i, sa := range searchResponse.ServiceAccounts {
		if sa.Name == serviceAccountName && sa.OrgID == pcfg.OrgID {

			// If the Service Account still has the correct role and is enabled, just return it
			if sa.Role == user.Role && !sa.IsDisabled {
				return &searchResponse.ServiceAccounts[i], nil
			}

			// Otherwise, update it so it is correct first before returning it
			if err := a.setGrafanaUserOrgContext(ctx); err != nil {
				return nil, err
			}
			patchRequest := &apiRequest{
				Method: http.MethodPatch,
				Path:   fmt.Sprintf("/api/serviceaccounts/%d", sa.ID),
				Body:   saAttrs,
			}
			var sa serviceAccount
			if err := a.grafanaApiRequest(ctx, patchRequest, &sa); err != nil {
				return nil, fmt.Errorf("error patching service account: %w", err)
			}
			return &sa, nil

		}
	}

	// If there was no match, create a new one
	if err := a.setGrafanaUserOrgContext(ctx); err != nil {
		return nil, err
	}
	var sa serviceAccount
	createRequest := &apiRequest{
		Method: http.MethodPost,
		Path:   "/api/serviceaccounts",
		Body:   saAttrs,
	}
	if err := a.grafanaApiRequest(ctx, createRequest, &sa); err != nil {
		return nil, fmt.Errorf("error creating service account: %w", err)
	}
	return &sa, nil

}

type apiRequest struct {
	Method string
	Path   string
	Query  map[string]string
	Body   any
}

func (a *App) grafanaApiRequest(ctx context.Context, attrs *apiRequest, out any) error {

	gcfg := backend.GrafanaConfigFromContext(ctx)

	// Get the Grafana URL from the request context
	grafanaAppURL, err := gcfg.AppURL()
	if err != nil {
		return err
	}

	// Join the Grafana URL with the requested path
	reqURL, err := url.JoinPath(grafanaAppURL, attrs.Path)
	if err != nil {
		return err
	}

	// If a Body was passed, marshall it and create a reader for use with the request
	var bodyReader io.Reader
	if attrs.Body != nil {
		raw, err := json.Marshal(attrs.Body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(raw)
	}

	// Create the new HTTP request
	req, err := http.NewRequest(attrs.Method, reqURL, bodyReader)
	if err != nil {
		return err
	}

	// If any query parameters were passed, add them to the request URL
	if attrs.Query != nil {
		q := req.URL.Query()
		for k, v := range attrs.Query {
			q.Add(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}

	// Set request headers

	req.Header.Set("Accept", "application/json")

	// Set the Content-Type header if a body was passed
	if attrs.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Set the Authorization header
	if err := a.setGrafanaAuthHeader(ctx, req); err != nil {
		return err
	}

	// Send the request, get its body, and Unmarshal it into the provided out parameter

	backend.Logger.Debug("sending Grafana API request", "method", attrs.Method, "url", reqURL)
	resp, err := http.DefaultClient.Do(req)
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
		return fmt.Errorf("received %s when sending request to Grafana API: %s", resp.Status, string(raw))
	}

	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("parsing %s response: %w", attrs.Path, err)
	}
	return nil
}

func (a *App) setGrafanaUserOrgContext(ctx context.Context) error {

	pcfg := backend.PluginConfigFromContext(ctx)
	gcfg := backend.GrafanaConfigFromContext(ctx)

	// If externalServiceAccounts feature is enabled and the request is coming from OrgID 1, see if we are using the plugin's service account
	// Ideally, support for all organizations will be added and we can make this the default
	if pcfg.OrgID == 1 && gcfg.FeatureToggles().IsEnabled("externalServiceAccounts") {
		// Get the service account token
		_, err := gcfg.PluginAppClientSecret()
		if err == nil {
			backend.Logger.Debug("plugin service account token in use, skipping switching user organization context")
			return nil
		}
	}

	// Similarly, if we are using an Organization service account token, we do not need to switch the user organization context
	_, tokenExists := pcfg.AppInstanceSettings.DecryptedSecureJSONData["token"]
	if tokenExists {
		backend.Logger.Debug("organization service account token in use, skipping switching user organization context")
		return nil
	}

	// Otherwise, we need to switch the user organization context to ensure that the API calls we make are in the context of the requesting user's organization

	useOrgIdRequest := &apiRequest{
		Method: http.MethodPost,
		Path:   fmt.Sprintf("/api/user/using/%d", pcfg.OrgID),
	}

	if err := a.grafanaApiRequest(ctx, useOrgIdRequest, nil); err != nil {
		return fmt.Errorf("error switching user organization context: %w", err)
	}

	return nil

}

func (a *App) setGrafanaAuthHeader(ctx context.Context, req *http.Request) error {

	pcfg := backend.PluginConfigFromContext(ctx)
	gcfg := backend.GrafanaConfigFromContext(ctx)

	// If externalServiceAccounts feature is enabled and the request is coming from OrgID 1, use the plugin's service account
	// Ideally, support for all organizations will be added and we can make this the default and/or only authentication method
	if pcfg.OrgID == 1 && gcfg.FeatureToggles().IsEnabled("externalServiceAccounts") {
		// Get the service account token
		saToken, err := gcfg.PluginAppClientSecret()
		if err == nil {
			req.Header.Add("Authorization", "Bearer "+saToken)
			backend.Logger.Debug("using plugin service account token")
			return nil
		}
		backend.Logger.Warn("error trying to retrieve plugin service account token, please ensure you have enabled the externalServiceAccounts feature and enabled auth.managed_service_accounts_enabled", "error", err)
	}

	orgToken, tokenExists := pcfg.AppInstanceSettings.DecryptedSecureJSONData["token"]
	if tokenExists {
		req.Header.Add("Authorization", "Bearer "+string(orgToken))
		backend.Logger.Debug("using Organization service account token", "orgId", pcfg.OrgID)
		return nil
	}

	username, usernameExists := os.LookupEnv("GF_PLUGIN_" + strings.ToUpper(strings.ReplaceAll(pcfg.PluginID, "-", "_")) + "_BACKEND_USERNAME")
	password, passwordExists := os.LookupEnv("GF_PLUGIN_" + strings.ToUpper(strings.ReplaceAll(pcfg.PluginID, "-", "_")) + "_BACKEND_PASSWORD")
	if usernameExists && passwordExists {
		req.SetBasicAuth(username, password)
		backend.Logger.Debug("using basic auth", "username", username)
		return nil
	}

	return errors.New("no Grafana API credentials found")

}

// registerRoutes takes a *http.ServeMux and registers some HTTP handlers.
func (a *App) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/token", a.handleTokenCreate)
	mux.HandleFunc("/tokens", a.handleTokenList)
	mux.HandleFunc("/tokens/{id}", a.handleTokenDelete)
}
