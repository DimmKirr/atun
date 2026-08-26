/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package ssh

import (
	"os"
	"strings"
	"testing"

	"github.com/DimmKirr/atun/internal/config"
)

func newTestApp(t *testing.T, idleTimeout string) *config.Atun {
	t.Helper()
	return &config.Atun{
		Config: &config.Config{
			TunnelDir:      t.TempDir(),
			RouterHostID:   "i-0123456789abcdef0",
			SSHIdleTimeout: idleTimeout,
			Hosts:          []config.Endpoint{},
		},
	}
}

func TestGenerateSSHConfigFile_DefaultKeepsCurrentBehavior(t *testing.T) {
	app := newTestApp(t, "")

	path, err := GenerateSSHConfigFile(app)
	if err != nil {
		t.Fatalf("GenerateSSHConfigFile returned error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read generated ssh config: %v", err)
	}

	got := string(content)

	if !strings.Contains(got, "ServerAliveInterval 180\n") {
		t.Errorf("expected default ServerAliveInterval 180, got:\n%s", got)
	}
	if strings.Contains(got, "ServerAliveCountMax") {
		t.Errorf("expected no ServerAliveCountMax when idle timeout is unset, got:\n%s", got)
	}
}

func TestGenerateSSHConfigFile_IdleTimeoutTightensKeepalive(t *testing.T) {
	app := newTestApp(t, "60s")

	path, err := GenerateSSHConfigFile(app)
	if err != nil {
		t.Fatalf("GenerateSSHConfigFile returned error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read generated ssh config: %v", err)
	}

	got := string(content)

	// 60s idle timeout / 3 probes = 20s interval, ServerAliveCountMax 3
	if !strings.Contains(got, "ServerAliveInterval 20\n") {
		t.Errorf("expected tightened ServerAliveInterval 20, got:\n%s", got)
	}
	if !strings.Contains(got, "ServerAliveCountMax 3\n") {
		t.Errorf("expected ServerAliveCountMax 3, got:\n%s", got)
	}
}

func TestGenerateSSHConfigFile_InvalidIdleTimeoutFallsBackToDefault(t *testing.T) {
	app := newTestApp(t, "not-a-duration")

	path, err := GenerateSSHConfigFile(app)
	if err != nil {
		t.Fatalf("GenerateSSHConfigFile returned error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read generated ssh config: %v", err)
	}

	got := string(content)

	if !strings.Contains(got, "ServerAliveInterval 180\n") {
		t.Errorf("expected fallback to default ServerAliveInterval 180 on invalid duration, got:\n%s", got)
	}
	if strings.Contains(got, "ServerAliveCountMax") {
		t.Errorf("expected no ServerAliveCountMax on invalid duration fallback, got:\n%s", got)
	}
}

func TestGenerateSSHConfigFile_VeryShortIdleTimeoutClampsToOneSecond(t *testing.T) {
	app := newTestApp(t, "2s")

	path, err := GenerateSSHConfigFile(app)
	if err != nil {
		t.Fatalf("GenerateSSHConfigFile returned error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read generated ssh config: %v", err)
	}

	got := string(content)

	if !strings.Contains(got, "ServerAliveInterval 1\n") {
		t.Errorf("expected clamped ServerAliveInterval 1 for very short idle timeout, got:\n%s", got)
	}
	if !strings.Contains(got, "ServerAliveCountMax 3\n") {
		t.Errorf("expected ServerAliveCountMax 3, got:\n%s", got)
	}
}
