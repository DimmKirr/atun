/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package cmd

import (
	"fmt"
	"github.com/DimmKirr/atun/internal/aws"
	"github.com/DimmKirr/atun/internal/config"
	"github.com/DimmKirr/atun/internal/constraints"
	"github.com/DimmKirr/atun/internal/logger"
	"github.com/DimmKirr/atun/internal/ssh"
	"github.com/DimmKirr/atun/internal/tunnel"
	"github.com/DimmKirr/atun/internal/ux"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"time"
)

// downCmd represents the down command
var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Bring the tunnel down",
	Long:  `Bring the existing tunnel down.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger.Debug("Down command called")
		var (
			err          error
			routerHostID string
		)

		// Check Constraints
		//if err := constraints.CheckConstraints(
		//	constraints.WithRouterHostID(),
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

		ux.Println("Deactivating Tunnel")

		routerHostID = cmd.Flag("router").Value.String()

		// If router ID is not provided via a flag
		if routerHostID == "" {

			spinnerGetRouterHostFromExistingSession := ux.NewProgressSpinner("Checking existing / previous sessions")
			routerHostID, err = ssh.GetRouterHostIDFromExistingSession(config.App.Config.TunnelDir)
			if err != nil {
				spinnerGetRouterHostFromExistingSession.UpdateText("Couldn't get router host ID locally. Trying with AWS", "error", err)
			}
			spinnerGetRouterHostFromExistingSession.Success("Tunnel config doesn't exist locally")

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
				spinnerRouterDetection.Warning("No router hosts found with atun.io tags.")

				spinnerRouterDetection.UpdateText("Discovering router host...")
				config.App.Config.RouterHostID, err = tunnel.GetRouterHostIDFromTags()
				if err != nil {
					spinnerRouterDetection.Fail("Error discovering router host", "error", err)
				}

				spinnerRouterDetection.Success("Routers found", "Discovered router host", config.App.Config.RouterHostID)
				// TODO: suggest creating a router host.
				// Use survey to ask if the user wants to create a router host
				// If yes, run the create command
				// If no, return

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
		spinnerGetRouterHostConfig.Success(fmt.Sprintf("Router endpoints config retrieved with %v endpoints", len(routerHostConfig.Config.Hosts)), "hosts", routerHostConfig.Config.Hosts)

		config.App.Version = routerHostConfig.Version
		config.App.Config.Hosts = routerHostConfig.Config.Hosts
		config.App.Config.RouterHostUser = routerHostConfig.Config.RouterHostUser

		spinnerGetSSHTunnelStatus := ux.NewProgressSpinner("Getting SSH tunnel status")
		tunnelActive, endpoints, err := ssh.GetSSHTunnelStatus(config.App)
		if err != nil {
			spinnerGetSSHTunnelStatus.Fail("Failed to get tunnel status", "error", err)
		}
		spinnerGetSSHTunnelStatus.Success(fmt.Sprintf("Tunnel status retrieved: %s", map[bool]string{true: "active", false: "inactive"}[tunnelActive]))

		if tunnelActive {
			spinnerDeactivateTunnel := ux.NewProgressSpinner("Deactivating tunnel")
			spinnerDeactivateTunnel.UpdateText("Tunnel is active", "tunnelActive", tunnelActive, "routerHostID", config.App.Config.RouterHostID)

			spinnerDeactivateTunnel.UpdateText("Deactivating tunnel")
			tunnelActive, err = tunnel.DeactivateTunnel(config.App)
			if err != nil {
				spinnerDeactivateTunnel.Fail("Failed to deactivate tunnel", "error", err)
			}

		}

		// Check tunnel for the second time
		spinnerGetSSHTunnelStatusFinal := ux.NewProgressSpinner("Checking tunnel status")
		tunnelActive, endpoints, err = ssh.GetSSHTunnelStatus(config.App)
		if !tunnelActive {
			spinnerGetSSHTunnelStatusFinal.Success("Tunnel inactive")
		}

		// Get delete flag
		deleteRouter, _ := cmd.Flags().GetBool("delete")

		if deleteRouter {
			spinnerDeleteRouter := ux.NewProgressSpinner("Deleting router")
			spinnerDeleteRouter.UpdateText("Delete flag is set. Deleting router host", "routerHostID", config.App.Config.RouterHostID)

			// Run create command from here
			err := routerDeleteCmd.RunE(routerDeleteCmd, args)
			if err != nil {
				spinnerDeleteRouter.Fail("Failed deleting the router", "err", err)
			}
		}

		time.Sleep(1000 * time.Millisecond)

		ux.ClearLines(7)
		err = ux.RenderEndpointsTable(endpoints)
		if err != nil {
			logger.Error("Failed to render env table", "error", err)
		}

		return nil
	},
}

func init() {
	logger.Debug("Initializing up command")
	downCmd.PersistentFlags().StringP("router", "r", "", "Router instance id to use. If not specified the first running instance with the atun.io tags is used")
	downCmd.PersistentFlags().BoolP("delete", "x", false, "Delete ad-hoc router (if exists). Won't delete any resources non-managed by atun")
}
