package plugin

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"
)

// Make sure App implements required interfaces. This is important to do
// since otherwise we will only get a not implemented error response from plugin in
// runtime. Plugin should not implement all these interfaces - only those which are
// required for a particular task.
var (
	_ backend.CallResourceHandler   = (*App)(nil)
	_ instancemgmt.InstanceDisposer = (*App)(nil)
	_ backend.CheckHealthHandler    = (*App)(nil)
)

// App is the Grafana gcx On-Prem OAuth app plugin backend.
//
// One App instance is created per Grafana Organization. The backend exposes
// a small REST API that the plugin's frontend uses to mint and manage per-user
// service account tokens.
type App struct {
	backend.CallResourceHandler

	httpClient *http.Client

	// Save startup contexts for use with the background cleanup process.
	pluginCtx  backend.PluginContext
	grafanaCfg *backend.GrafanaCfg

	// orgSwitchMutex serializes calls to /api/user/using/{orgId} so that two
	// concurrent requests authenticating with shared Basic Auth credentials
	// cannot race and end up operating in the wrong org. Grafana does not
	// currently honor the X-Grafana-Org-Id header on the service account
	// endpoints when using Basic Auth, so a global mutex is the simplest
	// option to avoid this problem.
	// This mutex will only be used for non-default (orgId!=1) orgs, when the
	// plugin is configured to use Basic Auth credentials.
	orgSwitchMutex sync.Mutex

	// Background cleanup process lifecycle.
	bgCancel context.CancelFunc
	bgDone   chan struct{}
}

// NewApp creates a new App instance.
func NewApp(ctx context.Context, _ backend.AppInstanceSettings) (instancemgmt.Instance, error) {
	pCtx := backend.PluginConfigFromContext(ctx)
	gCfg := backend.GrafanaConfigFromContext(ctx)

	a := &App{
		pluginCtx:  pCtx,
		grafanaCfg: gCfg,
	}

	timeout := a.requestTimeout()
	client, err := httpclient.New(httpclient.Options{
		Timeouts: &httpclient.TimeoutOptions{
			Timeout:               timeout,
			DialTimeout:           httpclient.DefaultTimeoutOptions.DialTimeout,
			KeepAlive:             httpclient.DefaultTimeoutOptions.KeepAlive,
			TLSHandshakeTimeout:   httpclient.DefaultTimeoutOptions.TLSHandshakeTimeout,
			ExpectContinueTimeout: httpclient.DefaultTimeoutOptions.ExpectContinueTimeout,
			MaxConnsPerHost:       httpclient.DefaultTimeoutOptions.MaxConnsPerHost,
			MaxIdleConns:          httpclient.DefaultTimeoutOptions.MaxIdleConns,
			MaxIdleConnsPerHost:   httpclient.DefaultTimeoutOptions.MaxIdleConnsPerHost,
			IdleConnTimeout:       httpclient.DefaultTimeoutOptions.IdleConnTimeout,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error creating http client: %w", err)
	}

	a.httpClient = client

	mux := http.NewServeMux()
	a.registerRoutes(mux)
	a.CallResourceHandler = httpadapter.New(mux)

	a.startCleanupProcess()

	backend.Logger.Info("Plugin instance started",
		"pluginId", pCtx.PluginID,
		"orgId", pCtx.OrgID,
		"maxTokensPerUser", fmt.Sprintf("%d", a.maxTokensPerUser()),
		"tokenMaxTTLSeconds", fmt.Sprintf("%d", a.maxTokenTTL()),
		"cleanupInterval", a.cleanupInterval().String(),
		"tokenCleanupGracePeriod", a.tokenCleanupGracePeriod().String(),
	)
	return a, nil
}

// Dispose stops the background cleanup process. It is called by the SDK when a
// new instance is created (e.g. settings have changed) or the plugin is
// shutting down.
func (a *App) Dispose() {
	if a.bgCancel != nil {
		a.bgCancel()
		select {
		case <-a.bgDone:
		case <-time.After(10 * time.Second):
			backend.Logger.Warn("Background cleanup process did not stop within timeout")
		}
	}
}

// CheckHealth verifies that the plugin can authenticate against Grafana's
// API and that any org-specific configuration looks plausible.
func (a *App) CheckHealth(ctx context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	pCtx := backend.PluginConfigFromContext(ctx)
	if pCtx.PluginID == "" {
		pCtx = a.pluginCtx
	}
	gCfg := backend.GrafanaConfigFromContext(ctx)
	if gCfg == nil {
		gCfg = a.grafanaCfg
	}

	// Hit a small, cheap endpoint that requires the exact permission we
	// need (read access to service accounts in the current org).
	probe := &apiRequest{
		Method: http.MethodGet,
		Path:   "/api/serviceaccounts/search",
		Query:  map[string]string{"perpage": "1"},
	}
	if err := a.grafanaAPIRequest(ctx, gCfg, pCtx, probe, nil); err != nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: fmt.Sprintf("Grafana API auth check failed: %v", err),
		}, nil
	}

	var warnings []string

	// The managed plugin service account is only supported in OrgID 1
	// (see https://grafana.com/developers/plugin-tools/how-to-guides/app-plugins/use-a-service account).
	// For any other org the operator must either provision an Org-scoped
	// service account token in the plugin's settings or wire up Basic Auth.
	if pCtx.OrgID != 1 {
		hasOrgToken := false
		if pCtx.AppInstanceSettings != nil {
			if v, ok := pCtx.AppInstanceSettings.DecryptedSecureJSONData["token"]; ok && v != "" {
				hasOrgToken = true
			}
		}
		_, _, hasBasic := a.backendBasicAuth()
		if !hasOrgToken && !hasBasic {
			warnings = append(warnings, fmt.Sprintf(
				"orgId %d has no Organization service account token configured and no Basic Auth credentials are set; the managed plugin service account only works in orgId 1",
				pCtx.OrgID))
		}
	}

	if len(warnings) > 0 {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusOk,
			Message: "ok with warnings: " + strings.Join(warnings, "; "),
		}, nil
	}
	return &backend.CheckHealthResult{Status: backend.HealthStatusOk, Message: "ok"}, nil
}
