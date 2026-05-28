package plugin

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// startCleanupProcess launches the background cleanup goroutine for this
// app instance.
func (a *App) startCleanupProcess() {
	interval := a.cleanupInterval()
	if interval <= 0 {
		backend.Logger.Info("Background cleanup process disabled (interval <= 0)",
			"orgId", a.pluginCtx.OrgID)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Propagate plugin context and grafana cfg so the API client can
	// authenticate.
	ctx = backend.WithPluginContext(ctx, a.pluginCtx)
	ctx = backend.WithGrafanaConfig(ctx, a.grafanaCfg)

	a.bgCancel = cancel
	a.bgDone = make(chan struct{})

	go func() {
		defer close(a.bgDone)

		// Run shortly after startup so misconfigurations show up
		// quickly in logs, then on a steady interval.
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}

			if err := a.cleanup(ctx); err != nil {
				backend.Logger.Warn("Cleanup cycle failed",
					"orgId", a.pluginCtx.OrgID, "error", err)
			}

			timer.Reset(interval)
		}
	}()
}

// cleanup runs a single cleanup cycle. It is intentionally idempotent and
// safe to retry: any partial failure is logged and the next cycle picks up
// where this one left off.
func (a *App) cleanup(ctx context.Context) error {
	pCtx := a.pluginCtx
	gCfg := a.grafanaCfg

	backend.Logger.Info("Cleanup cycle starting",
		"orgId", pCtx.OrgID)

	sas, err := a.listAllPluginServiceAccounts(ctx, gCfg, pCtx)
	if err != nil {
		return fmt.Errorf("listing service accounts: %w", err)
	}

	users, err := a.listAllOrgUsers(ctx, gCfg, pCtx)
	if err != nil {
		return fmt.Errorf("listing users: %w", err)
	}

	for _, sa := range sas {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Find the user matching this SA (if they still exist) and ensure the SA's
		// role still matches the user.
		isActiveUser := false
		for _, u := range users {
			if u.Login == strings.TrimPrefix(sa.Name, serviceAccountPrefix) {
				// Re-sync the role if the user's role in this org has changed.
				// Tokens inherit the SA's permissions per-request, so this
				// takes effect immediately without needing to revoke tokens.
				if u.Role != "" && u.Role != sa.Role {
					backend.Logger.Info("Reconciling service account role",
						"orgId", pCtx.OrgID, "userLogin", u.Login,
						"serviceAccountId", sa.ID, "serviceAccountName", sa.Name,
						"oldRole", sa.Role, "newRole", u.Role)
					patch := &apiRequest{
						Method: http.MethodPatch,
						Path:   fmt.Sprintf("/api/serviceaccounts/%d", sa.ID),
						Body: map[string]any{
							"name":       sa.Name,
							"role":       u.Role,
							"isDisabled": false,
						},
					}
					if err := a.grafanaAPIRequest(ctx, gCfg, pCtx, patch, nil); err != nil {
						backend.Logger.Warn("Failed to patch service account role",
							"orgId", pCtx.OrgID, "serviceAccountId", sa.ID,
							"serviceAccountName", sa.Name, "error", err)
					}
				}
				// As we have found the match, set isActiveUser and exit the loop.
				isActiveUser = !u.IsDisabled
				break
			}
		}

		// Delete the SA if the corresponding user no longer exists or has been
		// disabled.
		if !isActiveUser {
			backend.Logger.Info("User disabled or no longer exists; deleting service account",
				"orgId", pCtx.OrgID, "serviceAccountId", sa.ID,
				"serviceAccountName", sa.Name)
			if err := a.deleteServiceAccount(ctx, gCfg, pCtx, sa.ID); err != nil {
				backend.Logger.Warn("Failed to delete service account",
					"orgId", pCtx.OrgID, "serviceAccountId", sa.ID,
					"serviceAccountName", sa.Name, "error", err)
			}
			break
		}

		// If the user is still active, clean up any expired tokens.
		backend.Logger.Debug("Cleaning up expired tokens for active user service account",
			"orgId", pCtx.OrgID, "serviceAccountId", sa.ID, "serviceAccountName", sa.Name)
		a.cleanupExpiredTokens(ctx, gCfg, pCtx, &sa)
	}

	backend.Logger.Info("Cleanup cycle complete",
		"orgId", pCtx.OrgID, "serviceAccountsProcessed", len(sas))
	return nil
}

