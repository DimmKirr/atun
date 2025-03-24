/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package cmd

import (
	"github.com/automationd/atun/internal/aws"
	"github.com/automationd/atun/internal/config"
	"github.com/automationd/atun/internal/infra"
	"github.com/automationd/atun/internal/logger"
	"github.com/automationd/atun/internal/ux"
	"github.com/spf13/cobra"
)

// routerDeleteCmd represents the del command
var routerDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Deletes an ad-hoc router host",
	Long:  `Deletes an ad-hoc router host created by atun. Performed via CDKTF/Terraform: doesn't affect other resources`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Add check for --force flag

		// TODO: Add survey to check if the user is sure to destroy the stack

		deleteRouterInstanceSpinner := ux.NewProgressSpinner("Deleting Ad-Hoc EC2 Router Instance...")

		aws.InitAWSClients(config.App)

		err := infra.DestroyCDKTF(config.App.Config)
		if err != nil {
			deleteRouterInstanceSpinner.Fail("Failed to delete Router Ad-Hoc Instance")

			logger.Error("Error running CDKTF", "error", err)
			return err
		}
		deleteRouterInstanceSpinner.Success("Router Ad-Hoc Instance deleted successfully")

		logger.Info("CDKTF stack destroyed successfully")
		return nil
	},
}

func init() {
}
