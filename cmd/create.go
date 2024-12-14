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
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"log"
	"os"
	"regexp"
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

		//customTheme := pterm.Theme{
		//	InfoPrefixStyle: *pterm.NewStyle(pterm.FgGreen),
		//	//InfoPrefixStyle:  pterm.Prefix{Text: "i", Style: pterm.NewStyle(pterm.FgCyan)},
		//	InfoMessageStyle: *pterm.NewStyle(pterm.FgCyan),
		//}
		//
		//pterm.ThemeDefault = customTheme
		//
		//pterm.Info.Printfln("Creating EC2 Bastion Host")
		//pterm.Printfln("Creating EC2 Bastion Host")

		// Create and start a fork of the default spinner.
		createBastionInstanceSpinner, _ := pterm.DefaultSpinner.Start("Creating EC2 Instance...")
		//time.Sleep(time.Second * 2) // Simulate 3 seconds of processing something.

		// Check if the configuration is loaded
		if config.App.Config != nil {
			logger.Debug("App config file exists. Checking Hosts Config")
			pterm.Debug.Printfln("App config file exists. Checking Hosts Config")

			// If config is not loaded, offer to create a new configuration using survey package
			if len(config.App.Config.Hosts) == 0 {
				logger.Debug(
					"No hosts found in the configuration. Offering to create one")

				err = buildHostConfig(config.App)
				if err != nil {
					createBastionInstanceSpinner.Fail("Error creating host config: %v", err)
					logger.Error("Error creating host config: %v", err)
				}

				// TODO: Save config file

			}
			// TODO: Check for Subnet ID and if not ask for them
			logger.Debug("Hosts Config exists.", "hosts", config.App.Config.Hosts)
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

		aws.InitAWSClients(config.App)

		// TODO: Abstract this into a separate function since it's used in multiple places
		// Get VPC ID from Subnet ID if it's not populated
		if config.App.Config.BastionVPCID == "" {
			logger.Debug("Getting VPC ID from Subnet ID", "Subnet ID", config.App.Config.BastionSubnetID)
			config.App.Config.BastionVPCID, err = aws.GetVPCIDFromSubnet(config.App.Config.BastionSubnetID)
			if err != nil {
				createBastionInstanceSpinner.Fail("Error getting VPC ID from Subnet ID", err)
				logger.Fatal("Error getting VPC ID from Subnet ID", "err", err)
			}
		}
		logger.Debug("Obtained VPC ID from the Subnet ID", "BastionVPCID", config.App.Config.BastionVPCID)

		// Apply the configuration using CDKTF
		err = infra.ApplyCDKTF(config.App.Config)
		if err != nil {
			createBastionInstanceSpinner.Fail("Error running CDKTF", err)
			logger.Error("Error running CDKTF", "err", err)
			return err
		}

		logger.Info("CDKTF stack applied successfully")
		createBastionInstanceSpinner.Success()

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
	var confirmation = false

	// Confirm the selection
	startSurvey := false
	err = survey.AskOne(&survey.Confirm{
		Message: fmt.Sprintf("Would you like to create a new host configuration?"),
		Default: true,
	}, &startSurvey)
	if err != nil {
		logger.Fatal("Error getting confirmation:", err)
		return err
	}

	if !startSurvey {
		logger.Info("Config creation cancelled and we need some config to proceed. Exiting.")
		os.Exit(0)
	}

	// Prompt for Subnet ID using survey with validation
	err = survey.AskOne(&survey.Input{
		Message: "Enter AWS Subnet ID:",
		Default: app.Config.BastionSubnetID,
	}, &app.Config.BastionSubnetID, survey.WithValidator(survey.ComposeValidators(
		survey.Required,
		func(val interface{}) error {
			str, ok := val.(string)
			if !ok {
				return fmt.Errorf("invalid input")
			}
			if !regexp.MustCompile(`^subnet-[0-9a-f]{8,17}$`).MatchString(str) {
				return fmt.Errorf("unexpected subnet ID format, expected subnet-xxxxxxxxxxxxxx")
			}
			return nil
		},
	)))
	if err != nil {
		log.Fatalf("Error getting Subnet ID: %v", err)
		return err
	}

	err = survey.AskOne(&survey.Input{
		Message: "Enter AWS Key Pair (must exist):",
		Default: app.Config.AWSKeyPair, // If there is value from config we'll use it as a default, if not it will be empty
	}, &app.Config.AWSKeyPair, survey.WithValidator(survey.Required))
	if err != nil {
		log.Fatalf("Error getting Subnet ID: %v", err)
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

		// TODO: Detect remote port by the host name (if it's RDS mySQL use 3306, if it's RDS PostgreSQL use 5432)

		// Survey produces a string, so we need to convert it to int later
		var remotePortSurveyAnswer string

		// Ask for host remote port
		err = survey.AskOne(&survey.Input{
			Message: "Enter Host Remote Port \n(MySQL: 3306, PostgreSQL: 5432)",
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
			Message: "Enter a free local port on your machine (0 for automatic):",
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

	// Confirm the selection
	confirmation = false
	err = survey.AskOne(&survey.Confirm{
		Message: fmt.Sprintf("You entered: %v\nConfirm?", app.Config.Hosts),
		Default: false,
	}, &confirmation)

	if err != nil {
		log.Fatalf("Error getting confirmation: %v", err)
		return err
	}

	if confirmation {
		logger.Info("Configuration confirmed. Proceeding...")

		//	// Continue with the rest of your logic
	}

	//
	//if confirmation {
	//	logger.Info("Configuration confirmed. Proceeding...")
	//
	//	// Continue with the rest of your logic
	//	return nil
	//} else {
	//	logger.Info("Configuration not confirmed. Exiting.")
	//}

	return nil
}
