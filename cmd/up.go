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
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"os"
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

		if err := constraints.CheckConstraints(
			constraints.WithSSMPlugin(),
			constraints.WithAWSProfile(),
			constraints.WithENV(),
		); err != nil {
			return err
		}

		logger.Debug("All constraints satisfied")
		//multiPrinter := pterm.DefaultMultiPrinter
		//multiPrinter.Start()

		ux.Println("Activating SSM Tunnel")

		mfaInputRequired := aws.MFAInputRequired(config.App)
		if mfaInputRequired {
			pterm.Printfln(" %s Authenticating with AWS", pterm.LightBlue("▶︎"))
			aws.InitAWSClients(config.App)
		} else {
			spinnerAWSAuth := ux.NewProgressSpinner("Authenticating with AWS")
			aws.InitAWSClients(config.App)
			spinnerAWSAuth.Success(fmt.Sprintf("Authenticated with AWS account %s", aws.GetAccountId()))
		}

		// Get the router host ID from the command line
		routerHost = cmd.Flag("router").Value.String()

		// If router host is not provided, get the first running instance based on the discovery tag (atun.io/version)
		if routerHost == "" {
			spinnerRouterDetection := ux.NewProgressSpinner("Detecting Atun routers in AWS")

			config.App.Config.RouterHostID, err = tunnel.GetRouterHostIDFromTags()
			if err != nil {
				spinnerRouterDetection.Warning("No EC2 router instances found with atun.io tags.")

				// Get default from the flags
				createHost, _ := cmd.Flags().GetBool("create")

				// If the create flag is not set ask if the user wants to create a router host
				if !createHost {
					if !constraints.IsInteractiveTerminal() {
						err = fmt.Errorf("no --router flag specified and no EC2 instances with atun.io tags found in %s region of AWS account %s", config.App.Config.AWSRegion, aws.GetAccountId())
						spinnerRouterDetection.Fail("No routers found", "error", err)
						return err
					}

					createHost, err = ux.GetConfirmation(fmt.Sprintf("%s %s", "Create a new router host?", pterm.NewStyle(pterm.Italic, pterm.Fuzzy).Sprintf("(It's easy to cleanly delete afterwards)")))
					if err != nil {
						logger.Fatal("Error getting confirmation:", err)
						return err
					}
				}

				if !createHost {
					spinnerRouterDetection.Fail("Router host creation cancelled but it's required. Exiting.")
				}

				// Run create command from here
				err := routerCreateCmd.RunE(routerCreateCmd, args)
				if err != nil {
					return err
				}
				spinnerRouterDetection.UpdateText("Discovering router host...")

				config.App.Config.RouterHostID, err = tunnel.GetRouterHostIDFromTags()
				if err != nil {
					logger.Debug("Error discovering router host", "error", err)
					spinnerRouterDetection.Fail("Error discovering router host")

				}

				// TODO: suggest creating a router host.
				// Use survey to ask if the user wants to create a router host
				// If yes, run the create command
				// If no, return
				spinnerRouterDetection.Success("Routers found", "Discovered router host", config.App.Config.RouterHostID)
			}
			spinnerRouterDetection.Success(fmt.Sprintf("Router found: %s", config.App.Config.RouterHostID))
		} else {
			config.App.Config.RouterHostID = routerHost
		}

		// TODO: refactor as a better functional
		// Read atun:config from the instance as `config`
		routerHostConfig, err := tunnel.GetRouterHostConfig(config.App.Config.RouterHostID)
		if err != nil {
			logger.Fatal("Error getting router endpoints config", "err", err)
		}

		config.App.Version = routerHostConfig.Version
		config.App.Config.Hosts = routerHostConfig.Config.Hosts
		config.App.Config.RouterHostUser = routerHostConfig.Config.RouterHostUser

		for _, host := range config.App.Config.Hosts {
			// Review the hosts
			logger.Debug("Endpoint", "name", host.Name, "proto", host.Proto, "remote", host.Remote, "local", host.Local)
		}

		sshConfigSpinner := ux.NewProgressSpinner("Generating SSH Config")

		// Generate SSH config file
		config.App.Config.SSHConfigFile, err = ssh.GenerateSSHConfigFile(config.App)
		if err != nil {
			sshConfigSpinner.Fail("Error generating SSH config file", "SSHConfigFile", config.App.Config.SSHConfigFile, "error", err)
		}

		sshConfigSpinner.Success("SSH Config generated", "path", config.App.Config.SSHConfigFile)

		logger.Debug("Private key path", "path", config.App.Config.SSHKeyPath)

		//err := o.checkOsVersion()
		//if err != nil {
		//	return err
		//}

		// Try to start a tunnel before writing the SSH key (to save on time spent on SSM)

		activateTunnelSpinner := ux.NewProgressSpinner("Activating Tunnel")
		tunnelActive, connections, err := tunnel.ActivateTunnel(config.App)
		if err != nil {
			activateTunnelSpinner.UpdateText("SSH key doesn't seem to be present on the router host")

			// Read private key from HOME/id_rsa.pub
			publicKey, err := ssh.GetPublicKey(config.App.Config.SSHKeyPath)
			if err != nil {
				logger.Error("Error getting public key", "error", err)
			}
			logger.Debug("Public key", "key", publicKey)

			activateTunnelSpinner.UpdateText("Ensuring local SSH key is authorized on router...", "SSHPublicKeyPath", config.App.Config.SSHKeyPath, "RouterHostID", config.App.Config.RouterHostID)

			// Send the public key to the router instance
			err = aws.EnsureSSHPublicKeyPresent(config.App.Config.RouterHostID, publicKey, config.App.Config.RouterHostUser)
			if err != nil {
				activateTunnelSpinner.Fail("Failed to add local SSH Public key to the instance", "SSHPublicKey", publicKey, "RouterHostID", config.App.Config.RouterHostID, "error", err)
				os.Exit(1)
			}

			activateTunnelSpinner.UpdateText(fmt.Sprintf("Public key added to router host ~/.ssh/authorized_keys on %s", config.App.Config.RouterHostID))
			activateTunnelSpinner.UpdateText("SSH key authorized")

			// Retry starting the tunnel after the key is added
			tunnelActive, connections, err = tunnel.ActivateTunnel(config.App)
			if err != nil {
				activateTunnelSpinner.Fail(fmt.Sprintf("Error activating tunnel: %s", err))
				os.Exit(1)
			}
		}

		activateAttemptTunnelSpinner := ux.NewProgressSpinner("Activating Tunnel")
		activateAttemptTunnelSpinner.Success("Tunnel is active")

		// Clear the screen
		ux.ClearLines(5)

		activateAttemptTunnelSpinner.Status("Tunnel", tunnelActive, connections)
		// TODO: Check if Instance has forwarding working (check ipv4.forwarding sysctl)
		//ux.Println("Tunnel is active")

		return nil
	},
}

func init() {
	logger.Debug("Initializing up command")
	upCmd.PersistentFlags().StringP("router", "r", "", "Router instance id to use. If not specified the first running instance with the atun.io tags is used")
	upCmd.PersistentFlags().BoolP("create", "c", false, "Create ad-hoc router (if it doesn't exist). Will be managed by built-in CDKTf")
	logger.Debug("Up command initialized")
}
