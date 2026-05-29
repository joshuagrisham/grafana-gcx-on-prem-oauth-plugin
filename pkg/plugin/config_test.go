package plugin

import (
	"strings"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/config"
)

// newConfigTestApp creates a minimal App suitable for config/env tests.
func newConfigTestApp(t *testing.T) *App {
	t.Helper()
	return &App{
		pluginCtx: backend.PluginContext{
			PluginID: testPluginID,
			OrgID:    1,
		},
		grafanaCfg: config.NewGrafanaCfg(map[string]string{
			"GF_APP_URL": "http://localhost:3000",
		}),
	}
}

func setEnv(t *testing.T, suffix, value string) {
	t.Helper()
	name := "GF_PLUGIN_" + strings.ToUpper(strings.ReplaceAll(testPluginID, "-", "_")) + "_" + suffix
	t.Setenv(name, value)
}

// ---------------------------------------------------------------------------
// TOKEN_MAX_SECONDS_TO_LIVE (int64, default 2592000)
// ---------------------------------------------------------------------------

func TestMaxTokenTTL_Default(t *testing.T) {
	app := newConfigTestApp(t)
	if got := app.maxTokenTTL(); got != 2592000 {
		t.Errorf("maxTokenTTL() = %d, want 2592000", got)
	}
}

func TestMaxTokenTTL_ValidOverride(t *testing.T) {
	setEnv(t, "TOKEN_MAX_SECONDS_TO_LIVE", "3600")
	app := newConfigTestApp(t)
	if got := app.maxTokenTTL(); got != 3600 {
		t.Errorf("maxTokenTTL() = %d, want 3600", got)
	}
}

func TestMaxTokenTTL_InvalidNonNumeric(t *testing.T) {
	setEnv(t, "TOKEN_MAX_SECONDS_TO_LIVE", "not-a-number")
	app := newConfigTestApp(t)
	if got := app.maxTokenTTL(); got != 2592000 {
		t.Errorf("maxTokenTTL() = %d, want default 2592000 for invalid input", got)
	}
}

func TestMaxTokenTTL_InvalidFloat(t *testing.T) {
	setEnv(t, "TOKEN_MAX_SECONDS_TO_LIVE", "3.14")
	app := newConfigTestApp(t)
	if got := app.maxTokenTTL(); got != 2592000 {
		t.Errorf("maxTokenTTL() = %d, want default 2592000 for float input", got)
	}
}

func TestMaxTokenTTL_Empty(t *testing.T) {
	setEnv(t, "TOKEN_MAX_SECONDS_TO_LIVE", "")
	app := newConfigTestApp(t)
	if got := app.maxTokenTTL(); got != 2592000 {
		t.Errorf("maxTokenTTL() = %d, want default 2592000 for empty input", got)
	}
}

// ---------------------------------------------------------------------------
// MAX_TOKENS_PER_USER (int64, default 20)
// ---------------------------------------------------------------------------

func TestMaxTokensPerUser_Default(t *testing.T) {
	app := newConfigTestApp(t)
	if got := app.maxTokensPerUser(); got != 20 {
		t.Errorf("maxTokensPerUser() = %d, want 20", got)
	}
}

func TestMaxTokensPerUser_ValidOverride(t *testing.T) {
	setEnv(t, "MAX_TOKENS_PER_USER", "5")
	app := newConfigTestApp(t)
	if got := app.maxTokensPerUser(); got != 5 {
		t.Errorf("maxTokensPerUser() = %d, want 5", got)
	}
}

func TestMaxTokensPerUser_InvalidNonNumeric(t *testing.T) {
	setEnv(t, "MAX_TOKENS_PER_USER", "abc")
	app := newConfigTestApp(t)
	if got := app.maxTokensPerUser(); got != 20 {
		t.Errorf("maxTokensPerUser() = %d, want default 20 for invalid input", got)
	}
}

