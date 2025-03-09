/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package cmd

import (
	"fmt"
	"github.com/automationd/atun/internal/ux"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/automationd/atun/internal/aws"
	"github.com/automationd/atun/internal/config"
	"github.com/automationd/atun/internal/constraints"
	"github.com/automationd/atun/internal/logger"
	"github.com/automationd/atun/internal/ssh"
	"github.com/automationd/atun/internal/tunnel"
	"github.com/spf13/cobra"
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
			constraints.WithENV(),
		); err != nil {
			return err
		}

		logger.Debug("All constraints satisfied")

		upTunnelSpinner := ux.NewProgressSpinner("Activating SSM tunnel")

		upTunnelSpinner.UpdateText("Authenticating with AWS...")
		aws.InitAWSClients(config.App)

		// Get the bastion host ID from the command line
		bastionHost = cmd.Flag("bastion").Value.String()

		// If bastion host is not provided, get the first running instance based on the discovery tag (atun.io/version)
		if bastionHost == "" {
			upTunnelSpinner.UpdateText("Discovering bastion host...")

			config.App.Config.BastionHostID, err = tunnel.GetBastionHostIDFromTags()
			if err != nil {
				upTunnelSpinner.Warning("No bastion hosts found with atun.io tags.")
				//if showSpinner {
				//	upTunnelSpinner.Warning("No bastion hosts found with atun.io tags.")
				//} else {
				//	logger.Warn("No bastion hosts found with atun.io tags.", "error", err)
				//}

				// Get default from the flags
				createHost, _ := cmd.Flags().GetBool("create")

				// If the create flag is not set ask if the user wants to create a bastion host
				if !createHost {
					if !constraints.IsInteractiveTerminal() {
						upTunnelSpinner.Fail("No bastion host found and not running in an interactive terminal. Exiting.")
						os.Exit(1)
					}

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
					upTunnelSpinner.UpdateText("Bastion host creation cancelled. Exiting.")
					os.Exit(1)
				}

				// Run create command from here
				err := createCmd.RunE(createCmd, args)
				if err != nil {
					return fmt.Errorf("error running createCmd: %w", err)
				}
				upTunnelSpinner.UpdateText("Discovering bastion host...")

				config.App.Config.BastionHostID, err = tunnel.GetBastionHostIDFromTags()
				if err != nil {
					logger.Debug("Error discovering bastion host", "error", err)
					upTunnelSpinner.Fail("Error discovering bastion host")

				}
				upTunnelSpinner.Success("Discovered bastion host", config.App.Config.BastionHostID)

				upTunnelSpinner.UpdateText("Activating tunnel via bastion host", "BastionHostID", config.App.Config.BastionHostID)

				// TODO: suggest creating a bastion host.
				// Use survey to ask if the user wants to create a bastion host
				// If yes, run the create command
				// If no, return

			}
		} else {
			config.App.Config.BastionHostID = bastionHost
		}
		upTunnelSpinner.UpdateText("Discovered bastion host", "ID", config.App.Config.BastionHostID)

		// TODO: refactor as a better functional
		// Read atun:config from the instance as `config`
		bastionHostConfig, err := tunnel.GetBastionHostConfig(config.App.Config.BastionHostID)
		if err != nil {
			logger.Fatal("Error getting bastion host config", "err", err)
		}

		config.App.Version = bastionHostConfig.Version
		config.App.Config.Hosts = bastionHostConfig.Config.Hosts
		config.App.Config.BastionHostUser = bastionHostConfig.Config.BastionHostUser

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

		// Try to start a tunnel before writing the SSH key (to save on time spent on SSM)
		tunnelIsUp, connections, err := tunnel.StartTunnel(config.App)
		if err != nil {
			upTunnelSpinner.UpdateText("SSH key doesn't seem to be present on the bastion host")

			// Read private key from HOME/id_rsa.pub
			publicKey, err := ssh.GetPublicKey(config.App.Config.SSHKeyPath)
			if err != nil {
				logger.Error("Error getting public key", "error", err)
			}
			logger.Debug("Public key", "key", publicKey)

			upTunnelSpinner.UpdateText("Ensuring local SSH key is authorized on bastion...", "SSHPublicKeyPath", config.App.Config.SSHKeyPath, "BastionHostID", config.App.Config.BastionHostID)

			// Send the public key to the bastion instance
			err = aws.EnsureSSHPublicKeyPresent(config.App.Config.BastionHostID, publicKey, config.App.Config.BastionHostUser)
			if err != nil {
				upTunnelSpinner.Fail("Failed to add local SSH Public key to the instance", "SSHPublicKey", publicKey, "BastionHostID", config.App.Config.BastionHostID, "error", err)
				os.Exit(1)
			}

			upTunnelSpinner.UpdateText(fmt.Sprintf("Public key added to bastion host ~/.ssh/authorized_keys on %s", config.App.Config.BastionHostID))

			// Retry starting the tunnel after the key is added
			tunnelIsUp, connections, err = tunnel.StartTunnel(config.App)
			if err != nil {
				upTunnelSpinner.Fail(fmt.Sprintf("Error activating tunnel %s", err))
				os.Exit(1)
			}

		}

		// TODO: Check if Instance has forwarding working (check ipv4.forwarding sysctl)
		upTunnelSpinner.Status("Tunnel is active", tunnelIsUp, connections)

		return nil
	},
}

func init() {
	logger.Debug("Initializing up command")
	upCmd.PersistentFlags().StringP("bastion", "b", "", "Bastion instance id to use. If not specified the first running instance with the atun.io tags is used")
	upCmd.PersistentFlags().BoolP("create", "c", false, "Create ad-hoc bastion (if it doesn't exist). Will be managed by built-in CDKTf")
	logger.Debug("Up command initialized")
}
