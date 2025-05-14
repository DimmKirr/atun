/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package cmd

import (
	"fmt"
	"github.com/DimmKirr/atun/internal/aws"
	"github.com/DimmKirr/atun/internal/config"
	"github.com/DimmKirr/atun/internal/logger"
	"github.com/DimmKirr/atun/internal/ssh"
	"github.com/DimmKirr/atun/internal/tunnel"
	"github.com/DimmKirr/atun/internal/ux"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of the tunnel and current environment",
	Long: `Show status of the tunnel and current environment.
	This is also useful for troubleshooting`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var routerHostID string
		var err error

		if err != nil {
			return fmt.Errorf("can't load options for a command: %w", err)
		}

		detailedStatus, err := cmd.Flags().GetBool("detailed")
		if err != nil {
			return fmt.Errorf("can't get detailed flag: %w", err)
		}

		ux.Println("Checking Tunnel Status")

		// Get the router host ID from the command line
		routerHostID = cmd.Flag("router").Value.String()

		// If router host is not provided, get the first running instance based on the discovery tag (atun.io/version)
		if routerHostID == "" {
			mfaInputRequired := aws.MFAInputRequired(config.App)

			if mfaInputRequired {
				pterm.Printfln(" %s Authenticating with AWS", pterm.LightBlue("▶︎"))
				aws.InitAWSClients(config.App)
			} else {
				spinnerAWSAuth := ux.NewProgressSpinner("Authenticating with AWS")
				aws.InitAWSClients(config.App)
				spinnerAWSAuth.Success(fmt.Sprintf("Authenticated with AWS account %s", aws.GetAccountId()))
			}
			spinnerRouterDetection := ux.NewProgressSpinner("Detecting Atun routers in AWS")
			config.App.Config.RouterHostID, err = tunnel.GetRouterHostIDFromTags()
			if err != nil {
				spinnerRouterDetection.Fail(fmt.Sprintf("No routers found. No --router flag has not been specified and no EC2 instances with atun.io tags found in %s region of AWS account %s.", config.App.Config.AWSRegion, aws.GetAccountId()))
				if detailedStatus {
					ux.RenderDetailedStatus()
				}

				return nil

			}
			spinnerRouterDetection.Success(fmt.Sprintf("Router found: %s", config.App.Config.RouterHostID))
		} else {
			config.App.Config.RouterHostID = routerHostID
		}

		spinnerGetRouterHostConfig := ux.NewProgressSpinner("Getting router endpoints config")
		routerHostConfig, err := tunnel.GetRouterHostConfig(config.App.Config.RouterHostID)
		if err != nil {
			spinnerGetRouterHostConfig.Fail("Error getting router endpoints config", "err", err)
		}
		spinnerGetRouterHostConfig.Success("Router endpoints config retrieved")

		config.App.Version = routerHostConfig.Version
		config.App.Config.Hosts = routerHostConfig.Config.Hosts
		config.App.Config.RouterHostUser = routerHostConfig.Config.RouterHostUser

		spinnerGetSSHTunnelStatus := ux.NewProgressSpinner("Getting SSH tunnel status")
		tunnelActive, endpoints, err := ssh.GetSSHTunnelStatus(config.App)
		if err != nil {
			spinnerGetSSHTunnelStatus.Fail("Failed to get tunnel status", "error", err)
		}
		spinnerGetSSHTunnelStatus.Success("Tunnel status retrieved", "tunnelActive", tunnelActive)

		ux.ClearLines(5)
		//err = tunnel.RenderEndpointsTable(endpoints)
		//if err != nil {
		//	logger.Error("Failed to render endpoints table", "error", err)
		//}

		err = ux.RenderEndpointsTable(endpoints)
		if err != nil {
			logger.Error("Failed to render env table", "error", err)
		}

		config.App.Config.RouterHostID, err = tunnel.GetRouterHostIDFromTags()
		if err != nil {
			logger.Error("Router not found. You might want to create it.", "error", err)
		}
		if detailedStatus {
			ux.RenderDetailedStatus()
		}

		return nil
	},
}

func init() {
	// Show detailed status if log level is debug or info, otherwise hide
	defaultDetailedStatus := false
	if viper.GetString("LOG_LEVEL") == "debug" || viper.GetString("LOG_LEVEL") == "debug" {
		defaultDetailedStatus = true
	}

	statusCmd.PersistentFlags().StringP("router", "r", "", "Router instance id to use. If not specified the first running instance with the atun.io tags is used")
	statusCmd.Flags().BoolP("detailed", "d", defaultDetailedStatus, "Show detailed status")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	//statusCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
