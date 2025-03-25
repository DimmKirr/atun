//go:build !windows
// +build !windows

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
	c.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Detach process from the parent group
	}
}
