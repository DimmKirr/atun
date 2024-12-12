/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package cmd

import (
	"fmt"
	"github.com/automationd/atun/internal/aws"
	"github.com/automationd/atun/internal/config"

	"github.com/pterm/pterm"
	"os"

	"github.com/spf13/cobra"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of the tunnel and current environment",
	Long: `Show status of the tunnel and current environment.
	This is also useful for troubleshooting`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implement Tunnel Status

		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("can't load options for a command: %w", err)
		}

		dt := pterm.DefaultTable

		aws.InitAWSClients(config.App)

		// TODO: Hide this info behind --debug flag or move to a `debug` command
		pterm.DefaultSection.Println("Status")
		_ = dt.WithData(pterm.TableData{
			{"AWS_ACCOUNT", aws.GetAccountId()},
			{"AWS_PROFILE", config.App.Config.AWSProfile},
			{"AWS_REGION", config.App.Config.AWSRegion},
			{"PWD", cwd},
			{"SSH_KEY_PATH", config.App.Config.SSHKeyPath},
			{"Config File", config.App.Config.ConfigFile},
			{"Bastion Host", config.App.Config.BastionHostID},

			//{"Toggle", toggleValue},
		}).WithLeftAlignment().Render()

		return err
	},
}

func init() {

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// statusCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	//statusCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
