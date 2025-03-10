/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/spf13/cobra"

	awsLib "github.com/automationd/atun/internal/aws"
	"github.com/automationd/atun/internal/config"
	"github.com/automationd/atun/internal/constraints"
	"github.com/automationd/atun/internal/logger"
	"github.com/automationd/atun/internal/ux"
)

// routerInstallCmd represents the router install command
var routerInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install Atun tags on an existing EC2 instance",
	Long: `Install Atun tags on an existing EC2 instance to use it as a router.
This allows you to use an existing instance as an Atun router without creating a new one.

Example:
  atun router install --router i-1234abcd  # Install Atun tags on instance i-1234abcd`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error
		var routerHost string

		// Get the router host ID from the command line
		routerHost = cmd.Flag("router").Value.String()
		if routerHost == "" {
			return fmt.Errorf("router instance ID is required. Use --router flag to specify")
		}

		config.App.Config.RouterHostID = routerHost

		// Check if the configuration is loaded
		if config.App.Config != nil {
			logger.Debug("App config exists (via file, env vars or else). Checking Hosts Config")

			// If config is not loaded, offer to create a new configuration using survey package
			if len(config.App.Config.Hosts) == 0 {
				// TODO Add build config
				logger.Fatal("No hosts found in the configuration. Please create atun.toml")

			}
			// TODO: Check for Subnet ID and if not ask for them
			logger.Debug("Hosts Config exists.", "hosts", config.App.Config.Hosts)
		} else {
			logger.Warn("No configuration file found.")
		}

		// Create a spinner
		installSpinner := ux.NewProgressSpinner("Installing Atun on existing instance")

		// Check constraints
		if err := constraints.CheckConstraints(
			constraints.WithAWSProfile(),
			constraints.WithENV(),
		); err != nil {
			installSpinner.Fail("Constraint check failed")
			return err
		}

		// Initialize AWS clients
		installSpinner.UpdateText("Authenticating with AWS...")
		awsLib.InitAWSClients(config.App)

		// Get router instance ID from flag
		routerID, _ := cmd.Flags().GetString("router")
		if routerID == "" {
			installSpinner.Fail("No router instance ID provided")
			return fmt.Errorf("router instance ID is required. Use --router flag to specify")
		}

		// TODO: abstract EC2 stuff into aws package
		// Verify instance exists
		installSpinner.UpdateText(fmt.Sprintf("Verifying instance %s exists...", routerID))
		ec2Client, err := awsLib.NewEC2Client(*config.App.Session.Config)
		if err != nil {
			installSpinner.Fail(fmt.Sprintf("Failed to create EC2 client: %v", err))
			return err
		}

		_, err = ec2Client.DescribeInstances(&ec2.DescribeInstancesInput{
			InstanceIds: []*string{aws.String(routerID)},
		})
		if err != nil {
			installSpinner.Fail(fmt.Sprintf("Instance %s not found: %v", routerID, err))
			return fmt.Errorf("instance %s not found: %w", routerID, err)
		}

		// Create tags for the instance
		installSpinner.UpdateText("Creating Atun tags...")

		// Create EC2 tags array
		var tags []*ec2.Tag

		// Add version and environment tags
		tags = append(tags, &ec2.Tag{
			Key:   aws.String("atun.io/version"),
			Value: aws.String(config.App.Version),
		})

		tags = append(tags, &ec2.Tag{
			Key:   aws.String("atun.io/env"),
			Value: aws.String(config.App.Config.Env),
		})

		// Process each host and add it to the tags
		for _, host := range config.App.Config.Hosts {
			key := fmt.Sprintf("atun.io/host/%s", host.Name)
			hostConfig := map[string]interface{}{
				"proto":  host.Proto,
				"local":  host.Local,
				"remote": host.Remote,
			}

			configJSON, err := json.Marshal(hostConfig)
			if err != nil {
				installSpinner.Fail(fmt.Sprintf("Failed to marshal host config: %v", err))
				return fmt.Errorf("failed to marshal host config: %w", err)
			}

			tags = append(tags, &ec2.Tag{
				Key:   aws.String(key),
				Value: aws.String(string(configJSON)),
			})
		}

		// Apply tags to the instance
		_, err = ec2Client.CreateTags(&ec2.CreateTagsInput{
			Resources: []*string{aws.String(routerID)},
			Tags:      tags,
		})
		if err != nil {
			installSpinner.Fail(fmt.Sprintf("Failed to apply tags: %v", err))
			return fmt.Errorf("failed to apply tags: %w", err)
		}

		// Update the router host ID in config
		config.App.Config.RouterHostID = routerID

		installSpinner.Success(fmt.Sprintf("Successfully installed Atun on instance %s", routerID))
		logger.Info(fmt.Sprintf("Router installed on instance ID: %s", routerID))
		logger.Info("To connect to your infrastructure, run: atun up")

		return nil
	},
}

func init() {
	routerInstallCmd.Flags().String("router", "", "ID of the existing EC2 instance to install Atun on (required)")
	routerInstallCmd.MarkFlagRequired("router")
}