func TestMaxTokensPerUser_InvalidFloat(t *testing.T) {
	setEnv(t, "MAX_TOKENS_PER_USER", "2.5")
	app := newConfigTestApp(t)
	if got := app.maxTokensPerUser(); got != 20 {
		t.Errorf("maxTokensPerUser() = %d, want default 20 for float input", got)
	}
}

func TestMaxTokensPerUser_Negative(t *testing.T) {
	setEnv(t, "MAX_TOKENS_PER_USER", "-1")
	app := newConfigTestApp(t)
	// Negative values parse fine as int64; the plugin doesn't reject them at parse time.
	if got := app.maxTokensPerUser(); got != -1 {
		t.Errorf("maxTokensPerUser() = %d, want -1 (negative values are not rejected at parse)", got)
	}
}

// ---------------------------------------------------------------------------
// TOKEN_CLEANUP_GRACE_PERIOD (duration, default 72h)
// ---------------------------------------------------------------------------

func TestTokenCleanupGracePeriod_Default(t *testing.T) {
	app := newConfigTestApp(t)
	if got := app.tokenCleanupGracePeriod(); got != 72*time.Hour {
		t.Errorf("tokenCleanupGracePeriod() = %v, want 72h", got)
	}
}

func TestTokenCleanupGracePeriod_ValidOverride(t *testing.T) {
	setEnv(t, "TOKEN_CLEANUP_GRACE_PERIOD", "24h")
	app := newConfigTestApp(t)
	if got := app.tokenCleanupGracePeriod(); got != 24*time.Hour {
		t.Errorf("tokenCleanupGracePeriod() = %v, want 24h", got)
	}
}

func TestTokenCleanupGracePeriod_ValidComplex(t *testing.T) {
	setEnv(t, "TOKEN_CLEANUP_GRACE_PERIOD", "1h30m")
	app := newConfigTestApp(t)
	if got := app.tokenCleanupGracePeriod(); got != 90*time.Minute {
		t.Errorf("tokenCleanupGracePeriod() = %v, want 1h30m", got)
	}
}

func TestTokenCleanupGracePeriod_InvalidFormat(t *testing.T) {
	setEnv(t, "TOKEN_CLEANUP_GRACE_PERIOD", "three-hours")
	app := newConfigTestApp(t)
	if got := app.tokenCleanupGracePeriod(); got != 72*time.Hour {
		t.Errorf("tokenCleanupGracePeriod() = %v, want default 72h for invalid input", got)
	}
}

func TestTokenCleanupGracePeriod_InvalidBareNumber(t *testing.T) {
	// A bare number without a unit suffix is invalid for time.ParseDuration.
	setEnv(t, "TOKEN_CLEANUP_GRACE_PERIOD", "3600")
	app := newConfigTestApp(t)
	if got := app.tokenCleanupGracePeriod(); got != 72*time.Hour {
		t.Errorf("tokenCleanupGracePeriod() = %v, want default 72h for bare number", got)
	}
}

func TestTokenCleanupGracePeriod_Empty(t *testing.T) {
	setEnv(t, "TOKEN_CLEANUP_GRACE_PERIOD", "")
	app := newConfigTestApp(t)
	if got := app.tokenCleanupGracePeriod(); got != 72*time.Hour {
		t.Errorf("tokenCleanupGracePeriod() = %v, want default 72h for empty input", got)
	}
}

// ---------------------------------------------------------------------------
// CLEANUP_INTERVAL (duration, default 1h)
// ---------------------------------------------------------------------------

func TestCleanupInterval_Default(t *testing.T) {
	app := newConfigTestApp(t)
	if got := app.cleanupInterval(); got != 1*time.Hour {
		t.Errorf("cleanupInterval() = %v, want 1h", got)
	}
}

func TestCleanupInterval_ValidOverride(t *testing.T) {
	setEnv(t, "CLEANUP_INTERVAL", "15m")
	app := newConfigTestApp(t)
	if got := app.cleanupInterval(); got != 15*time.Minute {
		t.Errorf("cleanupInterval() = %v, want 15m", got)
	}
}

