package cmd

import (
	"fmt"
	"github.com/pterm/pterm"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/automationd/atun/internal/aws"
	"github.com/automationd/atun/internal/config"
	"github.com/automationd/atun/internal/logger"
	"github.com/automationd/atun/internal/tunnel"
	"github.com/automationd/atun/internal/ux"
)

// routerListCmd represents the router list command
var routerListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List available routers",
	Long: `List all available routers and their current status.

Example:
  atun router list      # List all available routers
  atun router ls        # Same as above (shorter syntax)`,
	RunE: listRouters,
}

// listRouters displays a list of available routers
func listRouters(cmd *cobra.Command, args []string) error {
	// Create progress spinner
	spinner := ux.NewProgressSpinner("Discovering available routers")

	// Initialize AWS clients
	spinner.UpdateText("Authenticating with AWS...")
	aws.InitAWSClients(config.App)

	// Get routers (routers) with atun.io tags
	spinner.UpdateText("Searching for routers...")
	instanceIDs, err := tunnel.GetRouterHostIDFromTags()
	if err != nil {
		spinner.Fail("Failed to discover routers")
		return fmt.Errorf("error listing routers: %w", err)
	}

	// Convert single ID to slice if needed
	var routerIDs []string
	if instanceIDs != "" {
		routerIDs = []string{instanceIDs}
	} else {
		routerIDs = []string{}
	}

	if len(routerIDs) == 0 {
		spinner.Success("Search completed")
		logger.Info("No routers found. Create one with 'atun router create'")
		return nil
	}

	// Fetch details for each router
	spinner.UpdateText(fmt.Sprintf("Found %d router(s), fetching details...", len(routerIDs)))
	routers := []RouterInfo{}

	// Process each instance
	for _, id := range routerIDs {
		instance, err := getEC2InstanceDetails(id)
		if err != nil {
			logger.Warn(fmt.Sprintf("Could not get details for router %s", id), "error", err)
			continue
		}

		routers = append(routers, instance)
	}

	// Display routers in a table
	renderRouterTable(routers)

	return nil
}

// getEC2InstanceDetails retrieves EC2 instance details
// This is a helper function that uses available AWS functions in your codebase
func getEC2InstanceDetails(instanceID string) (RouterInfo, error) {
	// Use your existing AWS functions to get instance details
	// This might be something like aws.DescribeInstance

	// For now, create a placeholder instance with basic information
	router := RouterInfo{
		ID:        instanceID,
		Type:      "ec2",
		State:     "running",
		CreatedAt: time.Now(),
	}

	return router, nil
}

// renderRouterTable displays a formatted table of routers
func renderRouterTable(routers []RouterInfo) {
	if len(routers) == 0 {
		logger.Info("No routers found")
		return
	}

	// Sort routers by creation time (newest first)
	sort.Slice(routers, func(i, j int) bool {
		return routers[i].CreatedAt.After(routers[j].CreatedAt)
	})

	// Create the table data
	tableData := [][]string{
		{"ID", "TYPE", "STATE", "CREATED"},
	}

	for _, router := range routers {
		tableData = append(tableData, []string{
			router.ID,
			router.Type,
			router.State,
			router.CreatedAt.Format(time.RFC3339),
		})
	}

	// Render table
	pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
}

func init() {

}
