package brand

import (
	"runtime"
	"testing"
)

func TestVersionInfoUsesDevelopmentDefaults(t *testing.T) {
	info := Info()
	if info.Version != "dev" || info.Revision != "unknown" || info.BuildDate != "unknown" {
		t.Fatalf("unexpected development metadata: %#v", info)
	}
	if info.GoVersion != runtime.Version() {
		t.Fatalf("unexpected Go version: got %q want %q", info.GoVersion, runtime.Version())
	}
}
