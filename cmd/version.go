/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package cmd

import (
	"context"
	"github.com/DimmKirr/atun/internal/config"
	"github.com/DimmKirr/atun/internal/constraints"
	"github.com/DimmKirr/atun/internal/logger"
	"github.com/DimmKirr/atun/internal/ux"
	"github.com/DimmKirr/atun/internal/version"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"os"
	"os/signal"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Version",
	Long:  `Print version`,
	Run: func(cmd *cobra.Command, args []string) {

		pterm.Printfln("Version: %s\n", version.FullVersionNumber())
		//version.CheckLatestRelease()

		// Detect if current terminal is capable of displaying ASCII art
		// If not, disable it
		if !constraints.SupportsANSIEscapeCodes() || constraints.IsCI() {
			logger.Debug("Terminal doesn't support ANSI escape codes", "supportsANSI", constraints.SupportsANSIEscapeCodes())
			logger.Debug("Terminal is CI", "isCI", constraints.IsCI())

			// If the terminal is non-interactive or doesn't support ANSI, enable plain text logging automatically (even if it's set to false)
			config.App.Config.LogPlainText = true
		} else {
			logger.Debug("Terminal supports ANSI escape codes")
			config.App.Config.LogPlainText = false
		}

		if !config.App.Config.LogPlainText {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go func() {
				ux.RenderAsciiArt()
				cancel()
			}()

			// Listen for interrupt signal (ctrl+c)
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt)
			select {
			case <-c:
				cancel()
			case <-ctx.Done():
			}
		}

	},
}

func init() {

}
