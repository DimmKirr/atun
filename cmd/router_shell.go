package cmd

import (
	"fmt"
	"github.com/DimmKirr/atun/internal/constraints"
	"github.com/spf13/cobra"
	"os/signal"
	"syscall"

	"github.com/DimmKirr/atun/internal/aws"
	"github.com/DimmKirr/atun/internal/config"
	"github.com/DimmKirr/atun/internal/tunnel"
	"github.com/DimmKirr/atun/internal/ux"
)

// routerShellCmd represents the router ssh command
var routerShellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Connect directly to a router endpoint via SSH",
	Long: `Connect into a router's shell directly to access the command line.
This allows you to perform troubleshooting or run commands directly on the router.

Example usage:
  atun router shell              # Connect to the most recently created router
  atun router shell --target i-1234abcd  # Connect to a specific router by ID`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var targetID string

		// Create progress spinner
		sshSpinner := ux.NewProgressSpinner("Connecting to router")

		// TODO: If there are multiple routers, prompt the user to select one using a survey
		// This is a placeholder for future functionality
		// if len(routerIDs) > 1 {
		// 	selectedRouterID, err := ux.SelectRouter(routerIDs)
		// 	if err != nil {
		// 		spinner.Fail("Failed to select router")
		// 		return fmt.Errorf("error selecting router: %w", err)
		// 	}
		// 	routerID = selectedRouterID
		// }

		// Initialize AWS clients
		sshSpinner.UpdateText("Authenticating with AWS...")
		aws.InitAWSClients(config.App)

		// Get the connection type and target ID
		routerType, _ := cmd.Flags().GetString("type")
		targetID = cmd.Flag("target").Value.String()

		// Default to EC2/SSM if not specified
		if routerType == "" {
			routerType = "ec2"
		}

		// Handle different connection types
		switch routerType {
		case "ec2":
			return consoleToEC2Router(sshSpinner, targetID)
		// Future connection types
		case "k8s":
			sshSpinner.Fail("Kubernetes connections not yet implemented")
			return fmt.Errorf("kubernetes connections are planned for a future release")
		case "ecs":
			sshSpinner.Fail("ECS connections not yet implemented")
			return fmt.Errorf("ecs connections are planned for a future release")
		default:
			sshSpinner.Fail(fmt.Sprintf("Unknown router type: %s", routerType))
			return fmt.Errorf("router type '%s' not supported", routerType)
		}
	},
}

// consoleToEC2Router manages SSM connections to EC2 router instances
func consoleToEC2Router(sshSpinner *ux.ProgressSpinner, targetID string) error {
	var err error

	if err := constraints.CheckConstraints(
		constraints.WithAWSProfile(),
		constraints.WithAWSCLI(),
		constraints.WithSSMPlugin(),
	); err != nil {
		return err
	}

	// If target not provided, get the first running instance
	if targetID == "" {
		sshSpinner.UpdateText("Discovering router...")
		config.App.Config.RouterHostID, err = tunnel.GetRouterHostIDFromTags()
		if err != nil {
			sshSpinner.Fail("No routers found with atun.io tags")
			return fmt.Errorf("no routers found: %w", err)
		}
	} else {
		config.App.Config.RouterHostID = targetID
	}

	sshSpinner.UpdateText(fmt.Sprintf("Connecting to %s...", config.App.Config.RouterHostID))

	err = aws.ConnectToSSMConsole(config.App.Config.RouterHostID)
	if err != nil {
		sshSpinner.Fail("Failed to connect to router", "routerID", config.App.Config.RouterHostID, "error", err)
		return fmt.Errorf("failed to connect to router: %w", err)
	}

	return nil
}

func init() {
	// Ignore interrupt signals during shell session (Ctrl+C)
	signal.Ignore(syscall.SIGINT)

	routerShellCmd.Flags().String("target", "", "Target router identifier (instance ID for EC2)")
	routerShellCmd.Flags().String("type", "", "Router type (ec2, k8s, ecs)")
}
