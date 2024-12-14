/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package cmd

import (
	"github.com/automationd/atun/internal/aws"
	"github.com/automationd/atun/internal/config"
	"github.com/automationd/atun/internal/constraints"
	"github.com/automationd/atun/internal/logger"
	"github.com/automationd/atun/internal/ssh"
	"github.com/automationd/atun/internal/tunnel"
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
		// TODO: Implement VPC, subnet picker (1. get list of VPCs, 2. Get a list of Subnets in it, 3. Ask user if it's not provided)
		var err error
		var bastionHost string
		logger.Debug("Up command called", "bastion", bastionHost, "aws profile", config.App.Config.AWSProfile, "env", config.App.Config.Env)

		if err := constraints.CheckConstraints(
			constraints.WithSSMPlugin(),
			constraints.WithAWSProfile(),
			constraints.WithAWSRegion(),
			constraints.WithENV(),
		); err != nil {
			return err
		}

		logger.Debug("All constraints satisfied")

		aws.InitAWSClients(config.App)

		// Get the bastion host ID from the command line
		bastionHost = cmd.Flag("bastion").Value.String()

		// TODO: Add logic if host not found offer create it, add --auto-create-bastion

		// If bastion host is not provided, get the first running instance based on the discovery tag (atun.io/version)
		if bastionHost == "" {
			config.App.Config.BastionHostID, err = tunnel.GetBastionHostID()
			if err != nil {
				logger.Error("Error discovering bastion host. You might need to run `atun create` command first", "error", err)
				os.Exit(1)

				// TODO: suggest creating a bastion host.
				// Use survey to ask if the user wants to create a bastion host
				// If yes, run the create command
				// If no, return

				//createHost := false
				//err = survey.AskOne(&survey.Confirm{
				//	Message: fmt.Sprintf("Would you like to create a bastion host?"),
				//	Default: true,
				//}, &createHost)
				//if err != nil {
				//	logger.Fatal("Error getting confirmation:", err)
				//	return err
				//}
				//
				//if !createHost {
				//	logger.Info("Bastion host creation cancelled. Exiting.")
				//	os.Exit(0)
				//}
				//
				//// Run create command from here
				//createCmd := &cobra.Command{}
				//createCmd.SetArgs([]string{"create"})
				//if err := createCmd.Execute(); err != nil {
				//	logger.Fatal("Error executing create command:", err)
				//	return err
				//}
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

		// Send the public key to the bastion instance
		err = aws.SendSSHPublicKey(config.App.Config.BastionHostID, publicKey, config.App.Config.BastionHostUser)
		if err != nil {
			logger.Fatal("Error adding local SSH Public key to the instance", "SSHPublicKey", publicKey, "BastionHostID", config.App.Config.BastionHostID, "error", err)
			os.Exit(1)
		}

		logger.Debug("Public key sent to bastion host", "bastion", config.App.Config.BastionHostID)

		// TODO: Refactor naming of connectionInfo
		connectionInfo, err := tunnel.StartTunnel(config.App)
		if err != nil {
			logger.Fatal("Error running tunnel", "error", err)
		}

		// TODO: Check if Instance has forwarding working (check ipv4.forwarding sysctl)

		logger.Info("Tunnel is up! Forwarded ports:", "connectionInfo", connectionInfo)

		return nil
	},
}

//func checkTunnel(app *config.Atun) (bool, error) {
//	bastionSocketPath := path.Join(app.Config.AppDir, "bastion.sock")
//
//	// Check if the socket file exists. If it does, check if the tunnel is up
//	if _, err := os.Stat(bastionSocketPath); !os.IsNotExist(err) {
//		logger.Info("A socket file from another tunnel has been found", "path", bastionSocketPath)
//		c := exec.Command(
//			logger.Debug("Checking tunnel in socket", "socket", bastionSocketPath)
//			"ssh", "-S", bastionSocketPath, "-O", "check", "",
//		)
//
//		out := &bytes.Buffer{}
//		c.Stdout = out
//		c.Stderr = out
//		c.Dir = dir
//
//		err := c.Run()
//		if err == nil {
//			sshConfigPath := fmt.Sprintf("%s/ssh.config", dir)
//			sshConfig, err := getSSHConfig(sshConfigPath)
//			if err != nil {
//				return false, fmt.Errorf("can't check tunnel: %w", err)
//			}
//
//			pterm.Success.Println("Tunnel is up. Forwarding config:")
//			hosts := getHosts(sshConfig)
//			var forwardConfig string
//			for _, h := range hosts {
//				forwardConfig += fmt.Sprintf("%s:%s ➡ localhost:%s\n", h[2], h[3], h[1])
//			}
//			pterm.Println(forwardConfig)
//
//			return true, nil
//		} else {
//			pterm.Warning.Println("Tunnel socket file seems to be not useable. We have deleted it")
//			err := os.Remove(bastionSocketPath)
//			if err != nil {
//				return false, err
//			}
//			return false, nil
//		}
//	}
//
//	return false, nil
//}

func init() {
	logger.Debug("Initializing up command")
	upCmd.PersistentFlags().StringP("bastion", "b", "", "Bastion instance id to use. If not specified the first running instance with the atun.io tags is used")
	logger.Debug("Up command initialized")
}