func TestCleanupInterval_InvalidFormat(t *testing.T) {
	setEnv(t, "CLEANUP_INTERVAL", "every-hour")
	app := newConfigTestApp(t)
	if got := app.cleanupInterval(); got != 1*time.Hour {
		t.Errorf("cleanupInterval() = %v, want default 1h for invalid input", got)
	}
}

func TestCleanupInterval_InvalidBareNumber(t *testing.T) {
	setEnv(t, "CLEANUP_INTERVAL", "60")
	app := newConfigTestApp(t)
	if got := app.cleanupInterval(); got != 1*time.Hour {
		t.Errorf("cleanupInterval() = %v, want default 1h for bare number", got)
	}
}

// ---------------------------------------------------------------------------
// REQUEST_TIMEOUT (duration, default 30s)
// ---------------------------------------------------------------------------

func TestRequestTimeout_Default(t *testing.T) {
	app := newConfigTestApp(t)
	if got := app.requestTimeout(); got != 30*time.Second {
		t.Errorf("requestTimeout() = %v, want 30s", got)
	}
}

func TestRequestTimeout_ValidOverride(t *testing.T) {
	setEnv(t, "REQUEST_TIMEOUT", "10s")
	app := newConfigTestApp(t)
	if got := app.requestTimeout(); got != 10*time.Second {
		t.Errorf("requestTimeout() = %v, want 10s", got)
	}
}

func TestRequestTimeout_InvalidFormat(t *testing.T) {
	setEnv(t, "REQUEST_TIMEOUT", "ten-seconds")
	app := newConfigTestApp(t)
	if got := app.requestTimeout(); got != 30*time.Second {
		t.Errorf("requestTimeout() = %v, want default 30s for invalid input", got)
	}
}

func TestRequestTimeout_InvalidBareNumber(t *testing.T) {
	setEnv(t, "REQUEST_TIMEOUT", "30")
	app := newConfigTestApp(t)
	if got := app.requestTimeout(); got != 30*time.Second {
		t.Errorf("requestTimeout() = %v, want default 30s for bare number", got)
	}
}

// ---------------------------------------------------------------------------
// BACKEND_INSECURE_TLS (bool, default false)
// ---------------------------------------------------------------------------

func TestBackendInsecureTLS_Default(t *testing.T) {
	app := newConfigTestApp(t)
	if got := app.backendInsecureTLS(); got != false {
		t.Errorf("backendInsecureTLS() = %v, want false", got)
	}
}

func TestBackendInsecureTLS_True(t *testing.T) {
	setEnv(t, "BACKEND_INSECURE_TLS", "true")
	app := newConfigTestApp(t)
	if got := app.backendInsecureTLS(); got != true {
		t.Errorf("backendInsecureTLS() = %v, want true", got)
	}
}

func TestBackendInsecureTLS_One(t *testing.T) {
	// strconv.ParseBool accepts "1" as true.
	setEnv(t, "BACKEND_INSECURE_TLS", "1")
	app := newConfigTestApp(t)
	if got := app.backendInsecureTLS(); got != true {
		t.Errorf("backendInsecureTLS() = %v, want true for '1'", got)
	}
}

func TestBackendInsecureTLS_False(t *testing.T) {
	setEnv(t, "BACKEND_INSECURE_TLS", "false")
	app := newConfigTestApp(t)
	if got := app.backendInsecureTLS(); got != false {
		t.Errorf("backendInsecureTLS() = %v, want false", got)
	}
}

func TestBackendInsecureTLS_Zero(t *testing.T) {
	setEnv(t, "BACKEND_INSECURE_TLS", "0")
	app := newConfigTestApp(t)
	if got := app.backendInsecureTLS(); got != false {
		t.Errorf("backendInsecureTLS() = %v, want false for '0'", got)
	}
}

func TestBackendInsecureTLS_InvalidGarbage(t *testing.T) {
	setEnv(t, "BACKEND_INSECURE_TLS", "yes-please")
	app := newConfigTestApp(t)
	if got := app.backendInsecureTLS(); got != false {
		t.Errorf("backendInsecureTLS() = %v, want default false for invalid input", got)
	}
}

