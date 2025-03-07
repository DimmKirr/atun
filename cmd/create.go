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
	"github.com/automationd/atun/internal/infra"
	"github.com/automationd/atun/internal/logger"
	"github.com/automationd/atun/internal/tunnel"
	"github.com/automationd/atun/internal/ux"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"log"
	"os"
	"strconv"
)

// createCmd represents the add command
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Creates an ad-hoc bastion host to a specified subnet",
	Long: `Creates ad-hoc bastion host to a specified subnet. Performed via CDKTF/Terraform 
	This is useful when there is no IaC in place and there is a need to connect to a resource private.
	State is saved locally and it's advised to delete it after the task is finished.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error

		aws.InitAWSClients(config.App)

		// Check if the configuration is loaded
		if config.App.Config != nil {
			logger.Debug("App config exists (via file, env vars or else). Checking Hosts Config")

			// If config is not loaded, offer to create a new configuration using survey package
			if len(config.App.Config.Hosts) == 0 {
				logger.Warn(
					"No hosts found in the configuration.")

				err = buildHostConfig(config.App)
				if err != nil {
					logger.Error("Error creating host config: %v", err)
				}

				// Use Survey to ask if the user wants to save the configuration
				saveConfig := true
				err = survey.AskOne(&survey.Confirm{
					Message: "Would you like to save the configuration?",
					Default: saveConfig,
				}, &saveConfig)
				if err != nil {
					logger.Fatal("Error getting confirmation:", "err", err)
				}

				if saveConfig {
					// Save the configuration to the file
					err = config.SaveConfig()
					if err != nil {
						logger.Error("Error saving the configuration", "err", err)
					}
					logger.Info("Hosts Config saved.", "hosts", config.App.Config.Hosts)

				}

			}
			// TODO: Check for Subnet ID and if not ask for them
			logger.Debug("Hosts Config exists.", "hosts", config.App.Config.Hosts)
		} else {
			logger.Warn("No configuration file found.")
		}

		// Verify all constraints are met
		if err := constraints.CheckConstraints(
			constraints.WithSSMPlugin(),
			constraints.WithAWSProfile(),
			constraints.WithAWSRegion(),
			constraints.WithENV(),
			constraints.WithHostConfig(),
		); err != nil {
			return err
		}

		// TODO: Abstract this into a separate function since it's used in multiple places
		// Get VPC ID from Subnet ID if it's not populated
		if config.App.Config.BastionVPCID == "" {
			if config.App.Config.BastionSubnetID == "" {
				logger.Info("No Subnet ID provided. Asking for it.")
				// Get list of subnets in the account (via GetSubnetsWithSSM) and ask the user to pick one with survey
				subnets, err := aws.GetSubnetsWithSSM()
				if err != nil {
					logger.Fatal("Error getting subnets with SSM", "err", err)
				}

				// Ask user to pick a subnet
				err = survey.AskOne(&survey.Select{
					Message: "Bastion Instance will be deployed.\nSelect Subnet ID:",
					Options: func() []string {
						var options []string
						for _, subnet := range subnets {
							options = append(options, *subnet.SubnetId)
						}
						return options
					}(),
					Help: "If you don't see your subnets it means there is no SSM connectivity there",
					Description: func(value string, index int) string {
						var name string
						for _, tag := range subnets[index].Tags {
							if *tag.Key == "Name" {
								name = *tag.Value
							}

						}
						return fmt.Sprintf("SSM: OK, CIDR: %s, Name: %s, VPC ID: %s", *subnets[index].CidrBlock, name, *subnets[index].VpcId)
					},
				}, &config.App.Config.BastionSubnetID, survey.WithValidator(survey.Required))
			}

			logger.Debug("Getting VPC ID from Subnet ID", "Subnet ID", config.App.Config.BastionSubnetID)
			config.App.Config.BastionVPCID, err = aws.GetVPCIDFromSubnet(config.App.Config.BastionSubnetID)
			if err != nil {
				logger.Fatal("Error getting VPC ID from Subnet ID", "err", err)
			}
		}
		logger.Debug("Obtained VPC ID from the Subnet ID", "BastionVPCID", config.App.Config.BastionVPCID)

		// Create and start a fork of the default spinner.
		var createBastionInstanceSpinner *pterm.SpinnerPrinter
		showSpinner := config.App.Config.LogLevel != "debug" && config.App.Config.LogLevel != "info" && constraints.IsInteractiveTerminal() && constraints.SupportsANSIEscapeCodes()

		if showSpinner {
			createBastionInstanceSpinner = ux.StartCustomSpinner("Creating Ad-Hoc EC2 Bastion Instance...")
		} else {
			logger.Debug("Not showing spinner", "logLevel", config.App.Config.LogLevel)
			logger.Info("Creating Ad-Hoc EC2 Bastion Instance...")
		}
		// Apply the configuration using CDKTF
		err = infra.ApplyCDKTF(config.App.Config)
		if err != nil {
			createBastionInstanceSpinner.Fail("Error running CDKTF", err)
			logger.Error("Error running CDKTF", "err", err)
			return err
		}

		if showSpinner {
			createBastionInstanceSpinner.UpdateText("CDKTF stack applied successfully")
		} else {
			logger.Info("CDKTF stack applied successfully")
		}

		// Get BastionHostID
		config.App.Config.BastionHostID, err = tunnel.GetBastionHostIDFromTags()
		if err != nil {
			logger.Fatal("Error discovering bastion host", "error", err)
		}

		if showSpinner {
			createBastionInstanceSpinner.UpdateText(fmt.Sprintf("Waiting for the instance %s to be running...", config.App.Config.BastionHostID))
		} else {
			logger.Debug("Waiting for the instance to be running...", "bastionHostID", config.App.Config.BastionHostID)
		}

		// Wait until the instance is ready to accept SSM connections
		err = aws.WaitForInstanceReady(config.App.Config.BastionHostID)
		if err != nil {
			if showSpinner {
				createBastionInstanceSpinner.Fail("Failed to add local SSH Public key to the instance")
			} else {
				logger.Fatal("Error waiting for instance to be ready", "BastionHostID", config.App.Config.BastionHostID, "error", err)
				os.Exit(1)
			}
		}

		if showSpinner {
			createBastionInstanceSpinner.Success(fmt.Sprintf("Bastion Host %s is ready. Run `atun up`.", config.App.Config.BastionHostID))
		} else {
			logger.Debug("Instance is ready", "bastionHostID", config.App.Config.BastionHostID)
		}

		return nil
	},
}

func init() {
	logger.Debug("Init create command")
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// createCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Add flags for VPC and Subnet
	createCmd.PersistentFlags().String("bastion-vpc-id", "", "VPC ID of the bastion host to be created")
	createCmd.PersistentFlags().String("bastion-subnet-id", "", "Subnet ID of the bastion host to be created")
	createCmd.PersistentFlags().String("aws-key-pair", "", "AWS Key Pair Name to use for the bastion host")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// createCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	logger.Debug("Init add command done")
}

func buildHostConfig(app *config.Atun) error {
	logger.Debug("Building host config")

	var err error

	// Confirm the selection
	startSurvey := false
	err = survey.AskOne(&survey.Confirm{
		Message: fmt.Sprintf("Would you like to create a new host configuration?"),
		Default: true,
	}, &startSurvey)
	if err != nil {
		logger.Fatal("Error getting confirmation:", "err", err)
		return err
	}

	if !startSurvey {
		logger.Info("Config creation cancelled and we need some config to proceed. Exiting.")
		os.Exit(0)
	}

	aws.InitAWSClients(config.App)

	// Get list of subnets in the account (via GetSubnetsWithSSM) to use as default values
	subnets, err := aws.GetSubnetsWithSSM()
	if err != nil {
		log.Fatalf("Error getting available subnets: %v", err)
		return err
	}

	// Prompt for Subnet ID using survey with validation
	err = survey.AskOne(&survey.Select{
		Message: "Select AWS Subnet ID:",
		Options: func() []string {
			var options []string
			for _, subnet := range subnets {
				options = append(options, *subnet.SubnetId)
			}
			return options
		}(),
		Description: func(value string, index int) string {
			var name string
			for _, tag := range subnets[index].Tags {
				if *tag.Key == "Name" {
					name = *tag.Value
				}
			}
			return fmt.Sprintf("CIDR: %s, Name: %s, VPC ID: %s", *subnets[index].CidrBlock, name, *subnets[index].VpcId)
		},
	}, &app.Config.BastionSubnetID, survey.WithValidator(survey.Required))
	if err != nil {
		log.Fatalf("Error getting Subnet ID: %v", err)
		return err
	}

	// Get list of key pairs in the account
	keyPairs, err := aws.GetAvailableKeyPairs()
	if err != nil {
		log.Fatalf("Error getting available key pairs: %v", err)
		return err
	}

	// Ask user to pick a key pair
	err = survey.AskOne(&survey.Select{
		Message: "Select AWS Key Pair:",
		Options: func() []string {
			var options []string
			for _, keyPair := range keyPairs {
				options = append(options, *keyPair.KeyName)
			}
			return options
		}(),
		Description: func(value string, index int) string {
			return fmt.Sprintf("(Created on %s, Signature: %s)", *keyPairs[index].CreateTime, *keyPairs[index].KeyFingerprint)
		},
	}, &app.Config.AWSKeyPair, survey.WithValidator(survey.Required))
	if err != nil {
		log.Fatalf("Error getting AWS Key Pair: %v", err)
		return err
	}

	// Get VPC ID from Subnet ID if it's not populated
	if app.Config.BastionVPCID != "" {
		logger.Debug("Getting VPC ID from Subnet ID", "Subnet ID", app.Config.BastionSubnetID)
		app.Config.BastionVPCID, err = aws.GetVPCIDFromSubnet(app.Config.BastionSubnetID)
		if err != nil {
			logger.Fatal("Error getting VPC ID from Subnet ID", "err", err)
		}
	}
	logger.Debug("VPC ID", "VPC ID", app.Config.BastionVPCID)

	appHosts := app.Config.Hosts

	// if app.Hosts is not empty, Ask user if they want to overwrite current config
	if len(appHosts) > 0 {
		overwrite := false
		err = survey.AskOne(&survey.Confirm{
			Message: fmt.Sprintf("You have %d hosts in the config. Do you want to overwrite them?", len(appHosts)),
			Default: false,
		}, &overwrite)
		if err != nil {
			log.Fatalf("Error getting confirmation: %v", err)
			return err
		}

		if !overwrite {
			logger.Info("Config creation cancelled and we need some config to proceed. Exiting.")
			os.Exit(0)
		}
	}

	// Clear the hosts slice
	app.Config.Hosts = []config.Host{}

	// Ask 4 questions for each host and then ask if there is one more host to add
	for i := 0; ; i++ {

		host := config.Host{}

		// Determine defaults based on the current iteration and `app.Config.Hosts`
		var defaultHost, defaultProtocol, defaultRemotePort, defaultLocalPort string

		if i < len(appHosts) {
			// Suggest defaults from `app.Config.Hosts` if available
			existingHost := appHosts[i]
			defaultHost = existingHost.Name
			defaultProtocol = existingHost.Proto
			defaultRemotePort = strconv.Itoa(existingHost.Remote)
			defaultLocalPort = strconv.Itoa(existingHost.Local)
		} else {
			// No more elements in `app.Config.Hosts`, fall back to no defaults
			defaultHost = ""
			defaultProtocol = "ssm"
			defaultRemotePort = ""
			defaultLocalPort = "0"
		}

		// Ask for host name
		err = survey.AskOne(&survey.Input{
			Message: "Enter DNS of the AWS-based host (nutcorp-api.cluster-xxxxxxxxxxxxxxx.us-east-1.rds.amazonaws.com):",
			Default: defaultHost,
			//Default: "nutcorp-api.cluster-xxxxxxxxxxxxxxx.us-east-1.rds.amazonaws.com", // Test only
		}, &host.Name, survey.WithValidator(survey.Required))
		if err != nil {
			log.Fatalf("Error getting Host Name: %v", err)
			return err
		}

		// Ask for tunnel protocol
		err = survey.AskOne(&survey.Select{
			Message: "Select Host Protocol:",
			Options: []string{"ssm", "k8s", "ssh"},
			Default: defaultProtocol,
			Description: func(value string, index int) string {
				if value == "ssm" {
					return "This is the only option supported (for now). But it's human nature to have an illusion of choice."
				}
				return ""
			},
		}, &host.Proto, survey.WithValidator(survey.Required))
		if err != nil {
			logger.Fatal("Error getting Host Protocol", err)

			return err
		}
		if host.Proto == "ssh" || host.Proto == "k8s" {
			logger.Fatal("Sorry, only SSM is supported for now, but it's on the roadmap. Back to the matrix 💊")
		}

		rp, err := aws.InferPortByHost(host.Name)
		if err != nil {
			logger.Debug("Error inferring port from the host", "host", host, "error", err)
		} else {
			logger.Debug("Inferred remote port from the host", "host", host, "port", rp)
			defaultRemotePort = strconv.Itoa(rp)

			lp, err := tunnel.CalculateLocalPort(rp)
			defaultLocalPort = strconv.Itoa(lp)
			if err != nil {
				logger.Fatal("Error calculating local port", "error", err)
			}
		}

		// Survey produces a string, so we need to convert it to int later
		var remotePortSurveyAnswer string

		// Ask for host remote port
		err = survey.AskOne(&survey.Input{
			Message: "Enter Host Remote Port",
			Default: defaultRemotePort,
		}, &remotePortSurveyAnswer, survey.WithValidator(survey.Required))
		if err != nil {
			log.Fatalf("Error getting Host Remote Port: %v", err)
			return err
		}

		host.Remote, err = strconv.Atoi(remotePortSurveyAnswer)
		if err != nil {
			log.Fatalf("Error converting Host Remote Port to int: %v", err)
			return err
		}

		// Survey produces a string, so we need to convert it to int later
		var localPortSurveyAnswer string

		// Ask for host local port
		err = survey.AskOne(&survey.Input{
			Message: "Enter a free local port on your machine:",
			Default: defaultLocalPort,
		}, &localPortSurveyAnswer, survey.WithValidator(survey.Required))
		if err != nil {
			logger.Fatal("Error getting Host Local Port", err)
			return err
		}

		host.Local, err = strconv.Atoi(localPortSurveyAnswer)
		if err != nil {
			logger.Fatal("Error converting Host Local Port to int", err)
		}

		// Add the host to the list
		app.Config.Hosts = append(app.Config.Hosts, host)

		// Ask if there is one more host to add
		oneMore := false
		err = survey.AskOne(&survey.Confirm{
			Message: "Do you want to add one more host?",
			Default: false,
		}, &oneMore)
		if err != nil {
			log.Fatalf("Error getting confirmation: %v", err)
			return err
		}

		if !oneMore {
			break
		}
	}

	return nil
}
