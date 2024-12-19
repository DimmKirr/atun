/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package cmd

import (
	"fmt"
	"github.com/automationd/atun/internal/aws"
	"github.com/automationd/atun/internal/config"
	"github.com/automationd/atun/internal/constraints"
	"github.com/automationd/atun/internal/logger"
	"github.com/automationd/atun/internal/ssh"
	"github.com/automationd/atun/internal/tunnel"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// downCmd represents the down command
var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Bring the tunnel down",
	Long:  `Bring the existing tunnel down.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger.Debug("Down command called")
		var (
			err         error
			bastionHost string
		)

		// Check Constraints
		//if err := constraints.CheckConstraints(
		//	constraints.WithBastionHostID(),
		//); err != nil {
		//	return err
		//}

		if err := constraints.CheckConstraints(
			constraints.WithAWSProfile(),
			//constraints.WithAWSRegion(), // Can be derived on the session level
			constraints.WithENV(),
		); err != nil {
			return err
		}

		var downTunnelSpinner *pterm.SpinnerPrinter
		showSpinner := config.App.Config.LogLevel != "debug" && config.App.Config.LogLevel != "info"

		bastionHost = cmd.Flag("bastion").Value.String()

		aws.InitAWSClients(config.App)

		// If bastion host is not provided, get the first running instance based on the discovery tag (atun.io/version)
		if bastionHost == "" {
			config.App.Config.BastionHostID, err = tunnel.GetBastionHostID()
			if err != nil {
				logger.Fatal("Error discovering bastion host", "error", err)
			}
		} else {
			config.App.Config.BastionHostID = bastionHost
		}

		logger.Debug("Bastion host ID", "bastion", config.App.Config.BastionHostID)

		tunnelStarted, _, err := ssh.GetSSHTunnelStatus(config.App)

		if err != nil {
			logger.Error("Failed to get tunnel status", "error", err)
		}

		if tunnelStarted {
			if showSpinner {
				downTunnelSpinner = logger.StartCustomSpinner("Stopping tunnel...")
			} else {
				logger.Debug("Not showing spinner", "logLevel", config.App.Config.LogLevel)
				logger.Info("Stopping tunnel...")
			}
			logger.Debug("Tunnel status", "status", tunnelStarted)

			logger.Debug("All constraints satisfied")

			tunnelStarted, err = ssh.StopSSHTunnel(config.App)
			if err != nil {
				logger.Error("Failed to stop tunnel", "error", err)
			}
		}
		if showSpinner {
			downTunnelSpinner.Success("Tunnel is stopped")
		} else {
			logger.Debug("Tunnel status", "status", tunnelStarted)
		}

		// Get delete flag

		deleteBastion, _ := cmd.Flags().GetBool("delete")

		if deleteBastion {
			logger.Info("Delete flag is set. Deleting bastion host", "bastion", config.App.Config.BastionHostID)

			// Run create command from here
			err := deleteCmd.RunE(deleteCmd, args)
			if err != nil {
				return fmt.Errorf("error running deleteCmd: %w", err)
			}
		}

		return nil
	},
}

func init() {
	logger.Debug("Initializing up command")
	downCmd.PersistentFlags().StringP("bastion", "b", "", "Bastion instance id to use. If not specified the first running instance with the atun.io tags is used")
	downCmd.PersistentFlags().BoolP("delete", "d", false, "Delete ad-hoc bastion (if exists). Won't delete any resources non-managed by atun")
}
