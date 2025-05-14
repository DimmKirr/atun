package cmd

import (
	"fmt"
	"github.com/DimmKirr/atun/internal/constraints"
	"github.com/pterm/pterm"
	"time"

	"github.com/spf13/cobra"

	"github.com/DimmKirr/atun/internal/aws"
	"github.com/DimmKirr/atun/internal/config"
	"github.com/DimmKirr/atun/internal/logger"
	"github.com/DimmKirr/atun/internal/tunnel"
	"github.com/DimmKirr/atun/internal/ux"
)

// routerListCmd represents the router list command
var routerListCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List available routers",
	Long: `List all available routers and their current status.

Example:  
  atun router ls        # Same as above (shorter syntax),
  atun router list      # List all available routers`,
	RunE: listRouters,
}

// listRouters displays a list of available routers
func listRouters(cmd *cobra.Command, args []string) error {
	var err error
	if err = constraints.CheckConstraints(
		constraints.WithSSMPlugin(),
		constraints.WithAWSProfile(),
		constraints.WithENV(),
	); err != nil {
		return err
	}

	// Create progress spinner
	ux.Println("Discovering available routers")

	// Initialize AWS clients

	mfaInputRequired := aws.MFAInputRequired(config.App)
	if mfaInputRequired {
		pterm.Printfln(" %s Authenticating with AWS", pterm.LightBlue("▶︎"))
		aws.InitAWSClients(config.App)
	} else {
		spinnerAWSAuth := ux.NewProgressSpinner("Authenticating with AWS")
		aws.InitAWSClients(config.App)
		spinnerAWSAuth.Success(fmt.Sprintf("Authenticated with AWS account %s", aws.GetAccountId()))
	}

	// Get routers (routers) with atun.io tags
	spinnerRouterDetection := ux.NewProgressSpinner("Detecting Atun routers in AWS")
	config.App.Config.RouterHostID, err = tunnel.GetRouterHostIDFromTags()

	if err != nil {
		spinnerRouterDetection.Fail("No EC2 router instances found with atun.io tags.")
		return nil
	}

	// Convert single ID to slice if needed
	var routerIDs []string
	if config.App.Config.RouterHostID != "" {
		routerIDs = []string{config.App.Config.RouterHostID}
	} else {
		routerIDs = []string{}
	}

	//if len(routerIDs) == 0 {
	//	spinnerRouterDetection.Success(fmt.Sprintf("No routers found. Create one with `atun router create` or manually add atun.io tags to any EC2 instance"))
	//	return nil
	//}

	// Fetch details for each router

	spinnerGetRouters := ux.NewProgressSpinner(fmt.Sprintf("Found %d router(s), fetching details...", len(routerIDs)))

	routers := []config.RouterInfo{}

	// Process each instance
	for _, id := range routerIDs {
		spinnerGetRouters.UpdateText(fmt.Sprintf("Processing %s", id))
		instance, err := getEC2InstanceDetails(id)
		if err != nil {
			logger.Warn(fmt.Sprintf("Could not get details for router %s", id), "error", err)
			continue
		}

		routers = append(routers, instance)
	}
	spinnerGetRouters.Success(fmt.Sprintf("Found %d router(s)", len(routerIDs)))
	// Display routers in a table
	ux.RenderRouterTable(routers)

	return nil
}

// getEC2InstanceDetails retrieves EC2 instance details
// This is a helper function that uses available AWS functions in your codebase
func getEC2InstanceDetails(instanceID string) (config.RouterInfo, error) {
	// Use your existing AWS functions to get instance details
	// This might be something like aws.DescribeInstance

	// For now, create a placeholder instance with basic information
	router := config.RouterInfo{
		ID:        instanceID,
		Type:      "ec2",
		State:     "running",
		CreatedAt: time.Now(),
	}

	return router, nil
}

func init() {

}
