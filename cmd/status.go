/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package cmd

import (
	"encoding/json"
	"fmt"
	"os"

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
		// Check if we're in JSON mode
		jsonOutput, _ := cmd.Root().PersistentFlags().GetBool("json")

		if routerHostID == "" {
			mfaInputRequired := aws.MFAInputRequired(config.App)

			if mfaInputRequired {
				if !jsonOutput {
					pterm.Printfln(" %s Authenticating with AWS", pterm.LightBlue("▶︎"))
				}
				aws.InitAWSClients(config.App)
			} else {
				var spinnerAWSAuth *ux.ProgressSpinner
				if !jsonOutput {
					spinnerAWSAuth = ux.NewProgressSpinner("Authenticating with AWS")
				}
				aws.InitAWSClients(config.App)
				if !jsonOutput {
					spinnerAWSAuth.Success(fmt.Sprintf("Authenticated with AWS account %s", aws.GetAccountId()))
				}
			}

			var spinnerRouterDetection *ux.ProgressSpinner
			if !jsonOutput {
				spinnerRouterDetection = ux.NewProgressSpinner("Detecting Atun routers in AWS")
			}

			config.App.Config.RouterHostID, err = tunnel.GetRouterHostIDFromTags()
			if err != nil {
				if !jsonOutput {
					spinnerRouterDetection.Fail(fmt.Sprintf("No routers found. No --router flag has not been specified and no EC2 instances with atun.io tags found in %s region of AWS account %s.", config.App.Config.AWSRegion, aws.GetAccountId()))
					if detailedStatus {
						ux.RenderDetailedStatus()
					}
				}
				return nil
			}

			if !jsonOutput {
				spinnerRouterDetection.Success(fmt.Sprintf("Router found: %s", config.App.Config.RouterHostID))
			}
		} else {
			config.App.Config.RouterHostID = routerHostID
		}

		var spinnerGetRouterHostConfig *ux.ProgressSpinner
		if !jsonOutput {
			spinnerGetRouterHostConfig = ux.NewProgressSpinner("Getting router endpoints config")
		}

		routerHostConfig, err := tunnel.GetRouterHostConfig(config.App.Config.RouterHostID)
		if err != nil {
			if !jsonOutput {
				spinnerGetRouterHostConfig.Fail("Error getting router endpoints config", "err", err)
			}
		} else if !jsonOutput {
			spinnerGetRouterHostConfig.Success("Router endpoints config retrieved")
		}

		config.App.Version = routerHostConfig.Version
		config.App.Config.Hosts = routerHostConfig.Config.Hosts
		config.App.Config.RouterHostUser = routerHostConfig.Config.RouterHostUser

		var spinnerGetSSHTunnelStatus *ux.ProgressSpinner
		if !jsonOutput {
			spinnerGetSSHTunnelStatus = ux.NewProgressSpinner("Getting SSH tunnel status")
		}

		tunnelActive, endpoints, err := ssh.GetSSHTunnelStatus(config.App)
		if err != nil && !jsonOutput {
			spinnerGetSSHTunnelStatus.Fail("Failed to get tunnel status", "error", err)
		} else if !jsonOutput {
			spinnerGetSSHTunnelStatus.Success("Tunnel status retrieved", "tunnelActive", tunnelActive)
		}

		// Get the global --json flag
		jsonOutput, _ = cmd.Root().PersistentFlags().GetBool("json")

		if jsonOutput {
			// Prepare data for JSON output
			statusData := struct {
				RouterHostID string            `json:"router_host_id"`
				TunnelActive bool              `json:"tunnel_active"`
				Endpoints    []ssh.Endpoint    `json:"endpoints"`
				AWSAccount   string            `json:"aws_account"`
				AWSRegion    string            `json:"aws_region"`
				Environment  string            `json:"environment"`
				Version      string            `json:"version"`
				Detailed     map[string]string `json:"detailed,omitempty"`
			}{
				RouterHostID: config.App.Config.RouterHostID,
				TunnelActive: tunnelActive,
				Endpoints:    endpoints,
				AWSAccount:   aws.GetAccountId(),
				AWSRegion:    config.App.Config.AWSRegion,
				Environment:  config.App.Config.Env,
				Version:      config.App.Version,
			}

			if detailedStatus {
				// Add detailed info if requested
				details := make(map[string]string)
				cwd, _ := os.Getwd()
				details["pwd"] = cwd
				details["ssh_key_path"] = config.App.Config.SSHKeyPath
				details["config_file"] = config.App.Config.ConfigFile
				details["router_endpoint_user"] = config.App.Config.RouterHostUser
				details["socket_path"] = ssh.GetRouterSockFilePath(config.App)
				details["ssh_config_file"] = ssh.GetSSHConfigFilePath(config.App)
				statusData.Detailed = details
			}

			// Output JSON
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetEscapeHTML(false)
			encoder.SetIndent("", "  ")
			if err := encoder.Encode(statusData); err != nil {
				return fmt.Errorf("failed to encode status to JSON: %w", err)
			}
		} else {
			// Original table output
			ux.ClearLines(5)
			err = ux.RenderEndpointsTable(endpoints)
			if err != nil {
				logger.Error("Failed to render env table", "error", err)
			}

			if detailedStatus {
				ux.RenderDetailedStatus()
			}
		}

		// Get the router host ID for the final status check
		config.App.Config.RouterHostID, err = tunnel.GetRouterHostIDFromTags()
		if err != nil {
			logger.Error("Router not found. You might want to create it.", "error", err)
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
