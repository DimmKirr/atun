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
	"github.com/spf13/cobra"
)

// deleteCmd represents the del command
var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Deletes an ad-hoc bastion host",
	Long:  `Deletes an ad-hoc bastion host created by atun. Performed via CDKTF/Terraform: doesn't affect other resources`,
	Run: func(cmd *cobra.Command, args []string) {
		// TODO: Add check for --force flag

		// TODO: Add survey to check if the user is sure to destroy the stack

		aws.InitAWSClients(config.App)

		err := infra.DestroyCDKTF(config.App.Config)
		if err != nil {
			logger.Error("Error running CDKTF", "error", err)
			return

		}
		logger.Info("CDKTF stack destroyed successfully")
	},
}

func init() {
	//rootCmd.AddCommand(deleteCmd)
}
