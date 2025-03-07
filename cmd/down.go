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
	"github.com/automationd/atun/internal/ux"
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
			err           error
			bastionHostID string
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

		downTunnelSpinner := ux.NewProgressSpinner("Stopping SSM tunnel")

		bastionHostID = cmd.Flag("bastion").Value.String()

		// If bastion ID is not provided via a flag
		if bastionHostID == "" {
			bastionHostID, err = ssh.GetBastionHostIDFromExistingSession(config.App.Config.TunnelDir)
			if err != nil {
				downTunnelSpinner.UpdateText("Couldn't get bastion host ID locally. Trying with AWS", "error", err)
			}

			downTunnelSpinner.UpdateText("Authenticating with AWS")

			aws.InitAWSClients(config.App)

			config.App.Config.BastionHostID, err = tunnel.GetBastionHostIDFromTags()
			if err != nil {
				downTunnelSpinner.Fail("No Bastion hosts found with atun.io tags and no bastion host provided", "error", err)
			}
		}

		tunnelActive, _, err := ssh.GetSSHTunnelStatus(config.App)
		if err != nil {
			downTunnelSpinner.Fail("Failed to get tunnel status", "error", err)
		}

		if tunnelActive {
			downTunnelSpinner.UpdateText("Tunnel is active", "tunnelActive", tunnelActive, "bastionHostID", config.App.Config.BastionHostID)

			downTunnelSpinner.UpdateText("Deactivating tunnel")
			tunnelActive, err = ssh.StopSSHTunnel(config.App)
			if err != nil {
				downTunnelSpinner.Fail("Failed to deactivate tunnel", "error", err)
			}

			downTunnelSpinner.UpdateText("Tunnel inactive", "tunnelActive", tunnelActive)

		} else {
			downTunnelSpinner.UpdateText("Tunnel inactive", "tunnelActive", tunnelActive)
		}

		// Get delete flag

		deleteBastion, _ := cmd.Flags().GetBool("delete")

		if deleteBastion {
			downTunnelSpinner.UpdateText("Delete flag is set. Deleting bastion host", "bastionHostID", config.App.Config.BastionHostID)

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
