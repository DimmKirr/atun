package cmd

import (
	"github.com/spf13/cobra"
)

// routerCmd represents the router command
var routerCmd = &cobra.Command{
	Use:   "router",
	Short: "Manage connection routers",
	Long: `Commands for creating, connecting to, and managing router endpoints 
that facilitate secure connections to your infrastructure.
	
Available router types:
- EC2: Amazon EC2 router hosts
- Kubernetes (planned): Kubernetes pods acting as jump hosts
- ECS (planned): Amazon ECS containers for connecting to services`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// By default, don't do anything
		return nil
	},
}

func init() {
	routerCmd.AddCommand(routerShellCmd)
	routerCmd.AddCommand(routerListCmd)
	routerCmd.AddCommand(routerCreateCmd)
	routerCmd.AddCommand(routerDeleteCmd)
	routerCmd.AddCommand(routerInstallCmd)
	routerCmd.AddCommand(routerUninstallCmd)

}
