/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package cmd

import (
	"github.com/automationd/atun/internal/aws"
	"github.com/automationd/atun/internal/config"
	"github.com/automationd/atun/internal/logger"
	"github.com/automationd/atun/internal/ssh"
	"github.com/automationd/atun/internal/tunnel"
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

		logger.Debug("All constraints satisfied")

		tunnelStatus, err := ssh.StopTunnel(config.App)
		if err != nil {
			logger.Error("Failed to stop tunnel", "error", err)
		}

		logger.Debug("Tunnel status", "status", tunnelStatus)
		return nil
	},
}

func init() {
	logger.Debug("Initializing up command")
	// Here you will define your flags and configuration settings.
	downCmd.PersistentFlags().StringP("bastion", "b", "", "Bastion instance id to use. If not specified the first running instance with the atun.io tags is used")
	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// downCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// downCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
