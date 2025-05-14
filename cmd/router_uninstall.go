/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package cmd

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/spf13/cobra"

	awsLib "github.com/DimmKirr/atun/internal/aws"
	"github.com/DimmKirr/atun/internal/config"
	"github.com/DimmKirr/atun/internal/constraints"
	"github.com/DimmKirr/atun/internal/logger"
	"github.com/DimmKirr/atun/internal/ux"
)

// routerUninstallCmd represents the router uninstall command
var routerUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove Atun tags from an EC2 instance",
	Long: `Remove all Atun tags (atun.io/*) from an EC2 instance.
This command does not terminate or modify the instance, only removes the Atun-specific tags.

Example:
  atun router uninstall --router i-1234abcd  # Remove Atun tags from instance i-1234abcd`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Create a spinner
		uninstallSpinner := ux.NewProgressSpinner("Uninstalling Atun from instance")

		// Check constraints
		if err := constraints.CheckConstraints(
			constraints.WithAWSProfile(),
			constraints.WithENV(),
		); err != nil {
			uninstallSpinner.Fail("Constraint check failed")
			return err
		}

		// Initialize AWS clients
		uninstallSpinner.UpdateText("Authenticating with AWS...")
		awsLib.InitAWSClients(config.App)

		// Get router instance ID from flag
		routerID, _ := cmd.Flags().GetString("router")
		if routerID == "" {
			uninstallSpinner.Fail("No router instance ID provided")
			return fmt.Errorf("router instance ID is required. Use --router flag to specify")
		}

		// Verify instance exists and get current tags
		uninstallSpinner.UpdateText(fmt.Sprintf("Verifying instance %s exists and getting tags...", routerID))
		ec2Client, err := awsLib.NewEC2Client(*config.App.Session.Config)
		if err != nil {
			uninstallSpinner.Fail(fmt.Sprintf("Failed to create EC2 client: %v", err))
			return err
		}

		result, err := ec2Client.DescribeInstances(&ec2.DescribeInstancesInput{
			InstanceIds: []*string{aws.String(routerID)},
		})
		if err != nil {
			uninstallSpinner.Fail(fmt.Sprintf("Instance %s not found: %v", routerID, err))
			return fmt.Errorf("instance %s not found: %w", routerID, err)
		}

		// Ensure we have an instance and it has reservations
		if len(result.Reservations) == 0 || len(result.Reservations[0].Instances) == 0 {
			uninstallSpinner.Fail(fmt.Sprintf("Instance %s not found", routerID))
			return fmt.Errorf("instance %s not found", routerID)
		}

		// Get the instance's tags
		instance := result.Reservations[0].Instances[0]
		var atunTags []*ec2.Tag

		// Find all atun.io/* tags
		for _, tag := range instance.Tags {
			if tag.Key != nil && strings.HasPrefix(*tag.Key, "atun.io/") {
				atunTags = append(atunTags, &ec2.Tag{
					Key:   tag.Key,
					Value: tag.Value,
				})
			}
		}

		// Check if any atun.io tags were found
		if len(atunTags) == 0 {
			uninstallSpinner.Warning(fmt.Sprintf("No Atun tags found on instance %s", routerID))
			return nil
		}

		// Remove the tags
		uninstallSpinner.UpdateText(fmt.Sprintf("Removing %d Atun tags from instance %s...", len(atunTags), routerID))
		_, err = ec2Client.DeleteTags(&ec2.DeleteTagsInput{
			Resources: []*string{aws.String(routerID)},
			Tags:      atunTags,
		})
		if err != nil {
			uninstallSpinner.Fail(fmt.Sprintf("Failed to remove tags: %v", err))
			return fmt.Errorf("failed to remove tags: %w", err)
		}

		// If this is the current router host in config, clear it
		if config.App.Config.RouterHostID == routerID {
			config.App.Config.RouterHostID = ""
		}

		uninstallSpinner.Success(fmt.Sprintf("Successfully removed %d Atun tags from instance %s", len(atunTags), routerID))
		logger.Info(fmt.Sprintf("Atun tags removed from instance %s", routerID))

		return nil
	},
}

func init() {
	routerCmd.AddCommand(routerUninstallCmd)
	routerUninstallCmd.Flags().String("router", "", "ID of the EC2 instance to remove Atun tags from (required)")
	routerUninstallCmd.MarkFlagRequired("router")
}
