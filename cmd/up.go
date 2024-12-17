/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package cmd

import (
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"github.com/automationd/atun/internal/aws"
	"github.com/automationd/atun/internal/config"
	"github.com/automationd/atun/internal/constraints"
	"github.com/automationd/atun/internal/logger"
	"github.com/automationd/atun/internal/ssh"
	"github.com/automationd/atun/internal/tunnel"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"os"
)

// upCmd represents the up command
var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Starts a tunnel to the bastion host",
	Long: `Starts a tunnel to the bastion host and forwards ports to the local machine.

	If the bastion host is not provided, the first running instance with the atun.io/version tag is used.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Use GO Method received on `atun`

		var err error
		var bastionHost string
		logger.Debug("Up command called", "bastion", bastionHost, "aws profile", config.App.Config.AWSProfile, "env", config.App.Config.Env)

		if err := constraints.CheckConstraints(
			constraints.WithSSMPlugin(),
			constraints.WithAWSProfile(),
			//constraints.WithAWSRegion(), // Can be derived on the session level
			constraints.WithENV(),
		); err != nil {
			return err
		}

		logger.Debug("All constraints satisfied")
		var upTunnelSpinner *pterm.SpinnerPrinter
		showSpinner := config.App.Config.LogLevel != "debug" && config.App.Config.LogLevel != "info"

		if showSpinner {
			upTunnelSpinner = logger.StartCustomSpinner(
				fmt.Sprintf("Starting tunnel via bastion host %s...", config.App.Config.BastionHostID),
			)
		} else {
			logger.Debug("Not showing spinner", "logLevel", config.App.Config.LogLevel)
			logger.Info("Starting tunnel via EC2 Bastion Instance...")
		}

		aws.InitAWSClients(config.App)

		// Get the bastion host ID from the command line
		bastionHost = cmd.Flag("bastion").Value.String()

		// TODO: Add logic if host not found offer create it, add --auto-create-bastion

		// If bastion host is not provided, get the first running instance based on the discovery tag (atun.io/version)
		if bastionHost == "" {
			config.App.Config.BastionHostID, err = tunnel.GetBastionHostID()
			if err != nil {
				if showSpinner {
					upTunnelSpinner.Warning("No Bastion hosts found with atun.io tags.")

				} else {
					logger.Warn("No Bastion hosts found with atun.io tags.", "error", err)
				}

				// Get default from the flags
				createHost, _ := cmd.Flags().GetBool("create")

				// If the create flag is not set ask if the user wants to create a bastion host
				if !createHost {
					err = survey.AskOne(&survey.Confirm{
						Message: fmt.Sprintf("Would you like to create an ad-hoc bastion host? (It's easy to cleanly delete)"),
						Default: true,
					}, &createHost)
					if err != nil {
						logger.Fatal("Error getting confirmation:", err)
						return err
					}
				}

				if !createHost {
					logger.Info("Bastion host creation cancelled. Exiting.")
					os.Exit(1)
				}

				// Run create command from here
				err := createCmd.RunE(createCmd, args)
				if err != nil {
					return fmt.Errorf("error running createCmd: %w", err)
				}

				logger.Debug("Created host")

				config.App.Config.BastionHostID, err = tunnel.GetBastionHostID()
				if err != nil {
					logger.Fatal("Error discovering bastion host", "error", err)
				}
				logger.Debug("Bastion host ID", "bastion", config.App.Config.BastionHostID)
				upTunnelSpinner = logger.StartCustomSpinner(fmt.Sprintf("Starting tunnel via bastion host %s...", config.App.Config.BastionHostID))

				// TODO: suggest creating a bastion host.
				// Use survey to ask if the user wants to create a bastion host
				// If yes, run the create command
				// If no, return

			}
		} else {
			config.App.Config.BastionHostID = bastionHost
		}

		logger.Debug("Bastion host ID", "bastion", config.App.Config.BastionHostID)

		// TODO: refactor as a better functional
		// Read atun:config from the instance as `config`

		bastionHostConfig, err := tunnel.GetBastionHostConfig(config.App.Config.BastionHostID)
		if err != nil {
			logger.Fatal("Error getting bastion host config", "err", err)
		}

		config.App.Version = bastionHostConfig.Version
		config.App.Config.Hosts = bastionHostConfig.Config.Hosts

		//config.App.Config = atun.Config
		//config.App.Hosts = atun.Hosts

		for _, host := range config.App.Config.Hosts {
			// Review the hosts
			logger.Debug("Host", "name", host.Name, "proto", host.Proto, "remote", host.Remote, "local", host.Local)
		}

		// Generate SSH config file
		config.App.Config.SSHConfigFile, err = ssh.GenerateSSHConfigFile(config.App)
		//atun.Config.SSHConfigFile, err = generateSSHConfigFile(atun)
		if err != nil {
			logger.Error("Error generating SSH config file", "SSHConfigFile", config.App.Config.SSHConfigFile, "err", err)
		}
		//if err != nil {
		//	pterm.Error.Printfln("Error writing SSH config to %s: %v", atun.Config.SSHConfigFile, err)
		//}

		logger.Debug("Saved SSH config file", "path", config.App.Config.SSHConfigFile)

		// TODO: Check & Install SSM Agent

		//logrus.Debugf("public key path: %s", publicKeyPath)
		logger.Debug("Private key path", "path", config.App.Config.SSHKeyPath)

		//err := o.checkOsVersion()
		//if err != nil {
		//	return err
		//}

		// Read private key from HOME/id_rsa.pub
		publicKey, err := ssh.GetPublicKey(config.App.Config.SSHKeyPath)
		if err != nil {
			logger.Error("Error getting public key", "error", err)
		}

		logger.Debug("Public key", "key", publicKey)

		if showSpinner {
			upTunnelSpinner.UpdateText("Adding local SSH Public key to the instance...")
		} else {
			logger.Debug("Adding local SSH Public key to the instance")
		}

		// TODO First try to auth with the current key and only then
		// Send the public key to the bastion instance
		err = aws.SendSSHPublicKey(config.App.Config.BastionHostID, publicKey, config.App.Config.BastionHostUser)
		if err != nil {
			if showSpinner {
				upTunnelSpinner.Fail("Failed to add local SSH Public key to the instance")
			} else {
				logger.Fatal("Error adding local SSH Public key to the instance", "SSHPublicKey", publicKey, "BastionHostID", config.App.Config.BastionHostID, "error", err)
				os.Exit(1)
			}

		}

		if showSpinner {
			upTunnelSpinner.UpdateText(fmt.Sprintf("Public key sent to bastion host %s", config.App.Config.BastionHostID))
		} else {
			logger.Debug("Public key sent to bastion host", "bastion", config.App.Config.BastionHostID)
		}

		tunnelIsUp, connections, err := tunnel.StartTunnel(config.App)
		if err != nil {
			if showSpinner {
				upTunnelSpinner.Fail(fmt.Sprintf("Error starting tunnel %s", err))
			} else {
				logger.Fatal("Error running tunnel", "error", err)
			}
		}

		// TODO: Check if Instance has forwarding working (check ipv4.forwarding sysctl)

		if showSpinner {
			upTunnelSpinner.Success("Tunnel is up!")

			err = tunnel.RenderTunnelStatusTable(tunnelIsUp, connections)
			if err != nil {
				logger.Error("Error rendering connections table", "error", err)
			}
			// Print pterm table with connection info
			// Create a new table pterm.DefaultTable.WithHasHeader() with connections

		} else {
			logger.Info("Tunnel is up! Forwarded ports:", "connections", connections)
		}

		return nil
	},
}

func init() {
	logger.Debug("Initializing up command")
	upCmd.PersistentFlags().StringP("bastion", "b", "", "Bastion instance id to use. If not specified the first running instance with the atun.io tags is used")
	upCmd.PersistentFlags().BoolP("create", "c", false, "Create ad-hoc bastion (if it doesn't exist). Will be managed by built-in CDKTf")
	logger.Debug("Up command initialized")
}
