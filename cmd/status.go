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
	"github.com/spf13/viper"

	"github.com/pterm/pterm"
	"os"

	"github.com/spf13/cobra"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of the tunnel and current environment",
	Long: `Show status of the tunnel and current environment.
	This is also useful for troubleshooting`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var routerHost string

		logger.Debug("Status command called")
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("can't load options for a command: %w", err)
		}

		dt := pterm.DefaultTable

		aws.InitAWSClients(config.App)
		// Get the router host ID from the command line
		routerHost = cmd.Flag("router").Value.String()

		var upTunnelSpinner *pterm.SpinnerPrinter
		showSpinner := config.App.Config.LogLevel != "debug" && config.App.Config.LogLevel != "info" && constraints.IsInteractiveTerminal() && constraints.SupportsANSIEscapeCodes()

		// If router host is not provided, get the first running instance based on the discovery tag (atun.io/version)
		if routerHost == "" {
			config.App.Config.RouterHostID, err = tunnel.GetRouterHostIDFromTags()
			if err != nil {
				if showSpinner {
					upTunnelSpinner.Warning("No Router hosts found with atun.io tags.")

				} else {
					logger.Warn("No Router hosts found with atun.io tags.", "error", err)
				}

				config.App.Config.RouterHostID, err = tunnel.GetRouterHostIDFromTags()
				if err != nil {
					logger.Fatal("Error discovering router host", "error", err)
				}
				logger.Debug("Router host ID", "router", config.App.Config.RouterHostID)
				upTunnelSpinner = ux.StartCustomSpinner(fmt.Sprintf("Starting tunnel via router host %s...", config.App.Config.RouterHostID))

				// TODO: suggest creating a router host.
				// Use survey to ask if the user wants to create a router host
				// If yes, run the create command
				// If no, return

			}
		} else {
			config.App.Config.RouterHostID = routerHost
		}

		routerHostConfig, err := tunnel.GetRouterHostConfig(config.App.Config.RouterHostID)
		if err != nil {
			logger.Fatal("Error getting router host config", "err", err)
		}

		config.App.Version = routerHostConfig.Version
		config.App.Config.Hosts = routerHostConfig.Config.Hosts
		config.App.Config.RouterHostUser = routerHostConfig.Config.RouterHostUser

		tunnelIsUp, connections, err := ssh.GetSSHTunnelStatus(config.App)

		err = tunnel.RenderTunnelStatusTable(tunnelIsUp, connections)
		if err != nil {
			logger.Error("Failed to render connections table", "error", err)
		}

		config.App.Config.RouterHostID, err = tunnel.GetRouterHostIDFromTags()
		if err != nil {
			logger.Error("Router host not found. You might want to create it.", "error", err)
		}

		detailedStatus, err := cmd.Flags().GetBool("detailed")
		if err != nil {
			return fmt.Errorf("can't get detailed flag: %w", err)
		}

		if !detailedStatus {
			os.Exit(0)
		}

		logger.Debug("Getting detailed status")

		// TODO: Hide this info behind --debug flag or move to a `debug` command
		pterm.DefaultSection.Println("App Debug Info")
		_ = dt.WithData(pterm.TableData{
			{"AWS_ACCOUNT", aws.GetAccountId()},
			{"AWS_PROFILE", config.App.Config.AWSProfile},
			{"AWS_REGION", config.App.Config.AWSRegion},
			{"PWD", cwd},
			{"SSH_KEY_PATH", config.App.Config.SSHKeyPath},
			{"Config File", config.App.Config.ConfigFile},
			{"Router Host", config.App.Config.RouterHostID},
			{"Router Host User", config.App.Config.RouterHostUser},
			{"Socket Path", ssh.GetRouterSockFilePath(config.App)},
			{"SSH Config File", ssh.GetSSHConfigFilePath(config.App)},
			{"Log Level", config.App.Config.LogLevel},

			//{"Toggle", toggleValue},
		}).WithLeftAlignment().Render()
		logger.Debug("Status command finished")
		return nil
	},
}

func init() {
	// Show detailed status if log level is debug or info, otherwise hide
	defaultDetailedStatus := false
	if viper.GetString("LOG_LEVEL") == "debug" || viper.GetString("LOG_LEVEL") == "debug" {
		defaultDetailedStatus = true
	}

	// Here you will define your flags and configuration settings.d
	statusCmd.Flags().BoolP("detailed", "d", defaultDetailedStatus, "Show detailed status")
	statusCmd.PersistentFlags().StringP("router", "b", "", "Router instance id to use. If not specified the first running instance with the atun.io tags is used")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	//statusCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
