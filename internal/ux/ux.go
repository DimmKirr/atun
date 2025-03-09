/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2025 Dmitry Kireev
 */

package ux

import (
	"fmt"
	"github.com/automationd/atun/internal/config"
	"github.com/automationd/atun/internal/logger"
	"github.com/automationd/atun/internal/tunnel"
	"github.com/pterm/pterm"
	"time"
)

// ProgressSpinner is a wrapper around pterm.SpinnerPrinter that
// automatically falls back to logging when spinners are disabled
type ProgressSpinner struct {
	spinner *pterm.SpinnerPrinter
}

// StartCustomSpinner creates and starts a fresh custom spinner
func StartCustomSpinner(message string) *pterm.SpinnerPrinter {
	// Clone DefaultSpinner as a new variable
	spinner := pterm.DefaultSpinner // Direct assignment to create a new copy

	// Customize the spinner
	spinner.Sequence = []string{
		"    🐟",
		"   🐟 ",
		"  🐟  ",
		" 🐟   ",
		"🐟    ",
		"🫧    ",
	}

	spinner.Style = pterm.NewStyle(pterm.FgCyan) // Custom color
	spinner.Delay = 150 * time.Millisecond       // Frame delay

	// Start the spinner
	s, _ := spinner.Start(message)
	return s
}

// NewProgressSpinner creates a new progress spinner or logs a message
// if spinners are disabled
func NewProgressSpinner(message string) *ProgressSpinner {
	ps := &ProgressSpinner{}

	// TODO: make it without side-effects at some point
	if !config.App.Config.LogPlainText {
		ps.spinner = StartCustomSpinner(message)
	} else {
		logger.Info(message)
	}

	return ps
}

// UpdateText updates the spinner text or logs the message
func (ps *ProgressSpinner) UpdateText(message string, keysAndValues ...interface{}) *ProgressSpinner {
	if ps.spinner != nil {
		ps.spinner.UpdateText(message)
	} else {
		logger.Info(message, keysAndValues...)
	}
	return ps
}

// Success marks the spinner as successful or logs a success message
func (ps *ProgressSpinner) Success(message string, keysAndValues ...interface{}) *ProgressSpinner {
	if ps.spinner != nil {
		ps.spinner.Success(message)
	} else {
		logger.Success(message, keysAndValues...)
	}
	return ps
}

func (ps *ProgressSpinner) Fail(message string, keysAndValues ...interface{}) *ProgressSpinner {
	if ps.spinner != nil {
		ps.spinner.Fail(message)
	} else {
		logger.Error(message, keysAndValues...)
	}
	return ps
}

// Warning updates the spinner with a warning or logs a warning message
func (ps *ProgressSpinner) Warning(message string) *ProgressSpinner {
	if ps.spinner != nil {
		ps.spinner.Warning(message)
	} else {
		logger.Warn(message)
	}
	return ps
}

// RenderConnectionsTable renders a table with the connections
func (ps *ProgressSpinner) Status(message string, tunnelIsUp bool, connections [][]string) {
	if ps.spinner != nil {

		err := tunnel.RenderTunnelStatusTable(tunnelIsUp, connections)
		if err != nil {
			logger.Info(fmt.Sprintln(message))
			logger.Error("Error rendering connections table", "error", err, "connections", connections)
		}

	} else {
		logger.Info(fmt.Sprintln(message), "tunnelIsUp", tunnelIsUp, "connections", connections)
	}
}
