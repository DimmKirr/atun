/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/DimmKirr/atun/internal/config"
	"github.com/DimmKirr/atun/internal/constraints"
	"github.com/DimmKirr/atun/internal/logger"
	"github.com/pterm/pterm"
	"github.com/spf13/viper"

	"github.com/spf13/cobra"
	"os"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "atun",
	Short: "AWS Tagged Tunnel",
	Long: `SSH tunnel cli tool that works without local configuration. 
	It uses EC2 tags to define hosts and ports forwarding configuration. 
	atun.io schema namespace can be used to configure an SSM tunnel.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	// Parse the flags to make them available for use
	rootCmd.ParseFlags(os.Args[1:])

	// Check for --json flag in the args
	hasJSONFlag, _ := rootCmd.Flags().GetBool("json")

	// If --json flag is present, set up plain mode and quiet mode before any initialization
	if hasJSONFlag {
		// Set plain mode
		if err := rootCmd.PersistentFlags().Set("plain", "true"); err != nil {
			fmt.Fprintf(os.Stderr, "Error setting plain flag: %v\n", err)
			os.Exit(1)
		}

		// Disable pterm output for JSON
		pterm.DisableStyling()

		// Enable quiet mode to suppress all logs except errors
		logger.SetQuietMode(true)
	}

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().String("log-level", "", "Specify log level (debug/info/warn/error)")
	if err := viper.BindPFlag("LOG_LEVEL", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		pterm.Info.Println("Not binding log-level flag (none provied)")
	}

	// Plain flag to disable terminal formatting
	rootCmd.PersistentFlags().Bool("plain", false, "Disable all terminal formatting and colors")

	// JSON flag to root command for global use
	rootCmd.PersistentFlags().BoolP("json", "j", false, "Output in JSON format when available")

	rootCmd.PersistentFlags().String("aws-profile", "", "Specify AWS profile (defined in ~/.aws/credentials)")
	if err := viper.BindPFlag("AWS_PROFILE", rootCmd.PersistentFlags().Lookup("aws-profile")); err != nil {
		pterm.Info.Println("Not binding aws-profile flag (none provied)")
	}

	rootCmd.PersistentFlags().String("aws-region", "", "Specify AWS region (e.g. us-east-1)")
	if err := viper.BindPFlag("AWS_REGION", rootCmd.PersistentFlags().Lookup("aws-region")); err != nil {
		pterm.Info.Println("Not binding binding aws-region flag (none provided)")
	}

	rootCmd.PersistentFlags().String("env", "", "Specify environment (dev/prod/...)")
	if err := viper.BindPFlag("ENV", rootCmd.PersistentFlags().Lookup("env")); err != nil {
		pterm.Info.Println("Not binding binding env flag (none provided)")
	}

	//if err := viper.BindPFlags(rootCmd.Flags()); err != nil {
	//	pterm.Error.Println("Error while binding flags")
	//}

	// TODO: Use Method Receiver (pass atun all the way to the command)
	rootCmd.AddCommand(
		upCmd,
		downCmd,
		statusCmd,
		versionCmd,
		routerCmd,
	)

	//cobra.OnInitialize(config.LoadConfig)
	cobra.OnInitialize(initializeAtun)
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.atun.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	//rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

}
func initializeAtun() {
	// Load config into a global struct
	err := config.LoadConfig()
	if err != nil {
		panic(err)
	}

	// Check if --plain flag was set
	if plain, _ := rootCmd.Flags().GetBool("plain"); plain {
		config.App.Config.LogPlainText = true
	}

	// Set directory for per-env-per-profile tunnel/cdk
	config.App.Config.TunnelDir = filepath.Join(config.App.Config.AppDir, fmt.Sprintf("%s-%s", config.App.Config.Env, config.App.Config.AWSProfile))

	// Only check for ANSI support if we're not in plain mode
	if !config.App.Config.LogPlainText {

		if !constraints.SupportsANSIEscapeCodes() || constraints.IsCI() {
			logger.Debug("Terminal doesn't support ANSI escape codes", "supportsANSI", constraints.SupportsANSIEscapeCodes())
			logger.Debug("Terminal is CI", "isCI", constraints.IsCI())

			// If the terminal is non-interactive or doesn't support ANSI, enable plain text logging automatically (even if it's set to false)
			config.App.Config.LogPlainText = true
		} else {
			logger.Debug("Terminal supports ANSI escape codes")
		}
	}

	logger.Debug("Tunnel directory set. Ensuring it exists", "tunnelDir", config.App.Config.TunnelDir)
	err = os.MkdirAll(config.App.Config.TunnelDir, 0755)
	if err != nil {
		logger.Fatal("Error creating tunnel directory", "tunnelDir", config.App.Config.TunnelDir, "error", err)
		panic(err)
	}

	////Initialize Atun struct with configuration
	//atun, err = NewAtun(cfg)
	//if err != nil {
	//	log.Fatalf("failed to initialize atun: %v", err)
	//}

}
