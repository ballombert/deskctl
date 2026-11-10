package winapi

import (
	"strings"
	"testing"
)

func TestDesktopIDRoundTrip(t *testing.T) {
	const expected = "b8c5a6e2-7f10-4d3a-9c2b-1234567890ab"
	id, err := ParseDesktopID(expected)
	if err != nil {
		t.Fatal(err)
	}
	if actual := FormatDesktopID(id); actual != expected {
		t.Fatalf("GUID round trip = %q, want %q", actual, expected)
	}
	if _, err := ParseDesktopID("not-a-guid"); err == nil {
		t.Fatal("malformed desktop GUID must be rejected")
	}
}

func TestEnumerateVirtualDesktops(t *testing.T) {
	desktops, err := EnumerateVirtualDesktops()
	if err != nil {
		t.Fatal(err)
	}
	if len(desktops) == 0 {
		t.Fatal("expected at least one virtual desktop")
	}
	seen := make(map[string]bool, len(desktops))
	for index, desktop := range desktops {
		if desktop.Index != uint32(index) {
			t.Errorf("desktop index = %d, want %d", desktop.Index, index)
		}
		if _, err := ParseDesktopID(desktop.ID); err != nil {
			t.Errorf("desktop %d has invalid ID: %v", index, err)
		}
		if strings.TrimSpace(desktop.Name) == "" {
			t.Errorf("desktop %d has an empty name", index)
		}
		if seen[desktop.ID] {
			t.Errorf("duplicate desktop ID %s", desktop.ID)
		}
		seen[desktop.ID] = true
		t.Logf("desktop %d: %s (%s)", desktop.Index, desktop.Name, desktop.ID)
	}
}

func TestEnumerateUserWindows(t *testing.T) {
	wins, err := EnumerateUserWindows()
	if err != nil {
		t.Fatalf("EnumerateUserWindows failed: %v", err)
	}

	seen := make(map[uint64]bool)
	processes := make(map[int]bool)
	for _, w := range wins {
		if w.ProcessId <= 0 {
			t.Errorf("Invalid PID: %d", w.ProcessId)
		}
		if w.ProcessName == "" {
			t.Errorf("Empty ProcessName for PID %d", w.ProcessId)
		}
		if _, err := ParseDesktopID(w.DesktopID); err != nil {
			t.Errorf("Invalid desktop GUID for PID %d: %v", w.ProcessId, err)
		}
		if strings.TrimSpace(w.DesktopName) == "" {
			t.Errorf("Empty desktop name for PID %d", w.ProcessId)
		}
		if w.WindowHandle == 0 {
			t.Errorf("Empty window handle for PID %d", w.ProcessId)
		}
		if w.WindowID == 0 {
			t.Errorf("Empty WindowId for PID %d", w.ProcessId)
		}
		if seen[w.WindowID] {
			t.Errorf("Duplicate HWND found: %d", w.WindowID)
		}
		seen[w.WindowID] = true
		processes[w.ProcessId] = true
	}

	t.Logf("EnumerateUserWindows returned %d windows across %d processes:", len(wins), len(processes))
	for _, w := range wins {
		t.Logf("  - PID=%d | %s | %q | %s", w.ProcessId, w.ProcessName, w.WindowTitle, w.DesktopName)
	}
}
