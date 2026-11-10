package winapi

import (
	"syscall"
	"testing"
)

func TestDisplayInfoFromMonitor(t *testing.T) {
	device, err := syscall.UTF16FromString(`\\.\DISPLAY2`)
	if err != nil {
		t.Fatal(err)
	}
	var info monitorInfoEx
	copy(info.DeviceName[:], device)
	info.Monitor = displayRect{Left: -1920, Top: 0, Right: 0, Bottom: 1080}
	info.Flags = 1

	display := displayInfoFromMonitor(info)
	if display.ID != `\\.\DISPLAY2` || display.X != -1920 || display.Width != 1920 || display.Height != 1080 || !display.Primary {
		t.Fatalf("unexpected monitor data: %+v", display)
	}
}
