package present

import (
	"os"
	"strings"
)

// DemoMode reports whether the workspace is running against the seeded demo
// dataset, mirroring main.go's own SEED lookup ("demo" is the default
// value main.go's selectSeed falls back to). The organizer layout and the
// public home page use this flag to hide demo-only chrome -- the "Demo
// mode" badge, the workspace-reset banner, and the home page's "demo
// workspace" framing -- once a deployment seeds real data with
// SEED=fresh or SEED=empty.
func DemoMode() bool {
	seed := strings.TrimSpace(os.Getenv("SEED"))
	if seed == "" {
		seed = "demo"
	}
	return strings.EqualFold(seed, "demo")
}

// ReadOnlyDemoMode reports the explicit hosted-demo posture. It is separate
// from DemoMode because a local seeded workspace remains intentionally
// interactive, while APP_MODE=demo is a deployment boundary that disables
// identity, mutations, imports, exports, and resets.
func ReadOnlyDemoMode() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("APP_MODE")), "demo")
}
