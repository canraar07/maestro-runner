package devicelab_ios

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// SetupOptions configures runner launch.
type SetupOptions struct {
	// ArtifactsDir contains the built runner. Expected layout:
	//   <ArtifactsDir>/Build/Products/*.xctestrun
	//   <ArtifactsDir>/Build/Products/Debug-iphonesimulator/DevicelabIOSRunner.app
	//   <ArtifactsDir>/Build/Products/Debug-iphonesimulator/DevicelabIOSRunnerUITests-Runner.app
	// In dev: $HOME/.devicelab-ios-runner/derived. In shipped builds the
	// CLI will point at drivers/ios/devicelab-ios-runner under the release
	// bundle.
	ArtifactsDir string

	// SimulatorUDID is the booted iOS simulator's UDID. Required.
	SimulatorUDID string

	// HostBundleID identifies the placeholder app the runner is hosted by.
	// Default "dev.devicelab.runner". Used to verify install.
	HostBundleID string

	// Port the runner should listen on. If 0, we pick an ephemeral port.
	Port int

	// Stdout / Stderr for xcodebuild output. Default os.Stderr.
	Stdout io.Writer
	Stderr io.Writer

	// ReadyTimeout caps how long to wait for the runner to start listening.
	// Default 60s — XCUITest cold-starts the AccessibilityFramework which
	// can take 10-20s on slow machines.
	ReadyTimeout time.Duration
}

// RunnerHandle owns the running xcodebuild subprocess and the chosen port.
type RunnerHandle struct {
	cmd  *exec.Cmd
	port int
	host string
}

// Port returns the resolved listen port.
func (h *RunnerHandle) Port() int { return h.port }

// Host returns the host the runner is reachable on (always 127.0.0.1 for
// sim; would be tunneled for real device).
func (h *RunnerHandle) Host() string { return h.host }

// Stop terminates the runner subprocess. Caller typically also issues a
// `shutdown` command first to let the runner exit cleanly; this is the
// fallback.
func (h *RunnerHandle) Stop() error {
	if h == nil || h.cmd == nil || h.cmd.Process == nil {
		return nil
	}
	// Send SIGTERM first; force-kill after 5s.
	_ = h.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { _ = h.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = h.cmd.Process.Kill()
		<-done
	}
	return nil
}

// Setup launches the runner on the simulator. Returns a Client wired to
// the chosen port and a Handle for shutdown. On error, any partial state
// is rolled back.
func Setup(ctx context.Context, opts SetupOptions) (*Client, *RunnerHandle, error) {
	if opts.ArtifactsDir == "" {
		return nil, nil, errors.New("ArtifactsDir is required")
	}
	if opts.SimulatorUDID == "" {
		return nil, nil, errors.New("SimulatorUDID is required")
	}
	if opts.HostBundleID == "" {
		opts.HostBundleID = "dev.devicelab.runner"
	}
	// Default: route xcodebuild output to a per-build log file so the
	// runner's `t = X.Xs Find the Window…` chatter doesn't drown the
	// user's console. Callers can override via opts.Stdout/Stderr if
	// they want to see it inline (e.g. debugging).
	if opts.Stdout == nil || opts.Stderr == nil {
		logsDir := filepath.Join(opts.ArtifactsDir, "logs")
		_ = os.MkdirAll(logsDir, 0o755)
		logPath := filepath.Join(logsDir, "runner.log")
		logFile, err := os.Create(logPath)
		if err != nil {
			// Fall back to stderr — better some output than blocking startup.
			if opts.Stdout == nil {
				opts.Stdout = os.Stderr
			}
			if opts.Stderr == nil {
				opts.Stderr = os.Stderr
			}
		} else {
			if opts.Stdout == nil {
				opts.Stdout = logFile
			}
			if opts.Stderr == nil {
				opts.Stderr = logFile
			}
		}
	}
	if opts.ReadyTimeout == 0 {
		opts.ReadyTimeout = 60 * time.Second
	}

	port := opts.Port
	if port == 0 {
		var err error
		port, err = pickEphemeralPort()
		if err != nil {
			return nil, nil, fmt.Errorf("pick port: %w", err)
		}
	}

	xctestrun, err := findXctestrun(opts.ArtifactsDir)
	if err != nil {
		return nil, nil, err
	}

	hostAppPath := filepath.Join(opts.ArtifactsDir, "Build/Products/Debug-iphonesimulator/DevicelabIOSRunner.app")
	if err := simctlInstall(ctx, opts.SimulatorUDID, hostAppPath); err != nil {
		return nil, nil, fmt.Errorf("install host app: %w", err)
	}

	if err := injectPortIntoXctestrun(xctestrun, port); err != nil {
		return nil, nil, fmt.Errorf("inject port: %w", err)
	}

	// Pin arch in the destination string. On Xcode 26 + iOS 26 simulators,
	// xcodebuild's destination resolver returns BOTH arm64 and x86_64
	// entries for the same UDID and warns "Using the first of multiple
	// matching destinations". When the resolver picks ambiguously it can
	// hang for minutes — observed as ~40% startup-fail rate on CI before
	// this pin (same root cause as the WDA fix in 822a511).
	destination := fmt.Sprintf(
		"platform=iOS Simulator,arch=%s,id=%s",
		runtime.GOARCH, opts.SimulatorUDID,
	)
	cmd := exec.Command(
		"xcodebuild",
		"test-without-building",
		"-xctestrun", xctestrun,
		"-destination", destination,
	)
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start xcodebuild: %w", err)
	}

	handle := &RunnerHandle{cmd: cmd, port: port, host: "127.0.0.1"}
	client := NewClient(handle.host, port)

	if err := awaitReady(ctx, client, opts.ReadyTimeout); err != nil {
		_ = handle.Stop()
		return nil, nil, fmt.Errorf("runner not ready: %w", err)
	}

	// Pre-warm XCTest's accessibility framework + screenshot path so the
	// first test step doesn't pay the cold-start cost (typically ~1-2s
	// for the first descendants() walk and the first XCUIScreen capture).
	// Best-effort: ignore errors, the actual test will surface real ones.
	warmCtx, warmCancel := context.WithTimeout(ctx, 5*time.Second)
	_, _ = client.Call(warmCtx, Command{Command: CmdScreenshot})
	_, _ = client.Call(warmCtx, Command{Command: CmdSnapshot})
	warmCancel()

	return client, handle, nil
}