// listAllPluginServiceAccounts returns every SA in this org whose name
// starts with the plugin's prefix.
func (a *App) listAllPluginServiceAccounts(ctx context.Context, gCfg *backend.GrafanaCfg, pCtx backend.PluginContext) ([]serviceAccount, error) {
	const perPage = 1000
	var all []serviceAccount
	for page := 1; page <= 1000; page++ {
		req := &apiRequest{
			Method: http.MethodGet,
			Path:   "/api/serviceaccounts/search",
			Query: map[string]string{
				"perpage": strconv.Itoa(perPage),
				"page":    strconv.Itoa(page),
				"query":   serviceAccountPrefix,
			},
		}
		var resp struct {
			ServiceAccounts []serviceAccount `json:"serviceAccounts"`
		}
		if err := a.grafanaAPIRequestWithOrgSwitch(ctx, gCfg, pCtx, req, &resp); err != nil {
			return nil, err
		}
		for _, sa := range resp.ServiceAccounts {
			if strings.HasPrefix(sa.Name, serviceAccountPrefix) && sa.OrgID == pCtx.OrgID {
				all = append(all, sa)
			}
		}
		if len(resp.ServiceAccounts) < perPage {
			break
		}
	}
	return all, nil
}

type grafanaOrgUser struct {
	OrgID      int64  `json:"orgId"`
	UserID     int64  `json:"userId"`
	Login      string `json:"login"`
	Email      string `json:"email"`
	Role       string `json:"role"`
	IsDisabled bool   `json:"isDisabled"`
}

// listAllOrgUsers returns all users in the current org.
func (a *App) listAllOrgUsers(ctx context.Context, gCfg *backend.GrafanaCfg, pCtx backend.PluginContext) ([]grafanaOrgUser, error) {
	req := &apiRequest{
		Method: http.MethodGet,
		Path:   "/api/org/users",
	}
	var users []grafanaOrgUser
	if err := a.grafanaAPIRequestWithOrgSwitch(ctx, gCfg, pCtx, req, &users); err != nil {
		return nil, err
	}
	return users, nil
}

// cleanupExpiredTokens gets all tokens for the given SA and deletes any that
// have expired beyond the grace period.
func (a *App) cleanupExpiredTokens(ctx context.Context, gCfg *backend.GrafanaCfg, pCtx backend.PluginContext, sa *serviceAccount) {
	tokens, err := a.listTokens(ctx, gCfg, pCtx, sa)
	if err != nil {
		backend.Logger.Warn("Failed to list tokens for cleanup",
			"orgId", pCtx.OrgID, "serviceAccountId", sa.ID,
			"serviceAccountName", sa.Name, "error", err)
		return
	}

	for _, t := range tokens {
		if t.Expiration == nil {
			backend.Logger.Warn("Token has no expiration timestamp which indicates that it was not created by the plugin; deleting",
				"orgId", pCtx.OrgID, "serviceAccountId", sa.ID, "serviceAccountName", sa.Name,
				"tokenId", t.ID, "tokenName", t.Name)
		} else if !t.HasExpired {
			backend.Logger.Debug("Token is still valid; skipping",
				"orgId", pCtx.OrgID, "serviceAccountId", sa.ID, "serviceAccountName", sa.Name,
				"tokenId", t.ID, "tokenName", t.Name,
				"expiration", t.Expiration)
			continue
		} else {
			exp, err := time.Parse(time.RFC3339, *t.Expiration)
			if err != nil {
				backend.Logger.Warn("Token is expired but has an invalid expiration timestamp; deleting",
					"orgId", pCtx.OrgID, "serviceAccountId", sa.ID, "serviceAccountName", sa.Name,
					"tokenId", t.ID, "tokenName", t.Name,
					"error", err)
			}
			if time.Since(exp) < a.tokenCleanupGracePeriod() {
				backend.Logger.Debug("Token is expired but still within the grace period; skipping",
					"orgId", pCtx.OrgID, "serviceAccountId", sa.ID, "serviceAccountName", sa.Name,
					"tokenId", t.ID, "tokenName", t.Name,
					"expiration", t.Expiration, "timeSinceExpiration", time.Since(exp).String())
				continue
			}
		}
		backend.Logger.Info("Cleaning up expired or invalid token",
			"orgId", pCtx.OrgID, "serviceAccountId", sa.ID, "serviceAccountName", sa.Name,
			"tokenId", t.ID, "tokenName", t.Name, "expiration", t.Expiration)
		if err := a.deleteToken(ctx, gCfg, pCtx, sa, t.ID); err != nil {
			backend.Logger.Warn("Failed to delete expired token",
				"orgId", pCtx.OrgID, "serviceAccountId", sa.ID, "serviceAccountName", sa.Name,
				"tokenId", t.ID, "tokenName", t.Name,
				"error", err)
		}
	}
}
