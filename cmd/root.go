/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/automationd/atun/internal/config"
	"github.com/automationd/atun/internal/constraints"
	"github.com/automationd/atun/internal/logger"
	"github.com/pterm/pterm"
	"github.com/spf13/viper"

	//"github.com/automationd/atun/internal/config"
	"os"

	"github.com/spf13/cobra"
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
	// TODO: Use Method receiver. Create atun (config) here
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
	//
	//// Ensure all constraints are met
	//if err := constraints.CheckConstraints(
	//	constraints.WithAWSProfile(),
	//	constraints.WithAWSRegion(),
	//); err != nil {
	//	pterm.Error.Println("Failed to check constraints:", err)
	//	os.Exit(1)
	//}
	//
	//// Init AWS Session (probably should be moved to a separate function)
	//sess, err := aws.GetSession(&aws.SessionConfig{
	//	Region:      config.App.Config.AWSRegion,
	//	Profile:     config.App.Config.AWSProfile,
	//	EndpointUrl: config.App.Config.EndpointUrl,
	//})
	//if err != nil {
	//	panic(err)
	//}
	//
	//logger.Debug("AWS Session initialized")
	//config.App.Session = sess

	// Set directory for per-env-per-profile tunnel/cdk
	config.App.Config.TunnelDir = filepath.Join(config.App.Config.AppDir, fmt.Sprintf("%s-%s", config.App.Config.Env, config.App.Config.AWSProfile))

	if !constraints.SupportsANSIEscapeCodes() || constraints.IsCI() {
		logger.Debug("Terminal supports ANSI escape codes", "supportsANSI", constraints.SupportsANSIEscapeCodes())
		logger.Debug("Terminal is CI", "isCI", constraints.IsCI())

		// If the terminal is non-interactive or doesn't support ANSI enable plain text logging automatically (even if it's set to true)
		config.App.Config.LogPlainText = true
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