// findXctestrun locates the .xctestrun file under <artifactsDir>/Build/Products/.
// Filename varies with arch + iOS version, so we glob.
func findXctestrun(artifactsDir string) (string, error) {
	pattern := filepath.Join(artifactsDir, "Build/Products/*.xctestrun")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no .xctestrun found under %s", pattern)
	}
	return matches[0], nil
}

// injectPortIntoXctestrun edits the xctestrun's nested
// :TestConfigurations:0:TestTargets:0:EnvironmentVariables:DEVICELAB_IOS_RUNNER_PORT
// path so the runner picks up our chosen port at launch.
func injectPortIntoXctestrun(path string, port int) error {
	const key = ":TestConfigurations:0:TestTargets:0:EnvironmentVariables:DEVICELAB_IOS_RUNNER_PORT"
	// Try Add first; if already present, fall through to Set.
	add := exec.Command("/usr/libexec/PlistBuddy",
		"-c", fmt.Sprintf("Add %s string %d", key, port),
		path,
	)
	if err := add.Run(); err == nil {
		return nil
	}
	set := exec.Command("/usr/libexec/PlistBuddy",
		"-c", fmt.Sprintf("Set %s %d", key, port),
		path,
	)
	if out, err := set.CombinedOutput(); err != nil {
		return fmt.Errorf("PlistBuddy set: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// simctlInstall calls `xcrun simctl install <udid> <appPath>`. Reinstalls
// if the app is already present; simctl handles that gracefully.
func simctlInstall(ctx context.Context, udid, appPath string) error {
	if _, err := os.Stat(appPath); err != nil {
		return fmt.Errorf("app not found: %s", appPath)
	}
	cmd := exec.CommandContext(ctx, "xcrun", "simctl", "install", udid, appPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("simctl install: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// awaitReady polls the runner with `uptime` until it answers or the
// deadline passes. Backoff is short (200ms) since the runner usually comes
// up within 10-15s of cold start. agent-device's transport doesn't expose
// a separate /health endpoint, so we use the lightest real command.
func awaitReady(parent context.Context, c *Client, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	deadline := time.Now().Add(timeout)
	for {
		probeCtx, probeCancel := context.WithTimeout(ctx, 2*time.Second)
		err := c.Ping(probeCtx)
		probeCancel()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout after %s: last error: %v", timeout, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// pickEphemeralPort asks the OS for a free port, closes the socket, and
// returns the port number. There is a small race where the OS could
// reassign the same port before xcodebuild claims it, but in practice this
// is fine because the runner is the only thing competing for ephemeral
// ports in this process and xcodebuild claims it within seconds.
func pickEphemeralPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// GracefulShutdown sends a `shutdown` command to the runner, then waits for
// the subprocess to exit. Falls back to SIGTERM after 5s.
func GracefulShutdown(ctx context.Context, c *Client, h *RunnerHandle) error {
	if c != nil {
		_, _ = c.Call(ctx, Command{Command: CmdShutdown})
	}
	return h.Stop()
}
