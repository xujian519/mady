package theme

import (
	"context"
	"testing"
	"time"
)

func TestSystemAppearanceString(t *testing.T) {
	cases := []struct {
		a    SystemAppearance
		want string
	}{
		{AppearanceUnknown, "unknown"},
		{AppearanceDark, "dark"},
		{AppearanceLight, "light"},
		{SystemAppearance(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.a.String(); got != tc.want {
			t.Errorf("SystemAppearance(%d).String() = %q, want %q", tc.a, got, tc.want)
		}
	}
}

func TestDetectAppearance(t *testing.T) {
	// detectAppearance never returns an error: macOS detection runs first;
	// if unavailable it falls through to Linux and finally returns
	// AppearanceUnknown.
	a := detectAppearance(context.Background())
	if a != AppearanceUnknown && a != AppearanceDark && a != AppearanceLight {
		t.Fatalf("detectAppearance returned invalid value %v", a)
	}
}

// TestDetectLinuxAppearance covers both branches adaptively:
//   - error branch: gsettings is missing or unusable (macOS hosts, non-GNOME
//     Linux, or hosts where gsettings exists but has no schemas) -> unknown.
//   - success branch: real GNOME desktop where gsettings answers -> dark or
//     light depending on the OS color-scheme setting.
func TestDetectLinuxAppearance(t *testing.T) {
	a, err := detectLinuxAppearance(context.Background())
	if err != nil {
		// Error branch: gsettings unavailable / unusable.
		if a != AppearanceUnknown {
			t.Fatalf("appearance = %v, want unknown on error", a)
		}
		return
	}
	// Success branch: real GNOME environment.
	if a != AppearanceDark && a != AppearanceLight {
		t.Fatalf("appearance = %v, want dark or light", a)
	}
}

func TestWatchSystemAppearanceStartAndCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	changed := make(chan SystemAppearance, 4)
	stop := WatchSystemAppearance(ctx, 20*time.Millisecond, func(a SystemAppearance) {
		changed <- a
	})

	// Let a few poll cycles run. Callback firing is not asserted: the OS
	// appearance is stable within the test, so `changed` is usually empty.
	time.Sleep(80 * time.Millisecond)
	stop()

	// Give the watcher goroutine a moment to observe cancellation.
	time.Sleep(30 * time.Millisecond)
	select {
	case a := <-changed:
		// A callback may legitimately fire on the first cycle if the
		// appearance differs from the cached value; accept any valid value.
		if a != AppearanceUnknown && a != AppearanceDark && a != AppearanceLight {
			t.Fatalf("invalid appearance callback value %v", a)
		}
	default:
	}
}
