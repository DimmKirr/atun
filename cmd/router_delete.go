/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package cmd

import (
	"fmt"
	"github.com/automationd/atun/internal/aws"
	"github.com/automationd/atun/internal/config"
	"github.com/automationd/atun/internal/infra"
	"github.com/automationd/atun/internal/logger"
	"github.com/automationd/atun/internal/ux"
	"github.com/pterm/pterm"
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
		ux.Println("Deleting Ad-Hoc EC2 Router Instance...")

		mfaInputRequired := aws.MFAInputRequired(config.App)
		if mfaInputRequired {
			pterm.Printfln(" %s Authenticating with AWS", pterm.LightBlue("▶︎"))
			aws.InitAWSClients(config.App)
		} else {
			spinnerAWSAuth := ux.NewProgressSpinner("Authenticating with AWS")
			aws.InitAWSClients(config.App)
			spinnerAWSAuth.Success(fmt.Sprintf("Authenticated with AWS account %s", aws.GetAccountId()))
		}

		spinnerDestroyCDK := ux.NewProgressSpinner("Destroying CDK of a Router Ad-Hoc Instance")
		err := infra.DestroyCDKTF(config.App.Config)
		if err != nil {
			spinnerDestroyCDK.Fail("Failed to destroy CDK of a Router Ad-Hoc Instance")

			logger.Error("Error running CDKTF", "error", err)
			return err
		}
		spinnerDestroyCDK.Success("Router Ad-Hoc Instance deleted successfully")
		return nil
	},
}

func init() {
}
