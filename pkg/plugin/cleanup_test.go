package plugin

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/config"
)

func newCleanupApp(t *testing.T, fake *fakeGrafana) *App {
	t.Helper()
	app := &App{
		httpClient: fake.server.Client(),
		pluginCtx:  backend.PluginContext{PluginID: testPluginID, OrgID: 1},
		grafanaCfg: config.NewGrafanaCfg(map[string]string{"GF_APP_URL": fake.URL()}),
	}
	mux := http.NewServeMux()
	app.registerRoutes(mux)
	return app
}

func TestCleanup_DeletesSAsForMissingUsers(t *testing.T) {
	fake := newFakeGrafana(t)
	setBasicAuthEnv(t, "admin", "admin")
	// Two SAs but only one corresponding active user. The other SA
	// should be removed by the cleanup cycle.
	fake.addServiceAccount(serviceAccountName("alice"), "Viewer", 1, false)
	fake.addServiceAccount(serviceAccountName("bob"), "Viewer", 1, false)
	fake.addOrgUser(1, "alice", "Viewer", false)

	app := newCleanupApp(t, fake)
	if err := app.cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	if sa := findFakeSAByName(fake, serviceAccountName("bob")); sa != nil {
		t.Errorf("expected bob's service account to be removed, still exists: %+v", sa)
	}
	if sa := findFakeSAByName(fake, serviceAccountName("alice")); sa == nil {
		t.Errorf("expected alice's service account to remain")
	}
}

func TestCleanup_DeletesSAsForDisabledUsers(t *testing.T) {
	fake := newFakeGrafana(t)
	setBasicAuthEnv(t, "admin", "admin")
	fake.addServiceAccount(serviceAccountName("alice"), "Viewer", 1, false)
	fake.addOrgUser(1, "alice", "Viewer", true) // disabled

	app := newCleanupApp(t, fake)
	if err := app.cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	if sa := findFakeSAByName(fake, serviceAccountName("alice")); sa != nil {
		t.Errorf("expected disabled alice's service account to be removed")
	}
}

func TestCleanup_DeletesAllExpiredTokensPastGrace(t *testing.T) {
	t.Setenv(pluginEnv("TOKEN_CLEANUP_GRACE_PERIOD"), "1ms")
	fake := newFakeGrafana(t)
	setBasicAuthEnv(t, "admin", "admin")
	sa := fake.addServiceAccount(serviceAccountName("alice"), "Viewer", 1, false)
	fake.addOrgUser(1, "alice", "Viewer", false)

	expired := fake.addToken(sa.ID, "old", -time.Hour)
	live := fake.addToken(sa.ID, "live", time.Hour)

	app := newCleanupApp(t, fake)
	if err := app.cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	fake.mu.Lock()
	_, expiredStill := fake.tokens[expired.ID]
	_, liveStill := fake.tokens[live.ID]
	fake.mu.Unlock()
	if expiredStill {
		t.Errorf("expected expired token to be deleted")
	}
	if !liveStill {
		t.Errorf("expected live token to be retained")
	}
}

func TestCleanup_ReconcilesRoleChange(t *testing.T) {
	fake := newFakeGrafana(t)
	setBasicAuthEnv(t, "admin", "admin")
	fake.addServiceAccount(serviceAccountName("alice"), "Viewer", 1, false)
	fake.addOrgUser(1, "alice", "Admin", false)

	app := newCleanupApp(t, fake)
	if err := app.cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if sa := findFakeSAByName(fake, serviceAccountName("alice")); sa == nil || sa.Role != "Admin" {
		t.Errorf("expected role reconciled to Admin, got %+v", sa)
	}
}

func TestCleanup_ContinuesAfterDeletingFirstSA(t *testing.T) {
	// Regression test for the previous bug where the cleanup loop used
	// `break` instead of `continue` after deleting an SA, causing
	// subsequent SAs to be skipped entirely.
	fake := newFakeGrafana(t)
	setBasicAuthEnv(t, "admin", "admin")
	// Two SAs with no corresponding users -- both should be deleted.
	fake.addServiceAccount(serviceAccountName("alice"), "Viewer", 1, false)
	fake.addServiceAccount(serviceAccountName("bob"), "Viewer", 1, false)
	fake.addServiceAccount(serviceAccountName("carol"), "Viewer", 1, false)

	app := newCleanupApp(t, fake)
	if err := app.cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	fake.mu.Lock()
	remaining := len(fake.serviceAccounts)
	fake.mu.Unlock()
	if remaining != 0 {
		t.Errorf("expected all 3 SAs deleted, %d remain", remaining)
	}
}
