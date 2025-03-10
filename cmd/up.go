/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package cmd

import (
	"fmt"
	"os"

	"github.com/automationd/atun/internal/ux"

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
	Short: "Starts a tunnel to the router host",
	Long: `Starts a tunnel to the router host and forwards ports to the local machine.

	If the router host is not provided, the first running instance with the atun.io/version tag is used.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Use GO Method received on `atun`

		var err error
		var routerHost string
		logger.Debug("Up command called", "router", routerHost, "aws profile", config.App.Config.AWSProfile, "env", config.App.Config.Env)

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

		// Get the router host ID from the command line
		routerHost = cmd.Flag("router").Value.String()

		// If router host is not provided, get the first running instance based on the discovery tag (atun.io/version)
		if routerHost == "" {
			upTunnelSpinner.UpdateText("Discovering router host...")

			config.App.Config.RouterHostID, err = tunnel.GetRouterHostIDFromTags()
			if err != nil {
				upTunnelSpinner.Warning("No router hosts found with atun.io tags.")
				//if showSpinner {
				//	upTunnelSpinner.Warning("No router hosts found with atun.io tags.")
				//} else {
				//	logger.Warn("No router hosts found with atun.io tags.", "error", err)
				//}

				// Get default from the flags
				createHost, _ := cmd.Flags().GetBool("create")

				// If the create flag is not set ask if the user wants to create a router host
				if !createHost {
					if !constraints.IsInteractiveTerminal() {
						upTunnelSpinner.Fail("No router host found and not running in an interactive terminal. Exiting.")
						os.Exit(1)
					}

					err = survey.AskOne(&survey.Confirm{
						Message: fmt.Sprintf("Would you like to create an ad-hoc router host? (It's easy to cleanly delete)"),
						Default: true,
					}, &createHost)
					if err != nil {
						logger.Fatal("Error getting confirmation:", err)
						return err
					}
				}

				if !createHost {
					upTunnelSpinner.UpdateText("Router host creation cancelled. Exiting.")
					os.Exit(1)
				}

				// Run create command from here
				err := routerCreateCmd.RunE(routerCreateCmd, args)
				if err != nil {
					return fmt.Errorf("error running routerCreateCmd: %w", err)
				}
				upTunnelSpinner.UpdateText("Discovering router host...")

				config.App.Config.RouterHostID, err = tunnel.GetRouterHostIDFromTags()
				if err != nil {
					logger.Debug("Error discovering router host", "error", err)
					upTunnelSpinner.Fail("Error discovering router host")

				}
				upTunnelSpinner.Success("Discovered router host", config.App.Config.RouterHostID)

				upTunnelSpinner.UpdateText("Activating tunnel via router host", "RouterHostID", config.App.Config.RouterHostID)

				// TODO: suggest creating a router host.
				// Use survey to ask if the user wants to create a router host
				// If yes, run the create command
				// If no, return

			}
		} else {
			config.App.Config.RouterHostID = routerHost
		}
		upTunnelSpinner.UpdateText("Discovered router host", "ID", config.App.Config.RouterHostID)

		// TODO: refactor as a better functional
		// Read atun:config from the instance as `config`
		routerHostConfig, err := tunnel.GetRouterHostConfig(config.App.Config.RouterHostID)
		if err != nil {
			logger.Fatal("Error getting router host config", "err", err)
		}

		config.App.Version = routerHostConfig.Version
		config.App.Config.Hosts = routerHostConfig.Config.Hosts
		config.App.Config.RouterHostUser = routerHostConfig.Config.RouterHostUser

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
			upTunnelSpinner.UpdateText("SSH key doesn't seem to be present on the router host")

			// Read private key from HOME/id_rsa.pub
			publicKey, err := ssh.GetPublicKey(config.App.Config.SSHKeyPath)
			if err != nil {
				logger.Error("Error getting public key", "error", err)
			}
			logger.Debug("Public key", "key", publicKey)

			upTunnelSpinner.UpdateText("Ensuring local SSH key is authorized on router...", "SSHPublicKeyPath", config.App.Config.SSHKeyPath, "RouterHostID", config.App.Config.RouterHostID)

			// Send the public key to the router instance
			err = aws.EnsureSSHPublicKeyPresent(config.App.Config.RouterHostID, publicKey, config.App.Config.RouterHostUser)
			if err != nil {
				upTunnelSpinner.Fail("Failed to add local SSH Public key to the instance", "SSHPublicKey", publicKey, "RouterHostID", config.App.Config.RouterHostID, "error", err)
				os.Exit(1)
			}

			upTunnelSpinner.UpdateText(fmt.Sprintf("Public key added to router host ~/.ssh/authorized_keys on %s", config.App.Config.RouterHostID))

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
	upCmd.PersistentFlags().StringP("router", "b", "", "Router instance id to use. If not specified the first running instance with the atun.io tags is used")
	upCmd.PersistentFlags().BoolP("create", "c", false, "Create ad-hoc router (if it doesn't exist). Will be managed by built-in CDKTf")
	logger.Debug("Up command initialized")
}
