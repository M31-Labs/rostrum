package present

import "testing"

func TestReadOnlyDemoModeRequiresExplicitAppMode(t *testing.T) {
	t.Setenv("APP_MODE", "live")
	if ReadOnlyDemoMode() {
		t.Fatal("live APP_MODE reported as read-only demo")
	}
	t.Setenv("APP_MODE", "demo")
	if !ReadOnlyDemoMode() {
		t.Fatal("demo APP_MODE not reported as read-only demo")
	}
}
