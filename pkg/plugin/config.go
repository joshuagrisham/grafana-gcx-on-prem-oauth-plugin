package plugin

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// serviceAccountPrefix is prepended to a user's login when provisioning their
// per-user Service Account. All discovery and cleanup logic in the plugin
// filters on this prefix, so do not change it without considering the impact
// on existing installations.
const serviceAccountPrefix = "user:"

// Defaults for configurable behavior. All can be overridden by environment
// variables in the form GF_PLUGIN_<PLUGIN_ID>_<NAME>.
const (
	defaultTokenMaxSecondsToLive = "2592000" // 30 days
	defaultTokenCleanupGrace     = "72h"
	defaultMaxTokensPerUser      = "20"
	defaultCleanupInterval       = "1h"
	defaultRequestTimeout        = "30s"

	// Hard cap on the length of a user-supplied token name. Grafana itself
	// accepts very long names but unbounded user input could lead to undesired
	// behavior.
	maxTokenNameLength = 256
)

// pluginEnvName returns the canonical environment-variable name used to
// configure a setting for this plugin.
func (a *App) pluginEnvName(suffix string) string {
	return "GF_PLUGIN_" + strings.ToUpper(strings.ReplaceAll(a.pluginCtx.PluginID, "-", "_")) + "_" + suffix
}

// pluginEnv returns the configured value of suffix for pluginID, or def if
// the environment variable is unset.
func (a *App) pluginEnv(suffix, def string) string {
	if value, ok := os.LookupEnv(a.pluginEnvName(suffix)); ok {
		return value
	}
	return def
}

func (a *App) pluginEnvInt64(suffix string, def int64) int64 {
	value, err := strconv.ParseInt(a.pluginEnv(suffix, strconv.FormatInt(def, 10)), 10, 64)
	if err != nil {
		backend.Logger.Warn("Invalid env var value, using default", "name", a.pluginEnvName(suffix), "default", def, "error", err)
		return def
	}
	return value
}

func (a *App) pluginEnvDuration(suffix string, def time.Duration) time.Duration {
	value, err := time.ParseDuration(a.pluginEnv(suffix, def.String()))
	if err != nil {
		backend.Logger.Warn("Invalid env var value, using default", "name", a.pluginEnvName(suffix), "default", def, "error", err)
		return def
	}
	return value
}

// maxTokenTTL returns the maximum seconds-to-live a token created by this
// plugin is allowed to have.
func (a *App) maxTokenTTL() int64 {
	defaultValue, _ := strconv.ParseInt(defaultTokenMaxSecondsToLive, 10, 64)
	return a.pluginEnvInt64("TOKEN_MAX_SECONDS_TO_LIVE", defaultValue)
}

// tokenCleanupGracePeriod is how long an expired token is kept around before
// the background cleanup process deletes it.
func (a *App) tokenCleanupGracePeriod() time.Duration {
	defaultValue, _ := time.ParseDuration(defaultTokenCleanupGrace)
	return a.pluginEnvDuration("TOKEN_CLEANUP_GRACE_PERIOD", defaultValue)
}

// maxTokensPerUser is the maximum number of (live) tokens a single user is
// allowed to hold concurrently.
func (a *App) maxTokensPerUser() int64 {
	defaultValue, _ := strconv.ParseInt(defaultMaxTokensPerUser, 10, 64)
	return a.pluginEnvInt64("MAX_TOKENS_PER_USER", defaultValue)
}

// cleanupInterval is how often the background cleanup process runs.
func (a *App) cleanupInterval() time.Duration {
	defaultValue, _ := time.ParseDuration(defaultCleanupInterval)
	return a.pluginEnvDuration("CLEANUP_INTERVAL", defaultValue)
}

// requestTimeout is the per-request timeout applied to all outbound Grafana
// API calls made by this plugin.
func (a *App) requestTimeout() time.Duration {
	defaultValue, _ := time.ParseDuration(defaultRequestTimeout)
	return a.pluginEnvDuration("REQUEST_TIMEOUT", defaultValue)
}

// backendBasicAuth returns the configured basic-auth credentials, if any.
func (a *App) backendBasicAuth() (username, password string, ok bool) {
	username, uok := os.LookupEnv(a.pluginEnvName("BACKEND_USERNAME"))
	password, pok := os.LookupEnv(a.pluginEnvName("BACKEND_PASSWORD"))
	if !uok || !pok || username == "" || password == "" {
		return "", "", false
	}
	return username, password, true
}
