//go:build windows
// +build windows

/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2025 Dmitry Kireev
 */

package ssh

import (
	"os/exec"
	"syscall"
)

func setupSysProcAttr(c *exec.Cmd) {
	// Windows uses different fields for process detachment
	c.SysProcAttr = &syscall.SysProcAttr{
		// Windows-specific settings if needed
		// HideWindow: true,
	}
}
