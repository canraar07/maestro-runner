package devicelab

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// mockShell is a minimal ShellExecutor that records commands.
type mockShell struct {
	commands []string
	out      string
	err      error
}

func (m *mockShell) Shell(cmd string) (string, error) {
	m.commands = append(m.commands, cmd)
	return m.out, m.err
}

// =============================================================================
// Pure helper functions
// =============================================================================

func TestResolvePermissionShortcut(t *testing.T) {
	cases := []struct {
		shortcut string
		wantOne  string // any one permission expected in the result
		wantLen  int
	}{
		{"location", "android.permission.ACCESS_FINE_LOCATION", 3},
		{"LOCATION", "android.permission.ACCESS_FINE_LOCATION", 3}, // case insensitive
		{"camera", "android.permission.CAMERA", 1},
		{"contacts", "android.permission.READ_CONTACTS", 3},
		{"phone", "android.permission.READ_PHONE_STATE", 6},
		{"microphone", "android.permission.RECORD_AUDIO", 1},
		{"bluetooth", "android.permission.BLUETOOTH_CONNECT", 3},
		{"storage", "android.permission.READ_EXTERNAL_STORAGE", 5},
		{"notifications", "android.permission.POST_NOTIFICATIONS", 1},
		{"medialibrary", "android.permission.READ_MEDIA_IMAGES", 3},
		{"calendar", "android.permission.READ_CALENDAR", 2},
		{"sms", "android.permission.SEND_SMS", 5},
		{"sensors", "android.permission.BODY_SENSORS", 2},
		{"activity_recognition", "android.permission.ACTIVITY_RECOGNITION", 2},
		// Already fully qualified — passthrough
		{"android.permission.READ_PHONE_STATE", "android.permission.READ_PHONE_STATE", 1},
		// Unknown short name → upper-case prefix
		{"foo", "android.permission.FOO", 1},
	}

	for _, c := range cases {
		t.Run(c.shortcut, func(t *testing.T) {
			got := resolvePermissionShortcut(c.shortcut)
			if len(got) != c.wantLen {
				t.Fatalf("len=%d, want %d (got %v)", len(got), c.wantLen, got)
			}
			found := false
			for _, p := range got {
				if p == c.wantOne {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected %q in result %v", c.wantOne, got)
			}
		})
	}
}

func TestGetAllPermissions(t *testing.T) {
	got := getAllPermissions()
	if len(got) < 20 {
		t.Errorf("expected >= 20 runtime permissions, got %d", len(got))
	}
	// Spot-check a few canonical entries.
	want := []string{
		"android.permission.ACCESS_FINE_LOCATION",
		"android.permission.CAMERA",
		"android.permission.RECORD_AUDIO",
		"android.permission.READ_CONTACTS",
	}
	for _, w := range want {
		found := false
		for _, p := range got {
			if p == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("getAllPermissions missing %s", w)
		}
	}
}

func TestAddDotPrefix(t *testing.T) {
	d := &Driver{}
	cases := []struct {
		in, want string
	}{
		// No slash — passthrough
		{"MainActivity", "MainActivity"},
		// Activity already starts with "." — passthrough
		{"com.example/.MainActivity", "com.example/.MainActivity"},
		// Activity already fully qualified (contains a dot in name) — passthrough
		{"com.example/com.example.MainActivity", "com.example/com.example.MainActivity"},
		// Bare activity name — add the leading "."
		{"com.example/MainActivity", "com.example/.MainActivity"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := d.addDotPrefix(c.in); got != c.want {
				t.Errorf("addDotPrefix(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// =============================================================================
// Driver methods that only need a ShellExecutor
// =============================================================================

func TestStopApp_HappyPath(t *testing.T) {
	shell := &mockShell{}
	driver := New(&mockDeviceLabClient{}, &core.PlatformInfo{}, shell)
	res := driver.stopApp(&flow.StopAppStep{AppID: "com.test.app"})
	if !res.Success {
		t.Fatalf("stopApp failed: %v", res.Error)
	}
	if len(shell.commands) != 1 || !strings.Contains(shell.commands[0], "am force-stop com.test.app") {
		t.Errorf("expected 'am force-stop com.test.app', got %v", shell.commands)
	}
}

func TestStopApp_NoAppID(t *testing.T) {
	driver := New(&mockDeviceLabClient{}, &core.PlatformInfo{}, &mockShell{})
	res := driver.stopApp(&flow.StopAppStep{AppID: ""})
	if res.Success {
		t.Error("stopApp with empty appID should fail")
	}
}

func TestStopApp_NoDevice(t *testing.T) {
	driver := New(&mockDeviceLabClient{}, &core.PlatformInfo{}, nil)
	res := driver.stopApp(&flow.StopAppStep{AppID: "com.test.app"})
	if res.Success {
		t.Error("stopApp without ShellExecutor should fail")
	}
}

func TestStopApp_ShellError(t *testing.T) {
	driver := New(&mockDeviceLabClient{}, &core.PlatformInfo{}, &mockShell{err: errors.New("boom")})
	res := driver.stopApp(&flow.StopAppStep{AppID: "com.test.app"})
	if res.Success {
		t.Error("stopApp should fail when shell errors")
	}
}

func TestClearState_HappyPath(t *testing.T) {
	shell := &mockShell{}
	driver := New(&mockDeviceLabClient{}, &core.PlatformInfo{}, shell)
	res := driver.clearState(&flow.ClearStateStep{AppID: "com.test.app"})
	if !res.Success {
		t.Fatalf("clearState failed: %v", res.Error)
	}
	if len(shell.commands) != 1 || shell.commands[0] != "pm clear com.test.app" {
		t.Errorf("expected pm clear command, got %v", shell.commands)
	}
}

func TestClearState_NoAppID(t *testing.T) {
	driver := New(&mockDeviceLabClient{}, &core.PlatformInfo{}, &mockShell{})
	res := driver.clearState(&flow.ClearStateStep{AppID: ""})
	if res.Success {
		t.Error("clearState with empty appID should fail")
	}
}

func TestClearState_NoDevice(t *testing.T) {
	driver := New(&mockDeviceLabClient{}, &core.PlatformInfo{}, nil)
	res := driver.clearState(&flow.ClearStateStep{AppID: "com.test.app"})
	if res.Success {
		t.Error("clearState without ShellExecutor should fail")
	}
}

func TestKillApp_HappyPath(t *testing.T) {
	shell := &mockShell{}
	driver := New(&mockDeviceLabClient{}, &core.PlatformInfo{}, shell)
	res := driver.killApp(&flow.KillAppStep{AppID: "com.test.app"})
	if !res.Success {
		t.Fatalf("killApp failed: %v", res.Error)
	}
	if len(shell.commands) != 1 || !strings.Contains(shell.commands[0], "am force-stop com.test.app") {
		t.Errorf("expected force-stop command, got %v", shell.commands)
	}
}

func TestKillApp_NoAppID(t *testing.T) {
	driver := New(&mockDeviceLabClient{}, &core.PlatformInfo{}, &mockShell{})
	res := driver.killApp(&flow.KillAppStep{AppID: ""})
	if res.Success {
		t.Error("killApp with empty appID should fail")
	}
}

func TestKillApp_NoDevice(t *testing.T) {
	driver := New(&mockDeviceLabClient{}, &core.PlatformInfo{}, nil)
	res := driver.killApp(&flow.KillAppStep{AppID: "com.test.app"})
	if res.Success {
		t.Error("killApp without ShellExecutor should fail")
	}
}

// TestApplyPermissions verifies the grant/revoke dispatch (single
// batched shell command per direction) and the "all" + shortcut
// resolution paths.
func TestApplyPermissions(t *testing.T) {
	t.Run("grant single + revoke single", func(t *testing.T) {
		shell := &mockShell{}
		driver := New(&mockDeviceLabClient{}, &core.PlatformInfo{}, shell)
		res := driver.applyPermissions("com.test.app", map[string]string{
			"camera":     "allow",
			"microphone": "deny",
		})
		if !res.Success {
			t.Fatalf("applyPermissions failed: %v", res.Error)
		}
		// One batched grant + one batched revoke = 2 shell commands.
		if len(shell.commands) != 2 {
			t.Errorf("expected 2 shell calls (grant + revoke), got %d: %v", len(shell.commands), shell.commands)
		}
		// Grant batch contains camera.
		found := false
		for _, c := range shell.commands {
			if strings.Contains(c, "pm grant") && strings.Contains(c, "CAMERA") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected grant batch with CAMERA, got %v", shell.commands)
		}
		// Revoke batch contains record_audio.
		found = false
		for _, c := range shell.commands {
			if strings.Contains(c, "pm revoke") && strings.Contains(c, "RECORD_AUDIO") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected revoke batch with RECORD_AUDIO, got %v", shell.commands)
		}
	})

	t.Run("all=allow grants every permission in one shell call", func(t *testing.T) {
		shell := &mockShell{}
		driver := New(&mockDeviceLabClient{}, &core.PlatformInfo{}, shell)
		res := driver.applyPermissions("com.test.app", map[string]string{"all": "allow"})
		if !res.Success {
			t.Fatalf("applyPermissions failed: %v", res.Error)
		}
		if len(shell.commands) != 1 {
			t.Fatalf("expected 1 shell call, got %d", len(shell.commands))
		}
		// Ensure the batched command contains a representative entry.
		if !strings.Contains(shell.commands[0], "ACCESS_FINE_LOCATION") {
			t.Errorf("all=allow should include ACCESS_FINE_LOCATION; got %s", shell.commands[0])
		}
	})

	t.Run("unset is treated as revoke", func(t *testing.T) {
		shell := &mockShell{}
		driver := New(&mockDeviceLabClient{}, &core.PlatformInfo{}, shell)
		res := driver.applyPermissions("com.test.app", map[string]string{"camera": "unset"})
		if !res.Success {
			t.Fatalf("applyPermissions failed: %v", res.Error)
		}
		if len(shell.commands) != 1 || !strings.Contains(shell.commands[0], "pm revoke") {
			t.Errorf("expected revoke command, got %v", shell.commands)
		}
	})

	t.Run("unknown value is no-op", func(t *testing.T) {
		shell := &mockShell{}
		driver := New(&mockDeviceLabClient{}, &core.PlatformInfo{}, shell)
		res := driver.applyPermissions("com.test.app", map[string]string{"camera": "maybe"})
		if !res.Success {
			t.Fatalf("applyPermissions failed: %v", res.Error)
		}
		if len(shell.commands) != 0 {
			t.Errorf("expected no shell calls for unknown value, got %v", shell.commands)
		}
	})
}

// =============================================================================
// scrollByAdb coordinate geometry (no device required — pure math)
// =============================================================================

func TestScrollByAdb_Coordinates(t *testing.T) {
	const W, H = 1080, 2400
	cases := []struct {
		direction string
		// halfV = H*0.3/2 = 360 → centered at H/2=1200, ±360
		// halfH = W*0.3/2 = 162 → centered at W/2=540, ±162
		wantCmd string
	}{
		{"down", "input swipe 540 1560 540 840 300"},
		{"up", "input swipe 540 840 540 1560 300"},
		{"left", "input swipe 378 1200 702 1200 300"},
		{"right", "input swipe 702 1200 378 1200 300"},
		{"unknown_direction_defaults_to_down", "input swipe 540 1560 540 840 300"},
	}
	for _, c := range cases {
		t.Run(c.direction, func(t *testing.T) {
			shell := &mockShell{}
			driver := &Driver{device: shell}
			direction := c.direction
			if direction == "unknown_direction_defaults_to_down" {
				direction = "diagonal"
			}
			if err := driver.scrollByAdb(direction, W, H, 0.3); err != nil {
				t.Fatalf("scrollByAdb: %v", err)
			}
			if len(shell.commands) != 1 || shell.commands[0] != c.wantCmd {
				t.Errorf("got %q, want %q", shell.commands, c.wantCmd)
			}
		})
	}
}

func TestScrollByAdb_ShellError(t *testing.T) {
	driver := &Driver{device: &mockShell{err: fmt.Errorf("permission denied")}}
	if err := driver.scrollByAdb("down", 1080, 2400, 0.3); err == nil {
		t.Error("expected error from shell")
	}
}

// =============================================================================
// Trackable mock client (extends mockDeviceLabClient with call recording)
// =============================================================================

// trackingClient records the most recent calls. Embeds mockDeviceLabClient
// so unused methods retain their no-op behavior.
type trackingClient struct {
	*mockDeviceLabClient
	backCalls       int
	pressKeyCodes   []int
	clipboardText   string
	orientation     string
	screenshotData  []byte
	screenshotErr   error
	setClipErr      error
	setOrientErr    error
	backErr         error
	pressKeyErr     error
}

func newTrackingClient() *trackingClient {
	return &trackingClient{mockDeviceLabClient: &mockDeviceLabClient{}}
}

func (t *trackingClient) Back() error {
	t.backCalls++
	return t.backErr
}
func (t *trackingClient) PressKeyCode(keyCode int) error {
	t.pressKeyCodes = append(t.pressKeyCodes, keyCode)
	return t.pressKeyErr
}
func (t *trackingClient) Screenshot() ([]byte, error)   { return t.screenshotData, t.screenshotErr }
func (t *trackingClient) SetClipboard(s string) error   { t.clipboardText = s; return t.setClipErr }
func (t *trackingClient) SetOrientation(s string) error { t.orientation = s; return t.setOrientErr }

// =============================================================================
// Driver methods using the tracking client
// =============================================================================

func TestBack(t *testing.T) {
	client := newTrackingClient()
	driver := New(client, &core.PlatformInfo{}, &mockShell{})
	res := driver.back(&flow.BackStep{})
	if !res.Success || client.backCalls != 1 {
		t.Errorf("back: success=%v, calls=%d", res.Success, client.backCalls)
	}
	// Error path
	client.backErr = errors.New("nope")
	res = driver.back(&flow.BackStep{})
	if res.Success {
		t.Error("back should propagate client error")
	}
}

func TestPressKey(t *testing.T) {
	client := newTrackingClient()
	driver := New(client, &core.PlatformInfo{}, &mockShell{})

	// Happy path: known key
	res := driver.pressKey(&flow.PressKeyStep{Key: "enter"})
	if !res.Success {
		t.Fatalf("pressKey enter: %v", res.Error)
	}
	if len(client.pressKeyCodes) != 1 {
		t.Fatalf("expected 1 press, got %d", len(client.pressKeyCodes))
	}

	// Unknown key
	res = driver.pressKey(&flow.PressKeyStep{Key: "this-key-does-not-exist"})
	if res.Success {
		t.Error("pressKey with unknown key should fail")
	}

	// Client error
	client.pressKeyErr = errors.New("nope")
	res = driver.pressKey(&flow.PressKeyStep{Key: "enter"})
	if res.Success {
		t.Error("pressKey should propagate client error")
	}
}

func TestSetClipboard(t *testing.T) {
	client := newTrackingClient()
	driver := New(client, &core.PlatformInfo{}, &mockShell{})

	res := driver.setClipboard(&flow.SetClipboardStep{Text: "hello"})
	if !res.Success || client.clipboardText != "hello" {
		t.Errorf("setClipboard: success=%v, value=%q", res.Success, client.clipboardText)
	}

	// Empty text rejected
	res = driver.setClipboard(&flow.SetClipboardStep{Text: ""})
	if res.Success {
		t.Error("setClipboard empty should fail")
	}

	// Client error
	client.setClipErr = errors.New("nope")
	res = driver.setClipboard(&flow.SetClipboardStep{Text: "hello"})
	if res.Success {
		t.Error("setClipboard should propagate client error")
	}
}

func TestSetOrientation_PortraitLandscape(t *testing.T) {
	client := newTrackingClient()
	driver := New(client, &core.PlatformInfo{}, &mockShell{})

	for _, in := range []string{"PORTRAIT", "portrait", "LANDSCAPE", "landscape"} {
		res := driver.setOrientation(&flow.SetOrientationStep{Orientation: in})
		if !res.Success {
			t.Errorf("setOrientation(%q): %v", in, res.Error)
		}
	}
	// Client error
	client.setOrientErr = errors.New("nope")
	res := driver.setOrientation(&flow.SetOrientationStep{Orientation: "PORTRAIT"})
	if res.Success {
		t.Error("setOrientation should propagate client error")
	}
}

func TestSetOrientation_ExtendedViaShell(t *testing.T) {
	client := newTrackingClient()
	shell := &mockShell{}
	driver := New(client, &core.PlatformInfo{}, shell)

	res := driver.setOrientation(&flow.SetOrientationStep{Orientation: "LANDSCAPE_LEFT"})
	if !res.Success {
		t.Fatalf("LANDSCAPE_LEFT via shell failed: %v", res.Error)
	}
	if len(shell.commands) != 2 {
		t.Errorf("expected 2 shell calls (accelerometer + rotation), got %d: %v", len(shell.commands), shell.commands)
	}
	if !strings.Contains(shell.commands[1], "user_rotation 1") {
		t.Errorf("expected user_rotation 1 (landscape_left), got %q", shell.commands[1])
	}

	// UPSIDE_DOWN
	shell.commands = nil
	driver.setOrientation(&flow.SetOrientationStep{Orientation: "UPSIDE_DOWN"})
	if !strings.Contains(shell.commands[1], "user_rotation 2") {
		t.Errorf("expected user_rotation 2 (upside_down), got %q", shell.commands[1])
	}

	// LANDSCAPE_RIGHT
	shell.commands = nil
	driver.setOrientation(&flow.SetOrientationStep{Orientation: "LANDSCAPE_RIGHT"})
	if !strings.Contains(shell.commands[1], "user_rotation 3") {
		t.Errorf("expected user_rotation 3 (landscape_right), got %q", shell.commands[1])
	}

	// Invalid orientation
	res = driver.setOrientation(&flow.SetOrientationStep{Orientation: "SIDEWAYS"})
	if res.Success {
		t.Error("invalid orientation should fail")
	}

	// Extended orientation but no device
	driverNoShell := New(client, &core.PlatformInfo{}, nil)
	res = driverNoShell.setOrientation(&flow.SetOrientationStep{Orientation: "LANDSCAPE_LEFT"})
	if res.Success {
		t.Error("extended orientation without shell should fail")
	}
}

func TestTakeScreenshot(t *testing.T) {
	client := newTrackingClient()
	client.screenshotData = []byte{0x89, 0x50, 0x4E, 0x47}

	driver := New(client, &core.PlatformInfo{}, &mockShell{})
	res := driver.takeScreenshot(&flow.TakeScreenshotStep{})
	if !res.Success {
		t.Fatalf("takeScreenshot failed: %v", res.Error)
	}
	if data, ok := res.Data.([]byte); !ok || len(data) != 4 {
		t.Errorf("expected 4 bytes of PNG data in result, got %v", res.Data)
	}

	client.screenshotErr = errors.New("device offline")
	res = driver.takeScreenshot(&flow.TakeScreenshotStep{})
	if res.Success {
		t.Error("takeScreenshot should fail when client errors")
	}
}

func TestOpenNotifications(t *testing.T) {
	shell := &mockShell{}
	driver := New(newTrackingClient(), &core.PlatformInfo{}, shell)
	res := driver.openNotifications(&flow.OpenNotificationsStep{})
	if !res.Success {
		t.Fatalf("openNotifications: %v", res.Error)
	}
	if len(shell.commands) != 1 || !strings.Contains(shell.commands[0], "cmd statusbar expand-notifications") {
		t.Errorf("expected statusbar expand command, got %v", shell.commands)
	}

	// No device
	res = New(newTrackingClient(), &core.PlatformInfo{}, nil).openNotifications(&flow.OpenNotificationsStep{})
	if res.Success {
		t.Error("openNotifications without device should fail")
	}

	// Shell error
	res = New(newTrackingClient(), &core.PlatformInfo{}, &mockShell{err: errors.New("blocked")}).openNotifications(&flow.OpenNotificationsStep{})
	if res.Success {
		t.Error("openNotifications should propagate shell error")
	}
}

func TestOpenLink(t *testing.T) {
	shell := &mockShell{}
	driver := New(newTrackingClient(), &core.PlatformInfo{}, shell)

	// Default (no browser flag) — am start VIEW
	res := driver.openLink(&flow.OpenLinkStep{Link: "https://example.com"})
	if !res.Success {
		t.Fatalf("openLink failed: %v", res.Error)
	}
	if !strings.Contains(shell.commands[0], "android.intent.action.VIEW") || !strings.Contains(shell.commands[0], "https://example.com") {
		t.Errorf("unexpected command: %s", shell.commands[0])
	}
	// browser=false path should NOT include the BROWSABLE category
	if strings.Contains(shell.commands[0], "BROWSABLE") {
		t.Errorf("default openLink should not set BROWSABLE: %s", shell.commands[0])
	}

	// browser=true path — force browser via BROWSABLE category
	shell.commands = nil
	browser := true
	res = driver.openLink(&flow.OpenLinkStep{Link: "https://example.com", Browser: &browser})
	if !strings.Contains(shell.commands[0], "BROWSABLE") {
		t.Errorf("openLink with browser=true should add BROWSABLE category: %s", shell.commands[0])
	}

	// Empty link
	res = driver.openLink(&flow.OpenLinkStep{Link: ""})
	if res.Success {
		t.Error("openLink with empty link should fail")
	}

	// No device
	res = New(newTrackingClient(), &core.PlatformInfo{}, nil).openLink(&flow.OpenLinkStep{Link: "https://x"})
	if res.Success {
		t.Error("openLink without device should fail")
	}
}

func TestOpenBrowser(t *testing.T) {
	shell := &mockShell{}
	driver := New(newTrackingClient(), &core.PlatformInfo{}, shell)

	res := driver.openBrowser(&flow.OpenBrowserStep{URL: "https://example.com"})
	if !res.Success {
		t.Fatalf("openBrowser failed: %v", res.Error)
	}

	res = driver.openBrowser(&flow.OpenBrowserStep{URL: ""})
	if res.Success {
		t.Error("openBrowser with empty URL should fail")
	}

	res = New(newTrackingClient(), &core.PlatformInfo{}, nil).openBrowser(&flow.OpenBrowserStep{URL: "https://x"})
	if res.Success {
		t.Error("openBrowser without device should fail")
	}
}

func TestAddMedia(t *testing.T) {
	shell := &mockShell{}
	driver := New(newTrackingClient(), &core.PlatformInfo{}, shell)
	res := driver.addMedia(&flow.AddMediaStep{Files: []string{"/sdcard/a.jpg", "/sdcard/b.mp4"}})
	if !res.Success {
		t.Fatalf("addMedia failed: %v", res.Error)
	}
	if len(shell.commands) != 2 {
		t.Errorf("expected 2 broadcast commands, got %d", len(shell.commands))
	}
	for _, c := range shell.commands {
		if !strings.Contains(c, "MEDIA_SCANNER_SCAN_FILE") {
			t.Errorf("expected MEDIA_SCANNER_SCAN_FILE, got: %s", c)
		}
	}

	// Empty file list
	res = driver.addMedia(&flow.AddMediaStep{Files: nil})
	if res.Success {
		t.Error("addMedia with empty files should fail")
	}

	// No device
	res = New(newTrackingClient(), &core.PlatformInfo{}, nil).addMedia(&flow.AddMediaStep{Files: []string{"/x"}})
	if res.Success {
		t.Error("addMedia without device should fail")
	}
}

func TestRemoveMedia(t *testing.T) {
	// Happy path — at least one provider clears successfully.
	shell := &mockShell{}
	driver := New(newTrackingClient(), &core.PlatformInfo{}, shell)
	res := driver.removeMedia(&flow.RemoveMediaStep{})
	if !res.Success {
		t.Fatalf("removeMedia failed: %v", res.Error)
	}
	// Two providers attempted.
	if len(shell.commands) != 2 {
		t.Errorf("expected 2 pm clear attempts, got %d", len(shell.commands))
	}

	// No device
	res = New(newTrackingClient(), &core.PlatformInfo{}, nil).removeMedia(&flow.RemoveMediaStep{})
	if res.Success {
		t.Error("removeMedia without device should fail")
	}

	// Both providers fail — surfaces error.
	res = New(newTrackingClient(), &core.PlatformInfo{}, &mockShell{err: errors.New("not installed")}).removeMedia(&flow.RemoveMediaStep{})
	if res.Success {
		t.Error("removeMedia should fail when both providers error")
	}
}

func TestStartStopRecording(t *testing.T) {
	shell := &mockShell{}
	driver := New(newTrackingClient(), &core.PlatformInfo{}, shell)

	res := driver.startRecording(&flow.StartRecordingStep{})
	if !res.Success {
		t.Fatalf("startRecording default path failed: %v", res.Error)
	}
	if !strings.Contains(shell.commands[0], "screenrecord /sdcard/recording.mp4") {
		t.Errorf("expected default path, got %s", shell.commands[0])
	}

	shell.commands = nil
	res = driver.startRecording(&flow.StartRecordingStep{Path: "/sdcard/my.mp4"})
	if !strings.Contains(shell.commands[0], "/sdcard/my.mp4") {
		t.Errorf("expected custom path, got %s", shell.commands[0])
	}

	// stop
	shell.commands = nil
	res = driver.stopRecording(&flow.StopRecordingStep{})
	if !res.Success {
		t.Fatalf("stopRecording failed: %v", res.Error)
	}
	if !strings.Contains(shell.commands[0], "pkill -INT screenrecord") {
		t.Errorf("expected pkill, got %s", shell.commands[0])
	}

	// no device
	res = New(newTrackingClient(), &core.PlatformInfo{}, nil).startRecording(&flow.StartRecordingStep{})
	if res.Success {
		t.Error("startRecording without device should fail")
	}
	res = New(newTrackingClient(), &core.PlatformInfo{}, nil).stopRecording(&flow.StopRecordingStep{})
	if res.Success {
		t.Error("stopRecording without device should fail")
	}
}

// =============================================================================
// getAPILevel — shell + caching paths
// =============================================================================

func TestGetAPILevel(t *testing.T) {
	// Happy path: returns the parsed value and caches it.
	shell := &mockShell{out: "33\n"}
	driver := New(newTrackingClient(), &core.PlatformInfo{}, shell)
	if got := driver.getAPILevel(); got != 33 {
		t.Errorf("expected 33, got %d", got)
	}
	if len(shell.commands) != 1 {
		t.Errorf("expected 1 shell call, got %d", len(shell.commands))
	}
	// Second call uses the cache.
	driver.getAPILevel()
	if len(shell.commands) != 1 {
		t.Errorf("expected cached call (still 1 shell call), got %d", len(shell.commands))
	}

	// Shell error → safe default 24.
	driver2 := New(newTrackingClient(), &core.PlatformInfo{}, &mockShell{err: errors.New("nope")})
	if got := driver2.getAPILevel(); got != 24 {
		t.Errorf("expected default 24 on error, got %d", got)
	}

	// Bad output → safe default 24.
	driver3 := New(newTrackingClient(), &core.PlatformInfo{}, &mockShell{out: "not a number"})
	if got := driver3.getAPILevel(); got != 24 {
		t.Errorf("expected default 24 on non-numeric output, got %d", got)
	}
}

// =============================================================================
// resolveLauncherFromDumpsys — pure parser
// =============================================================================

func TestResolveLauncherFromDumpsys_Happy(t *testing.T) {
	// Minimal dumpsys-style snippet with one MAIN/LAUNCHER activity.
	out := `
  com.test.app/com.test.app.MainActivity filter
    Action: "android.intent.action.MAIN"
    Category: "android.intent.category.LAUNCHER"
  com.test.app/com.test.app.SecondActivity filter
    Action: "android.intent.action.VIEW"
`
	shell := &mockShell{out: out}
	driver := New(newTrackingClient(), &core.PlatformInfo{}, shell)
	got, err := driver.resolveLauncherFromDumpsys("com.test.app")
	if err != nil {
		t.Fatalf("resolveLauncherFromDumpsys: %v", err)
	}
	if got != "com.test.app/com.test.app.MainActivity" {
		t.Errorf("got %q, want com.test.app/com.test.app.MainActivity", got)
	}
}

func TestResolveLauncherFromDumpsys_NoMatch(t *testing.T) {
	shell := &mockShell{out: "irrelevant output"}
	driver := New(newTrackingClient(), &core.PlatformInfo{}, shell)
	if _, err := driver.resolveLauncherFromDumpsys("com.test.app"); err == nil {
		t.Error("expected error when no MAIN/LAUNCHER activity is found")
	}
}

func TestResolveLauncherFromDumpsys_ShellError(t *testing.T) {
	shell := &mockShell{err: errors.New("permission denied")}
	driver := New(newTrackingClient(), &core.PlatformInfo{}, shell)
	if _, err := driver.resolveLauncherFromDumpsys("com.test.app"); err == nil {
		t.Error("expected error when shell fails")
	}
}

// =============================================================================
// launchWithMonkey
// =============================================================================

func TestLaunchWithMonkey(t *testing.T) {
	// Happy path
	shell := &mockShell{out: "Events injected: 1"}
	driver := New(newTrackingClient(), &core.PlatformInfo{}, shell)
	res := driver.launchWithMonkey("com.test.app")
	if !res.Success {
		t.Fatalf("launchWithMonkey: %v", res.Error)
	}
	if !strings.Contains(shell.commands[0], "monkey -p com.test.app") {
		t.Errorf("expected monkey command, got %s", shell.commands[0])
	}

	// Output contains "monkey aborted" → failure
	driver2 := New(newTrackingClient(), &core.PlatformInfo{}, &mockShell{out: "monkey aborted: no main activity"})
	res = driver2.launchWithMonkey("com.test.app")
	if res.Success {
		t.Error("monkey aborted output should fail")
	}

	// Shell error
	driver3 := New(newTrackingClient(), &core.PlatformInfo{}, &mockShell{err: errors.New("device offline")})
	res = driver3.launchWithMonkey("com.test.app")
	if res.Success {
		t.Error("shell error should propagate")
	}
}

// =============================================================================
// swipeWithAbsoluteCoords / swipeWithCoordinates
// =============================================================================

func TestSwipeWithAbsoluteCoords(t *testing.T) {
	shell := &mockShell{}
	driver := New(newTrackingClient(), &core.PlatformInfo{}, shell)

	res := driver.swipeWithAbsoluteCoords(100, 200, 300, 400, 500)
	if !res.Success {
		t.Fatalf("swipeWithAbsoluteCoords failed: %v", res.Error)
	}
	if shell.commands[0] != "input swipe 100 200 300 400 500" {
		t.Errorf("unexpected command: %s", shell.commands[0])
	}

	// durationMs <= 0 → default 300
	shell.commands = nil
	driver.swipeWithAbsoluteCoords(1, 2, 3, 4, 0)
	if !strings.HasSuffix(shell.commands[0], " 300") {
		t.Errorf("expected default duration 300, got %s", shell.commands[0])
	}

	// No device
	res = New(newTrackingClient(), &core.PlatformInfo{}, nil).swipeWithAbsoluteCoords(0, 0, 1, 1, 100)
	if res.Success {
		t.Error("swipeWithAbsoluteCoords without device should fail")
	}

	// Shell error
	res = New(newTrackingClient(), &core.PlatformInfo{}, &mockShell{err: errors.New("nope")}).swipeWithAbsoluteCoords(0, 0, 1, 1, 100)
	if res.Success {
		t.Error("swipeWithAbsoluteCoords should propagate shell error")
	}
}

func TestSwipeWithCoordinates(t *testing.T) {
	shell := &mockShell{}
	driver := New(newTrackingClient(), &core.PlatformInfo{ScreenWidth: 1000, ScreenHeight: 2000}, shell)

	// Percentage coords resolve to absolute via screen size.
	res := driver.swipeWithCoordinates("50%,25%", "50%,75%", 200)
	if !res.Success {
		t.Fatalf("swipeWithCoordinates failed: %v", res.Error)
	}
	if shell.commands[0] != "input swipe 500 500 500 1500 200" {
		t.Errorf("expected (500,500)→(500,1500) for 50%%,25%% → 50%%,75%% on 1000x2000, got %s", shell.commands[0])
	}

	// Invalid start coords
	res = driver.swipeWithCoordinates("not-a-coord", "50%,75%", 100)
	if res.Success {
		t.Error("invalid start coords should fail")
	}

	// Invalid end coords
	res = driver.swipeWithCoordinates("50%,25%", "not-a-coord", 100)
	if res.Success {
		t.Error("invalid end coords should fail")
	}

	// No device
	res = New(newTrackingClient(), &core.PlatformInfo{}, nil).swipeWithCoordinates("50%,25%", "50%,75%", 100)
	if res.Success {
		t.Error("swipeWithCoordinates without device should fail")
	}

	// No screen size
	res = New(newTrackingClient(), &core.PlatformInfo{}, &mockShell{}).swipeWithCoordinates("50%,25%", "50%,75%", 100)
	if res.Success {
		t.Error("swipeWithCoordinates without screen size should fail")
	}
}

// =============================================================================
// tapOnPointWithCoords
// =============================================================================

// clickTrackingClient is a trackingClient that also records Click calls.
type clickTrackingClient struct {
	*trackingClient
	clicks   [][2]int
	clickErr error
}

func (c *clickTrackingClient) Click(x, y int) error {
	c.clicks = append(c.clicks, [2]int{x, y})
	return c.clickErr
}

func TestTapOnPointWithCoords(t *testing.T) {
	client := &clickTrackingClient{trackingClient: newTrackingClient()}
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1000, ScreenHeight: 2000}, &mockShell{})

	res := driver.tapOnPointWithCoords("50%,50%")
	if !res.Success {
		t.Fatalf("tapOnPointWithCoords failed: %v", res.Error)
	}
	if len(client.clicks) != 1 || client.clicks[0] != [2]int{500, 1000} {
		t.Errorf("expected click at (500,1000), got %v", client.clicks)
	}

	// Invalid coord
	res = driver.tapOnPointWithCoords("not a coord")
	if res.Success {
		t.Error("invalid point coord should fail")
	}

	// Client error
	client.clickErr = errors.New("nope")
	res = driver.tapOnPointWithCoords("0,0")
	if res.Success {
		t.Error("client click error should propagate")
	}

	// No screen size
	res = New(client, &core.PlatformInfo{}, &mockShell{}).tapOnPointWithCoords("50%,50%")
	if res.Success {
		t.Error("no screen size should fail")
	}
}

// =============================================================================
// hideKeyboard — uses client.HideKeyboard plus a dumpsys check.
// =============================================================================

// hideKbClient counts HideKeyboard invocations.
type hideKbClient struct {
	*trackingClient
	hideCount int
}

func (h *hideKbClient) HideKeyboard() error {
	h.hideCount++
	return nil
}

func TestHideKeyboard_NoDevice_SucceedsImmediately(t *testing.T) {
	// When d.device == nil, isKeyboardVisible() returns false on the first
	// check (getKeyboardBounds returns nil without a shell), so
	// hideKeyboard returns success after a single client call.
	client := &hideKbClient{trackingClient: newTrackingClient()}
	driver := New(client, &core.PlatformInfo{}, nil)

	res := driver.hideKeyboard(&flow.HideKeyboardStep{})
	if !res.Success {
		t.Fatalf("hideKeyboard should succeed when keyboard is reported hidden: %v", res.Error)
	}
	if client.hideCount != 1 {
		t.Errorf("expected 1 HideKeyboard call (no retries), got %d", client.hideCount)
	}
}

// =============================================================================
// isBrowserMode — pure helper
// =============================================================================

func TestIsBrowserMode(t *testing.T) {
	d := &Driver{}
	if d.isBrowserMode() {
		t.Error("isBrowserMode default should be false")
	}
	d.knownCDPType = "browser"
	if !d.isBrowserMode() {
		t.Error("knownCDPType=browser should report true")
	}
	d.knownCDPType = "webview"
	if d.isBrowserMode() {
		t.Error("knownCDPType=webview should NOT be browser mode")
	}
}

// =============================================================================
// mapDirection / mapKeyCode — pure dispatchers
// =============================================================================

func TestMapDirection(t *testing.T) {
	cases := map[string]string{
		"up":      "up",
		"down":    "down",
		"left":    "left",
		"right":   "right",
		"":        "down", // default
		"unknown": "down", // default
	}
	for in, want := range cases {
		if got := mapDirection(in); got != want {
			t.Errorf("mapDirection(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMapKeyCode(t *testing.T) {
	// Just verify the dispatcher returns non-zero for known keys and zero for unknown.
	known := []string{
		"enter", "ENTER", "back", "home", "menu", "delete", "backspace",
		"tab", "space", "volume_up", "volume_down", "power", "camera",
		"search", "dpad_up", "dpad_down", "dpad_left", "dpad_right", "dpad_center",
	}
	for _, k := range known {
		if code := mapKeyCode(k); code == 0 {
			t.Errorf("mapKeyCode(%q) returned 0; known key should map to non-zero", k)
		}
	}
	unknown := []string{"", "asdf", "unknown_key"}
	for _, k := range unknown {
		if code := mapKeyCode(k); code != 0 {
			t.Errorf("mapKeyCode(%q) = %d, want 0", k, code)
		}
	}
}

// =============================================================================
// swipeWithMaestroCoordinates — pure geometry that delegates to absolute swipe
// =============================================================================

func TestSwipeWithMaestroCoordinates(t *testing.T) {
	const W, H = 1000, 2000
	cases := []struct {
		direction string
		wantCmd   string
	}{
		// up: (50%,50%) → (50%,10%)
		{"up", "input swipe 500 1000 500 200 300"},
		// down: (50%,20%) → (50%,90%)
		{"down", "input swipe 500 400 500 1800 300"},
		// left: (90%,50%) → (10%,50%)
		{"left", "input swipe 900 1000 100 1000 300"},
		// right: (10%,50%) → (90%,50%)
		{"right", "input swipe 100 1000 900 1000 300"},
		// default == up
		{"unknown", "input swipe 500 1000 500 200 300"},
	}
	for _, c := range cases {
		t.Run(c.direction, func(t *testing.T) {
			shell := &mockShell{}
			driver := New(newTrackingClient(), &core.PlatformInfo{}, shell)
			res := driver.swipeWithMaestroCoordinates(c.direction, W, H, 300)
			if !res.Success {
				t.Fatalf("swipeWithMaestroCoordinates failed: %v", res.Error)
			}
			if shell.commands[0] != c.wantCmd {
				t.Errorf("got %q, want %q", shell.commands[0], c.wantCmd)
			}
		})
	}
}

// =============================================================================
// setLocation
// =============================================================================

func TestSetLocation(t *testing.T) {
	shell := &mockShell{}
	driver := New(newTrackingClient(), &core.PlatformInfo{}, shell)

	res := driver.setLocation(&flow.SetLocationStep{Latitude: "37.7749", Longitude: "-122.4194"})
	if !res.Success {
		t.Fatalf("setLocation failed: %v", res.Error)
	}
	if !strings.Contains(shell.commands[0], "MOCK_LOCATION") ||
		!strings.Contains(shell.commands[0], "37.7749") ||
		!strings.Contains(shell.commands[0], "-122.4194") {
		t.Errorf("unexpected setLocation command: %s", shell.commands[0])
	}

	// Missing lat
	res = driver.setLocation(&flow.SetLocationStep{Latitude: "", Longitude: "-122"})
	if res.Success {
		t.Error("missing latitude should fail")
	}
	// Missing lon
	res = driver.setLocation(&flow.SetLocationStep{Latitude: "37", Longitude: ""})
	if res.Success {
		t.Error("missing longitude should fail")
	}
	// No device
	res = New(newTrackingClient(), &core.PlatformInfo{}, nil).setLocation(&flow.SetLocationStep{Latitude: "1", Longitude: "2"})
	if res.Success {
		t.Error("setLocation without device should fail")
	}
	// Shell error
	res = New(newTrackingClient(), &core.PlatformInfo{}, &mockShell{err: errors.New("blocked")}).setLocation(&flow.SetLocationStep{Latitude: "1", Longitude: "2"})
	if res.Success {
		t.Error("setLocation should propagate shell error")
	}
}

// =============================================================================
// applyAirplaneMode / setAirplaneMode / toggleAirplaneMode
// =============================================================================

func TestApplyAirplaneMode_ModernPath(t *testing.T) {
	// Modern command succeeds → no fallback shell calls.
	shell := &mockShell{out: "Mode enabled"}
	driver := New(newTrackingClient(), &core.PlatformInfo{}, shell)
	res := driver.applyAirplaneMode(true)
	if !res.Success {
		t.Fatalf("applyAirplaneMode enable failed: %v", res.Error)
	}
	if !strings.Contains(shell.commands[0], "cmd connectivity airplane-mode enable") {
		t.Errorf("expected cmd connectivity enable, got %s", shell.commands[0])
	}
	if len(shell.commands) != 1 {
		t.Errorf("modern path should only issue 1 shell call, got %d", len(shell.commands))
	}

	// Disable path
	shell.commands = nil
	driver.applyAirplaneMode(false)
	if !strings.Contains(shell.commands[0], "disable") {
		t.Errorf("expected disable, got %s", shell.commands[0])
	}
}

func TestApplyAirplaneMode_FallbackPath(t *testing.T) {
	// "Unknown command" in output triggers the settings + broadcast fallback.
	calls := 0
	shell := &fakeMultiShell{
		outputs: []string{"Unknown command: airplane-mode", "", ""},
		errs:    []error{nil, nil, nil},
		counter: &calls,
	}
	driver := New(newTrackingClient(), &core.PlatformInfo{}, shell)
	res := driver.applyAirplaneMode(true)
	if !res.Success {
		t.Fatalf("applyAirplaneMode fallback failed: %v", res.Error)
	}
	if calls != 3 {
		t.Errorf("expected 3 shell calls (modern attempt + settings put + broadcast), got %d", calls)
	}
	if !strings.Contains(shell.commands[1], "settings put global airplane_mode_on 1") {
		t.Errorf("expected fallback settings command, got %s", shell.commands[1])
	}
}

func TestApplyAirplaneMode_NoDevice(t *testing.T) {
	res := New(newTrackingClient(), &core.PlatformInfo{}, nil).applyAirplaneMode(true)
	if res.Success {
		t.Error("applyAirplaneMode without device should fail")
	}
}

func TestSetAirplaneMode_DelegatesToApply(t *testing.T) {
	shell := &mockShell{out: "OK"}
	driver := New(newTrackingClient(), &core.PlatformInfo{}, shell)
	res := driver.setAirplaneMode(&flow.SetAirplaneModeStep{Enabled: true})
	if !res.Success {
		t.Fatalf("setAirplaneMode failed: %v", res.Error)
	}
}

func TestToggleAirplaneMode(t *testing.T) {
	// Currently disabled (0) → toggle should enable.
	shell := &fakeMultiShell{
		outputs: []string{"0\n", "OK"},
		errs:    []error{nil, nil},
		counter: new(int),
	}
	driver := New(newTrackingClient(), &core.PlatformInfo{}, shell)
	res := driver.toggleAirplaneMode(&flow.ToggleAirplaneModeStep{})
	if !res.Success {
		t.Fatalf("toggleAirplaneMode failed: %v", res.Error)
	}
	if !strings.Contains(shell.commands[1], "enable") {
		t.Errorf("expected enable command (toggling from 0); got %s", shell.commands[1])
	}

	// Currently enabled (1) → toggle should disable.
	shell2 := &fakeMultiShell{
		outputs: []string{"1\n", "OK"},
		errs:    []error{nil, nil},
		counter: new(int),
	}
	driver2 := New(newTrackingClient(), &core.PlatformInfo{}, shell2)
	driver2.toggleAirplaneMode(&flow.ToggleAirplaneModeStep{})
	if !strings.Contains(shell2.commands[1], "disable") {
		t.Errorf("expected disable command (toggling from 1); got %s", shell2.commands[1])
	}

	// No device
	res = New(newTrackingClient(), &core.PlatformInfo{}, nil).toggleAirplaneMode(&flow.ToggleAirplaneModeStep{})
	if res.Success {
		t.Error("toggleAirplaneMode without device should fail")
	}

	// Get fails
	res = New(newTrackingClient(), &core.PlatformInfo{}, &mockShell{err: errors.New("blocked")}).toggleAirplaneMode(&flow.ToggleAirplaneModeStep{})
	if res.Success {
		t.Error("toggleAirplaneMode should propagate get error")
	}
}

// fakeMultiShell returns scripted (output, err) pairs in sequence.
type fakeMultiShell struct {
	outputs  []string
	errs     []error
	idx      int
	commands []string
	counter  *int // optional external call counter
}

func (f *fakeMultiShell) Shell(cmd string) (string, error) {
	f.commands = append(f.commands, cmd)
	if f.counter != nil {
		*f.counter++
	}
	if f.idx >= len(f.outputs) {
		return "", nil
	}
	out := f.outputs[f.idx]
	err := f.errs[f.idx]
	f.idx++
	return out, err
}

// =============================================================================
// waitForAnimationToEnd
// =============================================================================

// screenshotClient returns scripted screenshots in sequence so we can
// simulate "screen settled" vs "still animating".
type screenshotClient struct {
	*trackingClient
	frames [][]byte
	idx    int
	err    error
}

func (s *screenshotClient) Screenshot() ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.idx >= len(s.frames) {
		return s.frames[len(s.frames)-1], nil
	}
	out := s.frames[s.idx]
	s.idx++
	return out, nil
}

func TestWaitForAnimationToEnd_SettlesQuickly(t *testing.T) {
	// Two identical frames → diff is 0% → returns success immediately.
	identical := []byte{
		// Minimal 1x1 PNG header so ImageDifference can decode it.
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x00, 0x00, 0x00, 0x00, 0x3B, 0x7E, 0x9B,
		0x55, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9C, 0x63, 0x00, 0x00, 0x00, 0x02,
		0x00, 0x01, 0xE5, 0x27, 0xDE, 0xFC, 0x00, 0x00,
		0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42,
		0x60, 0x82,
	}
	client := &screenshotClient{
		trackingClient: newTrackingClient(),
		frames:         [][]byte{identical, identical},
	}
	driver := New(client, &core.PlatformInfo{}, &mockShell{})
	res := driver.waitForAnimationToEnd(&flow.WaitForAnimationToEndStep{})
	if !res.Success {
		t.Fatalf("waitForAnimationToEnd should succeed on identical frames: %v", res.Error)
	}
}

func TestWaitForAnimationToEnd_ScreenshotError(t *testing.T) {
	client := &screenshotClient{
		trackingClient: newTrackingClient(),
		err:            errors.New("screenshot blocked"),
	}
	driver := New(client, &core.PlatformInfo{}, &mockShell{})
	res := driver.waitForAnimationToEnd(&flow.WaitForAnimationToEndStep{})
	if res.Success {
		t.Error("waitForAnimationToEnd should fail when screenshot errors")
	}
}

// =============================================================================
// looksLikeRegex — pure dispatcher
// =============================================================================

func TestLooksLikeRegex(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"hello", false},
		{"hello world", false},
		{"^anchor", true},
		{"end$", true},
		{"middle$x", false}, // $ not at end
		{"x^start", false},  // ^ not at start
		{".*wildcard", true},
		{".+wildcard", true},
		{".?optional", true},
		{"literal.dot", false}, // dot without quantifier
		{"a*b", true},          // *
		{"a+b", true},          // +
		{"a?b", true},          // ?
		{"set[abc]", true},     // [
		{"end]", true},         // ]
		{"group()", true},      // (
		{"alt|alt", true},      // |
		{"\\.escaped", false},  // escaped dot
		{"", false},
	}
	for _, c := range cases {
		if got := looksLikeRegex(c.in); got != c.want {
			t.Errorf("looksLikeRegex(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// =============================================================================
// escapeUIAutomatorString
// =============================================================================

func TestEscapeUIAutomatorString(t *testing.T) {
	cases := map[string]string{
		"":          "",
		"plain":     "plain",
		`"quoted"`:  `\"quoted\"`,
		`a"b"c`:     `a\"b\"c`,
		`single'`:   `single'`,
	}
	for in, want := range cases {
		if got := escapeUIAutomatorString(in); got != want {
			t.Errorf("escapeUIAutomatorString(%q) = %q, want %q", in, got, want)
		}
	}
}

// =============================================================================
// buildStateFilters
// =============================================================================

func TestBuildStateFilters(t *testing.T) {
	tr, fa := true, false
	cases := []struct {
		name string
		sel  flow.Selector
		want string
	}{
		{"empty", flow.Selector{}, ""},
		{"enabled=true", flow.Selector{Enabled: &tr}, ".enabled(true)"},
		{"enabled=false", flow.Selector{Enabled: &fa}, ".enabled(false)"},
		{"selected=true", flow.Selector{Selected: &tr}, ".selected(true)"},
		{"checked=true", flow.Selector{Checked: &tr}, ".checked(true)"},
		{"focused=true", flow.Selector{Focused: &tr}, ".focused(true)"},
		{"multi", flow.Selector{Enabled: &tr, Selected: &fa},
			".enabled(true).selected(false)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := buildStateFilters(c.sel); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// =============================================================================
// calculateTimeout
// =============================================================================

func TestCalculateTimeout(t *testing.T) {
	d := &Driver{}
	// stepTimeoutMs > 0 wins
	if got := d.calculateTimeout(true, 5000); got != 5*time.Second {
		t.Errorf("step timeout: got %v, want 5s", got)
	}
	if got := d.calculateTimeout(false, 5000); got != 5*time.Second {
		t.Errorf("step timeout: got %v, want 5s", got)
	}

	// optional=true uses OptionalFindTimeout default
	if got := d.calculateTimeout(true, 0); got != time.Duration(OptionalFindTimeout)*time.Millisecond {
		t.Errorf("optional default: got %v", got)
	}
	// driver-level optionalFindTimeout override
	d.optionalFindTimeout = 1234
	if got := d.calculateTimeout(true, 0); got != 1234*time.Millisecond {
		t.Errorf("driver optionalFindTimeout override: got %v", got)
	}

	// !optional uses DefaultFindTimeout
	d2 := &Driver{}
	if got := d2.calculateTimeout(false, 0); got != time.Duration(DefaultFindTimeout)*time.Millisecond {
		t.Errorf("required default: got %v", got)
	}
	// driver-level findTimeout override
	d2.findTimeout = 5678
	if got := d2.calculateTimeout(false, 0); got != 5678*time.Millisecond {
		t.Errorf("driver findTimeout override: got %v", got)
	}
}

// =============================================================================
// tapOnPoint (no findElement — testable via Click/LongClick mocks)
// =============================================================================

// clickLongClickClient extends trackingClient with Click and LongClick recording.
type clickLongClickClient struct {
	*trackingClient
	clicks       [][2]int
	longClicks   []longClickArgs
	clickErr     error
	longClickErr error
}
type longClickArgs struct {
	X, Y, Duration int
}

func (c *clickLongClickClient) Click(x, y int) error {
	c.clicks = append(c.clicks, [2]int{x, y})
	return c.clickErr
}
func (c *clickLongClickClient) LongClick(x, y, duration int) error {
	c.longClicks = append(c.longClicks, longClickArgs{x, y, duration})
	return c.longClickErr
}

func TestTapOnPoint_AbsoluteXY(t *testing.T) {
	client := &clickLongClickClient{trackingClient: newTrackingClient()}
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1000, ScreenHeight: 2000}, &mockShell{})

	res := driver.tapOnPoint(&flow.TapOnPointStep{X: 100, Y: 200})
	if !res.Success {
		t.Fatalf("tapOnPoint(100, 200) failed: %v", res.Error)
	}
	if len(client.clicks) != 1 || client.clicks[0] != [2]int{100, 200} {
		t.Errorf("expected click at (100, 200), got %v", client.clicks)
	}
}

func TestTapOnPoint_PercentageString(t *testing.T) {
	client := &clickLongClickClient{trackingClient: newTrackingClient()}
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1000, ScreenHeight: 2000}, &mockShell{})

	res := driver.tapOnPoint(&flow.TapOnPointStep{Point: "50%,50%"})
	if !res.Success {
		t.Fatalf("tapOnPoint(50%%,50%%) failed: %v", res.Error)
	}
	if len(client.clicks) != 1 || client.clicks[0] != [2]int{500, 1000} {
		t.Errorf("expected click at (500, 1000), got %v", client.clicks)
	}
}

func TestTapOnPoint_LongPress(t *testing.T) {
	client := &clickLongClickClient{trackingClient: newTrackingClient()}
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1000, ScreenHeight: 2000}, &mockShell{})

	// Explicit DurationMs
	res := driver.tapOnPoint(&flow.TapOnPointStep{X: 50, Y: 50, DurationMs: 2000})
	if !res.Success {
		t.Fatalf("tapOnPoint long press failed: %v", res.Error)
	}
	if len(client.longClicks) != 1 || client.longClicks[0] != (longClickArgs{50, 50, 2000}) {
		t.Errorf("expected LongClick(50,50,2000), got %v", client.longClicks)
	}

	// LongPress flag with no duration → default 1000
	client.longClicks = nil
	driver.tapOnPoint(&flow.TapOnPointStep{X: 10, Y: 20, LongPress: true})
	if len(client.longClicks) != 1 || client.longClicks[0].Duration != 1000 {
		t.Errorf("LongPress should default to 1000ms; got %v", client.longClicks)
	}
}

func TestTapOnPoint_Errors(t *testing.T) {
	client := &clickLongClickClient{trackingClient: newTrackingClient()}
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1000, ScreenHeight: 2000}, &mockShell{})

	// No point and zero X/Y → fail
	res := driver.tapOnPoint(&flow.TapOnPointStep{})
	if res.Success {
		t.Error("tapOnPoint with no point and zero coords should fail")
	}

	// Bad percentage string
	res = driver.tapOnPoint(&flow.TapOnPointStep{Point: "not-a-coord"})
	if res.Success {
		t.Error("tapOnPoint with bad point should fail")
	}

	// Click error
	client.clickErr = errors.New("click failed")
	res = driver.tapOnPoint(&flow.TapOnPointStep{X: 100, Y: 100})
	if res.Success {
		t.Error("tapOnPoint should propagate click error")
	}

	// LongClick error
	client.clickErr = nil
	client.longClickErr = errors.New("long click failed")
	res = driver.tapOnPoint(&flow.TapOnPointStep{X: 100, Y: 100, LongPress: true})
	if res.Success {
		t.Error("tapOnPoint should propagate long click error")
	}

	// No screen size for percentage point
	driverNoSize := New(client, &core.PlatformInfo{}, &mockShell{})
	res = driverNoSize.tapOnPoint(&flow.TapOnPointStep{Point: "50%,50%"})
	if res.Success {
		t.Error("tapOnPoint percentage without screen size should fail")
	}
}

// =============================================================================
// swipe — non-find paths
// =============================================================================

func TestSwipe_StartEndStrings(t *testing.T) {
	shell := &mockShell{}
	driver := New(newTrackingClient(), &core.PlatformInfo{ScreenWidth: 1000, ScreenHeight: 2000}, shell)
	res := driver.swipe(&flow.SwipeStep{Start: "50%,25%", End: "50%,75%", Duration: 100})
	if !res.Success {
		t.Fatalf("swipe start/end failed: %v", res.Error)
	}
	if shell.commands[0] != "input swipe 500 500 500 1500 100" {
		t.Errorf("unexpected swipe command: %s", shell.commands[0])
	}
}

func TestSwipe_AbsoluteXY(t *testing.T) {
	shell := &mockShell{}
	driver := New(newTrackingClient(), &core.PlatformInfo{ScreenWidth: 1000, ScreenHeight: 2000}, shell)
	res := driver.swipe(&flow.SwipeStep{StartX: 10, StartY: 20, EndX: 100, EndY: 200, Duration: 50})
	if !res.Success {
		t.Fatalf("swipe absolute failed: %v", res.Error)
	}
	if shell.commands[0] != "input swipe 10 20 100 200 50" {
		t.Errorf("unexpected swipe command: %s", shell.commands[0])
	}
}

func TestSwipe_DirectionOnly(t *testing.T) {
	shell := &mockShell{}
	driver := New(newTrackingClient(), &core.PlatformInfo{ScreenWidth: 1000, ScreenHeight: 2000}, shell)
	// No start/end, no x/y, no selector → use Maestro coordinates
	res := driver.swipe(&flow.SwipeStep{Direction: "up", Duration: 300})
	if !res.Success {
		t.Fatalf("swipe direction failed: %v", res.Error)
	}
	// swipeWithMaestroCoordinates: up = (50%,50%) → (50%,10%)
	if shell.commands[0] != "input swipe 500 1000 500 200 300" {
		t.Errorf("unexpected swipe command for direction=up: %s", shell.commands[0])
	}

	// Empty direction defaults to up
	shell.commands = nil
	driver.swipe(&flow.SwipeStep{Direction: "", Duration: 300})
	if shell.commands[0] != "input swipe 500 1000 500 200 300" {
		t.Errorf("default direction should be up, got: %s", shell.commands[0])
	}
}

// =============================================================================
// checkKeyboardBlocking — pure helper
// =============================================================================

func TestCheckKeyboardBlocking_NotInput(t *testing.T) {
	d := &Driver{}
	// wasInput=false → no check at all
	if r := d.checkKeyboardBlocking(false, flow.Selector{Text: "foo"}); r != nil {
		t.Errorf("expected nil for wasInput=false, got %+v", r)
	}
}

func TestCheckKeyboardBlocking_NoSelector(t *testing.T) {
	d := &Driver{}
	// Empty selector with wasInput=true: no element to check; returns nil
	if r := d.checkKeyboardBlocking(true, flow.Selector{}); r != nil {
		t.Errorf("expected nil for empty selector, got %+v", r)
	}
}

// =============================================================================
// Driver delegators / setters / getters
// =============================================================================

// settleableClient + sourceful client extends trackingClient with
// scriptable Source(), GetOrientation(), GetClipboard(), WaitForSettle().
type richClient struct {
	*trackingClient
	source       string
	sourceErr    error
	orientation  string
	clipboard    string
	settleQuiet  bool
	settleErr    error
	applySettErr error
}

func (r *richClient) Source() (string, error)                            { return r.source, r.sourceErr }
func (r *richClient) GetOrientation() (string, error)                    { return r.orientation, nil }
func (r *richClient) GetClipboard() (string, error)                      { return r.clipboard, nil }
func (r *richClient) WaitForSettle(timeoutMs, quietMs int) (bool, error) { return r.settleQuiet, r.settleErr }
func (r *richClient) SetAppiumSettings(settings map[string]interface{}) error {
	return r.applySettErr
}

func TestDriver_Setters(t *testing.T) {
	client := &richClient{trackingClient: newTrackingClient()}
	d := New(client, &core.PlatformInfo{}, &mockShell{})

	d.SetCDPStateFunc(func() *core.CDPInfo { return &core.CDPInfo{} })
	if d.cdpStateFunc == nil {
		t.Error("SetCDPStateFunc should set the field")
	}
	if d.CDPState() == nil {
		t.Error("CDPState should return non-nil after SetCDPStateFunc")
	}

	// Without the func, CDPState returns nil
	d2 := New(client, &core.PlatformInfo{}, &mockShell{})
	if d2.CDPState() != nil {
		t.Error("CDPState should be nil by default")
	}

	d.SetFindTimeout(1234)
	if d.findTimeout != 1234 {
		t.Errorf("SetFindTimeout: got %d, want 1234", d.findTimeout)
	}
	d.SetOptionalFindTimeout(5678)
	if d.optionalFindTimeout != 5678 {
		t.Errorf("SetOptionalFindTimeout: got %d", d.optionalFindTimeout)
	}

	// SetWaitForIdleTimeout — happy + negative clamp + error
	if err := d.SetWaitForIdleTimeout(100); err != nil {
		t.Errorf("SetWaitForIdleTimeout(100): %v", err)
	}
	if err := d.SetWaitForIdleTimeout(-5); err != nil {
		t.Errorf("SetWaitForIdleTimeout(-5) should clamp, not error: %v", err)
	}
	client.applySettErr = errors.New("nope")
	if err := d.SetWaitForIdleTimeout(100); err == nil {
		t.Error("SetWaitForIdleTimeout should propagate client error")
	}
}

func TestDriver_Close(t *testing.T) {
	d := New(newTrackingClient(), &core.PlatformInfo{}, &mockShell{})
	// No webView set → Close is a no-op (no panic).
	d.Close()
}

func TestDriver_SetContext(t *testing.T) {
	d := New(newTrackingClient(), &core.PlatformInfo{}, &mockShell{})
	// parentContext returns Background by default.
	if d.parentContext() == nil {
		t.Error("parentContext should never return nil")
	}
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "v")
	d.SetContext(ctx)
	if d.parentContext().Value(ctxKey{}) != "v" {
		t.Error("SetContext should override parentContext")
	}
}

func TestDriver_Screenshot(t *testing.T) {
	client := &screenshotClient{
		trackingClient: newTrackingClient(),
		frames:         [][]byte{{1, 2, 3}},
	}
	d := New(client, &core.PlatformInfo{}, &mockShell{})
	data, err := d.Screenshot()
	if err != nil || len(data) != 3 {
		t.Errorf("Screenshot: data=%v err=%v", data, err)
	}
}

func TestDriver_Hierarchy(t *testing.T) {
	client := &richClient{
		trackingClient: newTrackingClient(),
		source:         "<hierarchy/>",
	}
	d := New(client, &core.PlatformInfo{}, &mockShell{})
	got, err := d.Hierarchy()
	if err != nil {
		t.Fatalf("Hierarchy: %v", err)
	}
	if string(got) != "<hierarchy/>" {
		t.Errorf("Hierarchy: got %q", got)
	}

	// Source error propagates
	client.sourceErr = errors.New("nope")
	if _, err := d.Hierarchy(); err == nil {
		t.Error("Hierarchy should propagate Source error")
	}
}

func TestDriver_GetState(t *testing.T) {
	client := &richClient{
		trackingClient: newTrackingClient(),
		orientation:    "LANDSCAPE",
		clipboard:      "copied",
	}
	d := New(client, &core.PlatformInfo{}, &mockShell{})
	state := d.GetState()
	if state == nil {
		t.Fatal("GetState returned nil")
	}
	if state.Orientation != "landscape" {
		t.Errorf("orientation: got %q, want landscape", state.Orientation)
	}
	if state.ClipboardText != "copied" {
		t.Errorf("clipboard: got %q", state.ClipboardText)
	}
}

func TestDriver_GetPlatformInfo(t *testing.T) {
	info := &core.PlatformInfo{Platform: "android"}
	d := New(newTrackingClient(), info, &mockShell{})
	if got := d.GetPlatformInfo(); got != info {
		t.Errorf("GetPlatformInfo: got %+v, want %+v", got, info)
	}
}

func TestDriver_WaitForSettle(t *testing.T) {
	client := &richClient{trackingClient: newTrackingClient(), settleQuiet: true}
	d := New(client, &core.PlatformInfo{}, &mockShell{})
	quiet, err := d.WaitForSettle(1000, 500)
	if err != nil || !quiet {
		t.Errorf("WaitForSettle: quiet=%v err=%v", quiet, err)
	}

	client.settleErr = errors.New("nope")
	if _, err := d.WaitForSettle(1000, 500); err == nil {
		t.Error("WaitForSettle should propagate client error")
	}
}
