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
var routerCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Creates an ad-hoc router host to a specified subnet",
	Long: `Creates ad-hoc router host to a specified subnet. Performed via CDKTF/Terraform 
	This is useful when there is no IaC in place and there is a need to connect to a resource private.
	State is saved locally and it's advised to delete it after the task is finished.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error

		mfaInputRequired := aws.MFAInputRequired(config.App)
		if mfaInputRequired {
			pterm.Printfln(" %s Authenticating with AWS", pterm.LightBlue("▶︎"))
			aws.InitAWSClients(config.App)
		} else {
			spinnerAWSAuth := ux.NewProgressSpinner("Authenticating with AWS")
			aws.InitAWSClients(config.App)
			spinnerAWSAuth.Success(fmt.Sprintf("Authenticated with AWS account %s", aws.GetAccountId()))
		}

		spinnerCheckConfig := ux.NewProgressSpinner("Checking if local config exists")
		// Check if the configuration is loaded
		if config.App.Config != nil {
			if config.App.Config.ConfigFile != "" {
				spinnerCheckConfig.Success(fmt.Sprintf("Router config file found. Deployment subnet: %s", config.App.Config.RouterSubnetID), "config", config.App.Config)

			} else {
				spinnerCheckConfig.Success(fmt.Sprintf("Router config found in env vars. Subnet %s", config.App.Config.RouterSubnetID))
			}

			spinnerCheckHostsConfig := ux.NewProgressSpinner("Checking if hosts config exists")
			// If config is not loaded, offer to create a new configuration using survey package
			if len(config.App.Config.Hosts) == 0 {
				spinnerCheckHostsConfig.Warning("No hosts found in the configuration")

				err = buildHostConfig(config.App)
				if err != nil {
					logger.Error("Error creating endpoints config: %v", err)
				}

				// Use Survey to ask if the user wants to save the configuration
				saveConfig := true

				// Use pterm.InteractiveConfirm for user confirmation

				saveConfig, err := ux.GetConfirmation("Would you like to save the configuration?")
				if err != nil {
					log.Fatal("Error getting confirmation:", err)
				}

				if saveConfig {
					// Save the configuration to the file
					err = config.SaveConfig()
					if err != nil {
						logger.Error("Error saving the configuration", "err", err)
					}
					logger.Info("Hosts Config saved.", "hosts", config.App.Config.Hosts)

				}

			} else {
				spinnerCheckHostsConfig.Success(fmt.Sprintf("%v hosts found in config", len(config.App.Config.Hosts)))
			}
			// TODO: Check for Subnet ID and if not ask for them
			logger.Debug("Hosts Config exists.", "hosts", config.App.Config.Hosts)
		} else {
			spinnerCheckConfig.Success("No config found. Creating a new one.")
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
		if config.App.Config.RouterVPCID == "" {
			if config.App.Config.RouterSubnetID == "" {
				logger.Info("No Subnet ID provided. Asking for it.")
				// Get list of subnets in the account (via GetSubnetsWithSSM) and ask the user to pick one with survey
				subnets, err := aws.GetSubnetsWithSSM()
				if err != nil {
					logger.Fatal("Error getting subnets with SSM", "err", err)
				}

				// Ask user to pick a subnet
				err = survey.AskOne(&survey.Select{
					Message: "Router Instance will be deployed.\nSelect Subnet ID:",
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
				}, &config.App.Config.RouterSubnetID, survey.WithValidator(survey.Required))
			}

			spinnerGetVPCID := ux.NewProgressSpinner("Getting VPC ID from Subnet ID")
			config.App.Config.RouterVPCID, err = aws.GetVPCIDFromSubnet(config.App.Config.RouterSubnetID)
			if err != nil {
				spinnerGetVPCID.Fail(fmt.Sprintf("Error getting VPC ID from Subnet ID %s. Is your local config correct? \n%v", config.App.Config.RouterSubnetID, err))

				return fmt.Errorf("can't provision router host")
			}

		}
		logger.Debug("Obtained VPC ID from the Subnet ID", "RouterVPCID", config.App.Config.RouterVPCID)

		// Create and start a fork of the default spinner.
		createRouterInstanceSpinner := ux.NewProgressSpinner("Creating Ad-Hoc EC2 Router Instance...")

		// Apply the configuration using CDKTF
		err = infra.ApplyCDKTF(config.App.Config)
		if err != nil {
			createRouterInstanceSpinner.Fail("Error running CDKTF", err)
			logger.Error("Error running CDKTF", "err", err)
			return err
		}
		createRouterInstanceSpinner.UpdateText("CDKTF stack applied successfully")

		// Get RouterHostID
		config.App.Config.RouterHostID, err = tunnel.GetRouterHostIDFromTags()
		if err != nil {
			logger.Fatal("Error discovering router host", "error", err)
		}
		createRouterInstanceSpinner.UpdateText(fmt.Sprintf("Waiting for the instance %s to be running...", config.App.Config.RouterHostID))

		// Wait until the instance is ready to accept SSM connections
		err = aws.WaitForInstanceReady(config.App.Config.RouterHostID)
		if err != nil {
			createRouterInstanceSpinner.Fail("Failed to add local SSH Public key to the instance")

		}
		createRouterInstanceSpinner.Success(fmt.Sprintf("Router Endpoint %s is ready. Run `atun up`.", config.App.Config.RouterHostID))

		return nil
	},
}

func buildHostConfig(app *config.Atun) error {
	logger.Debug("Building endpoints config")

	var err error

	// Confirm the selection
	startSurvey := false
	startSurvey, err = ux.GetConfirmation("Create a new endpoint configuration?")
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

	// Prompt for Subnet ID using pterm InteractiveSelect
	options := []string{}
	subnetMap := map[string]string{}

	for _, subnet := range subnets {
		var name string
		for _, tag := range subnet.Tags {
			if *tag.Key == "Name" {
				name = *tag.Value
			}
		}
		displayText := fmt.Sprintf("CIDR: %s, Name: %s, VPC ID: %s", *subnet.CidrBlock, name, *subnet.VpcId)
		options = append(options, displayText)
		subnetMap[displayText] = *subnet.SubnetId
	}

	selectedSubnetID, err := ux.GetInteractiveSelection("Select Subnet ID", options)

	if err != nil {
		logger.Fatal("Error selecting subnet:", "err", err)
	}

	app.Config.RouterSubnetID = subnetMap[selectedSubnetID]

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
	selectedAWSKeyPair, err := ux.GetInteractiveSelection("Select AWS Key Pair", func() []string {
		var options []string
		for _, keyPair := range keyPairs {
			options = append(options, *keyPair.KeyName)
		}
		return options
	}())
	if err != nil {
		log.Fatalf("Error getting AWS Key Pair: %v", err)
		return err
	}
	app.Config.AWSKeyPair = selectedAWSKeyPair
	if err != nil {
		log.Fatalf("Error getting AWS Key Pair: %v", err)
		return err
	}

	// Get VPC ID from Subnet ID if it's not populated
	if app.Config.RouterVPCID != "" {
		logger.Debug("Getting VPC ID from Subnet ID", "Subnet ID", app.Config.RouterSubnetID)
		app.Config.RouterVPCID, err = aws.GetVPCIDFromSubnet(app.Config.RouterSubnetID)
		if err != nil {
			logger.Fatal("Error getting VPC ID from Subnet ID", "err", err)
		}
	}
	logger.Debug("VPC ID", "VPC ID", app.Config.RouterVPCID)

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
	app.Config.Hosts = []config.Endpoint{}

	// Ask 4 questions for each host and then ask if there is one more host to add
	for i := 0; ; i++ {

		host := config.Endpoint{}

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
		host.Name, err = ux.GetTextInput("Enter Endpoint Name", defaultHost)
		if err != nil {
			log.Fatalf("Error getting Endpoint Name: %v", err)
			return err
		}

		// Ask for tunnel protocol
		//err = survey.AskOne(&survey.Select{
		//	Message: "Select Endpoint Protocol:",
		//	Options: []string{"ssm", "k8s", "ssh"},
		//	Default: defaultProtocol,
		//	Description: func(value string, index int) string {
		//		if value == "ssm" {
		//			return "This is the only option supported (for now). But it's human nature to have an illusion of choice."
		//		}
		//		return ""
		//	},
		//}, &host.Proto, survey.WithValidator(survey.Required))

		host.Proto, err = ux.GetInteractiveSelection("Select Endpoint Protocol", []string{"ssm", "k8s", "ssh"}, defaultProtocol)
		if err != nil {
			logger.Fatal("Error getting Endpoint Protocol", err)

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
		remotePortSurveyAnswer, err = ux.GetTextInput("Enter Remote Port", defaultRemotePort)
		if err != nil {
			log.Fatalf("Error getting Endpoint Remote Port: %v", err)
			return err
		}

		host.Remote, err = strconv.Atoi(remotePortSurveyAnswer)
		if err != nil {
			log.Fatalf("Error converting Endpoint Remote Port to int: %v", err)
			return err
		}

		// Survey produces a string, so we need to convert it to int later
		var localPortSurveyAnswer string

		// Ask for host local port
		localPortSurveyAnswer, err = ux.GetTextInput("Enter Local Port", defaultLocalPort)
		if err != nil {
			logger.Fatal("Error getting Endpoint Local Port", err)
			return err
		}

		host.Local, err = strconv.Atoi(localPortSurveyAnswer)
		if err != nil {
			logger.Fatal("Error converting Endpoint Local Port to int", err)
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

func init() {
	// Add flags for VPC and Subnet
	routerCreateCmd.PersistentFlags().String("router-vpc-id", "", "VPC ID of the router host to be created")
	routerCreateCmd.PersistentFlags().String("router-subnet-id", "", "Subnet ID of the router host to be created")
	routerCreateCmd.PersistentFlags().String("aws-key-pair", "", "AWS Key Pair Name to use for the router host")
}
