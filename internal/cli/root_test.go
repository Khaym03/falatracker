package cli

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Khaym03/falatracker/internal/app"
)

// setCredentials sets the credential variables for the duration of the test.
// A nil value leaves the variable unset.
func setCredentials(t *testing.T, user, password *string) {
	t.Helper()
	for name, v := range map[string]*string{envUser: user, envPassword: password} {
		t.Setenv(name, "") // restores the original value after the test
		if v == nil {
			os.Unsetenv(name)
		} else {
			os.Setenv(name, *v)
		}
	}
}

// execute runs the root command with args and returns the config passed to run.
func execute(t *testing.T, args ...string) (app.Config, error) {
	t.Helper()
	var got app.Config
	fake := func(_ context.Context, cfg app.Config, _ *slog.Logger) error {
		got = cfg
		return nil
	}

	cmd := newRootCmd(fake)
	// Keep test runs away from the real .env and log directory.
	base := []string{"--env-file", filepath.Join(t.TempDir(), "missing.env"), "--log-file", "", "--template", testTemplate}
	cmd.SetArgs(append(base, args...))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.ExecuteContext(context.Background())
	return got, err
}

func TestDefaults(t *testing.T) {
	setCredentials(t, new("user"), new("secret"))

	cfg, err := execute(t)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if cfg.Interval != 3*time.Minute {
		t.Errorf("Interval = %v, want 3m", cfg.Interval)
	}
	if cfg.HTTPAddr != ":2027" {
		t.Errorf("HTTPAddr = %q, want :2027", cfg.HTTPAddr)
	}
	if cfg.ShutdownTimeout != 5*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 5s", cfg.ShutdownTimeout)
	}
	if cfg.Browser.Headless || cfg.Browser.UserDataDir != "./rod_profile" || cfg.Browser.DebugPort != 9222 || !cfg.Browser.Trace {
		t.Errorf("unexpected browser defaults: %+v", cfg.Browser)
	}
	if cfg.Credential.User != "user" || cfg.Credential.Password != "secret" {
		t.Errorf("unexpected credential: %+v", cfg.Credential)
	}
	if cfg.DataDir != "data" || cfg.Converter == nil {
		t.Errorf("DataDir = %q, Converter = %v", cfg.DataDir, cfg.Converter)
	}
}

// A bad template is detected at startup, before the browser is launched.
func TestInvalidTemplateFailsAtStartup(t *testing.T) {
	setCredentials(t, new("u"), new("p"))
	_, err := execute(t, "--template", filepath.Join(t.TempDir(), "missing.xlsx"))
	if err == nil || !strings.Contains(err.Error(), "open template") {
		t.Fatalf("error = %v, want a template error", err)
	}
}

func TestFlags(t *testing.T) {
	setCredentials(t, new("user"), new("secret"))

	cfg, err := execute(t,
		"--env", "production",
		"--interval", "30s",
		"--addr", "127.0.0.1:9000",
		"--shutdown-timeout", "15s",
		"--headless",
		"--user-data-dir", "/tmp/profile",
		"--debug-port", "0",
		"--trace=false",
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if cfg.Interval != 30*time.Second || cfg.HTTPAddr != "127.0.0.1:9000" || cfg.ShutdownTimeout != 15*time.Second {
		t.Errorf("unexpected config: %+v", cfg)
	}
	if !cfg.Browser.Headless || cfg.Browser.UserDataDir != "/tmp/profile" || cfg.Browser.DebugPort != 0 || cfg.Browser.Trace {
		t.Errorf("unexpected browser options: %+v", cfg.Browser)
	}
}

func TestEnvFileIsLoaded(t *testing.T) {
	setCredentials(t, nil, nil)

	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("USER=from-file\nPASSWORD=file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := execute(t, "--env-file", envFile)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if cfg.Credential.User != "from-file" || cfg.Credential.Password != "file-secret" {
		t.Errorf("credential = %+v, want values from the env file", cfg.Credential)
	}
}

func TestInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		user     *string
		password *string
		args     []string
		wantErr  string
	}{
		{name: "missing password", user: new("user"), wantErr: "missing password"},
		{name: "missing both", wantErr: "missing user and password"},
		{name: "unknown env", user: new("u"), password: new("p"), args: []string{"--env", "staging"}, wantErr: "unknown env"},
		{name: "bad log level", user: new("u"), password: new("p"), args: []string{"--log-level", "loud"}, wantErr: "invalid log level"},
		{name: "zero interval", user: new("u"), password: new("p"), args: []string{"--interval", "0s"}, wantErr: "--interval"},
		{name: "unexpected argument", user: new("u"), password: new("p"), args: []string{"extra"}, wantErr: "unknown command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setCredentials(t, tt.user, tt.password)
			_, err := execute(t, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestRunErrorIsReturned(t *testing.T) {
	setCredentials(t, new("u"), new("p"))
	boom := errors.New("boom")

	cmd := newRootCmd(func(context.Context, app.Config, *slog.Logger) error { return boom })
	cmd.SetArgs([]string{"--env-file", "", "--log-file", "", "--template", testTemplate})
	if err := cmd.ExecuteContext(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v", err, boom)
	}
}

func TestVersionCommand(t *testing.T) {
	cmd := newRootCmd(nil)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.HasPrefix(out.String(), "falatracker ") {
		t.Errorf("output = %q", out.String())
	}
}