func TestBackendInsecureTLS_Empty(t *testing.T) {
	setEnv(t, "BACKEND_INSECURE_TLS", "")
	app := newConfigTestApp(t)
	if got := app.backendInsecureTLS(); got != false {
		t.Errorf("backendInsecureTLS() = %v, want default false for empty input", got)
	}
}

// ---------------------------------------------------------------------------
// BACKEND_URL (string, defaults to GF_APP_URL from GrafanaCfg)
// ---------------------------------------------------------------------------

func TestBackendUrl_Default(t *testing.T) {
	app := newConfigTestApp(t)
	if got := app.backendUrl(); got != "http://localhost:3000" {
		t.Errorf("backendUrl() = %q, want %q", got, "http://localhost:3000")
	}
}

func TestBackendUrl_Override(t *testing.T) {
	setEnv(t, "BACKEND_URL", "https://grafana.internal:8443")
	app := newConfigTestApp(t)
	if got := app.backendUrl(); got != "https://grafana.internal:8443" {
		t.Errorf("backendUrl() = %q, want %q", got, "https://grafana.internal:8443")
	}
}

func TestBackendUrl_EmptyOverride(t *testing.T) {
	// Setting to empty string means the env var IS set but empty.
	// pluginEnv returns the env value (empty) since LookupEnv finds it.
	setEnv(t, "BACKEND_URL", "")
	app := newConfigTestApp(t)
	if got := app.backendUrl(); got != "" {
		t.Errorf("backendUrl() = %q, want empty string when env var is set but empty", got)
	}
}

// ---------------------------------------------------------------------------
// BACKEND_USERNAME / BACKEND_PASSWORD (string pair)
// ---------------------------------------------------------------------------

func TestBackendBasicAuth_NotSet(t *testing.T) {
	app := newConfigTestApp(t)
	_, _, ok := app.backendBasicAuth()
	if ok {
		t.Error("backendBasicAuth() ok = true, want false when env vars not set")
	}
}

func TestBackendBasicAuth_BothSet(t *testing.T) {
	setEnv(t, "BACKEND_USERNAME", "admin")
	setEnv(t, "BACKEND_PASSWORD", "secret")
	app := newConfigTestApp(t)
	user, pass, ok := app.backendBasicAuth()
	if !ok {
		t.Fatal("backendBasicAuth() ok = false, want true")
	}
	if user != "admin" {
		t.Errorf("username = %q, want %q", user, "admin")
	}
	if pass != "secret" {
		t.Errorf("password = %q, want %q", pass, "secret")
	}
}

func TestBackendBasicAuth_OnlyUsername(t *testing.T) {
	setEnv(t, "BACKEND_USERNAME", "admin")
	// PASSWORD not set
	app := newConfigTestApp(t)
	_, _, ok := app.backendBasicAuth()
	if ok {
		t.Error("backendBasicAuth() ok = true, want false when only username is set")
	}
}

func TestBackendBasicAuth_OnlyPassword(t *testing.T) {
	setEnv(t, "BACKEND_PASSWORD", "secret")
	// USERNAME not set
	app := newConfigTestApp(t)
	_, _, ok := app.backendBasicAuth()
	if ok {
		t.Error("backendBasicAuth() ok = true, want false when only password is set")
	}
}

func TestBackendBasicAuth_EmptyUsername(t *testing.T) {
	setEnv(t, "BACKEND_USERNAME", "")
	setEnv(t, "BACKEND_PASSWORD", "secret")
	app := newConfigTestApp(t)
	_, _, ok := app.backendBasicAuth()
	if ok {
		t.Error("backendBasicAuth() ok = true, want false when username is empty")
	}
}

func TestBackendBasicAuth_EmptyPassword(t *testing.T) {
	setEnv(t, "BACKEND_USERNAME", "admin")
	setEnv(t, "BACKEND_PASSWORD", "")
	app := newConfigTestApp(t)
	_, _, ok := app.backendBasicAuth()
	if ok {
		t.Error("backendBasicAuth() ok = true, want false when password is empty")
	}
}
