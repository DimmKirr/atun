/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package config

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DimmKirr/atun/internal/logger"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pterm/pterm"
	"github.com/spf13/viper"
)

type Atun struct {
	Version string `json:"atun.io/version"`
	Config  *Config
	Session *session.Session
}

type Config struct {
	Hosts                       []Endpoint
	SSHKeyPath                  string
	SSHConfigFile               string
	SSHStrictHostKeyChecking    bool
	SSHSocketFile               string
	AWSProfile                  string
	AWSRegion                   string
	AWSKeyPair                  string
	AWSEndpointUrl              string
	AWSInstanceType             string
	AWSMFASharedCredentialsFile string
	AWSMFACode                  string
	ConfigFile                  string
	RouterVPCID                 string
	RouterSubnetID              string
	RouterHostID                string
	RouterInstanceName          string
	RouterHostAMI               string
	RouterHostUser              string
	AppDir                      string
	TunnelDir                   string
	LogLevel                    string
	LogPlainText                bool
	Env                         string
	AutoAllocatePort            bool
	TerraformVersion            string
	DemoMode                    bool
}

// TODO: Add ability to add multiple ports for forwarding for one host
//  (maybe <host>: [{"local":0, "remote":22, "proto": "ssm"}, {"local":0, "remote":443, "proto": "ssm"}])

type Endpoint struct {
	Name   string `jsonschema:"-"`
	Proto  string `json:"proto" jsonschema:"proto"`
	Remote int    `json:"remote" jsonschema:"remote"`
	Local  int    `json:"local" jsonschema:"local"`
}

// RouterInfo represents the information about a router
type RouterInfo struct {
	ID        string
	Type      string
	State     string
	CreatedAt time.Time
}

var App *Atun
var InitialApp *Atun

func LoadConfig() error {
	viper.SetEnvPrefix("ATUN")

	replacer := strings.NewReplacer(".", "__")

	viper.SetEnvKeyReplacer(replacer)
	viper.AutomaticEnv()

	// Optionally read from a configuration file
	viper.SetConfigName("atun")
	viper.SetConfigType("toml")

	// Set default log level early
	viper.SetDefault("LOG_LEVEL", "info")

	// Initialize the logger for a bit to provide early logging (using viper defaults)
	logger.Initialize(viper.GetString("LOG_LEVEL"), viper.GetBool("LOG_PLAIN_TEXT"))
	logger.Debug("Initialized Logger")

	currentDir, err := os.Getwd()
	if err != nil {
		logger.Fatal("Error getting current directory")
		panic(err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Fatal("Error getting user home directory")
		panic(err)
	}

	appDir := filepath.Join(homeDir, ".atun")

	// Add config paths. Current directory is the priority over home app path
	viper.AddConfigPath(currentDir)
	viper.AddConfigPath(appDir)

	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			// Config file not found; ignore error if desired
			logger.Debug("No config file found. Using defaults and environment variables.")
		}
	} else {
		logger.Debug("Using config file", "configFile", viper.ConfigFileUsed())
	}
	logger.Debug("Re-initializing logger\n")

	// Initialize the logger after config is read (second time, getting log level and plain text setting from config)
	logger.Initialize(viper.GetString("LOG_LEVEL"), viper.GetBool("LOG_PLAIN_TEXT"))

	// Use ENV env var as a default for viper ENV
	if viper.GetString("ENV") == "" {
		if len(os.Getenv("ENV")) > 0 {
			viper.SetDefault("ENV", os.Getenv("ENV"))
		} else {
			viper.SetDefault("ENV", "adhoc")
		}
	}

	// Use AWS_PROFILE env var as a default for viper AWS_PROFILE
	if viper.GetString("AWS_PROFILE") == "" {
		if len(os.Getenv("AWS_PROFILE")) > 0 {
			viper.SetDefault("AWS_PROFILE", os.Getenv("AWS_PROFILE"))
		}
		// No default intentionally to avoid confusion
	}

	// Use AWS_REGION env var as a default for viper AWS_REGION
	if viper.GetString("AWS_REGION") == "" {
		if len(os.Getenv("AWS_REGION")) > 0 {
			viper.SetDefault("AWS_REGION", os.Getenv("AWS_REGION"))
		}
		// No default intentionally to avoid confusion
	}

	// Use AWS_ENDPOINT_URL env var as a default for viper AWS_ENDPOINT_URL
	if viper.GetString("AWS_ENDPOINT_URL") == "" {
		if len(os.Getenv("AWS_ENDPOINT_URL")) > 0 {
			viper.SetDefault("AWS_ENDPOINT_URL", os.Getenv("AWS_ENDPOINT_URL"))
		}
		// No default intentionally to avoid confusion
	} // Use AWS_ENDPOINT_URL env var as a default for viper AWS_ENDPOINT_URL
	viper.SetDefault("AWS_MFA_SHARED_CREDENTIALS_FILE", filepath.Join(homeDir, ".aws", "credentials"))

	// Set Default Values if none are set
	viper.SetDefault("SSH_KEY_PATH", filepath.Join(homeDir, ".ssh", "id_rsa"))
	viper.SetDefault("SSH_STRICT_HOST_KEY_CHECKING", true)
	viper.SetDefault("AWS_INSTANCE_TYPE", "t3.nano")
	viper.SetDefault("ROUTER_INSTANCE_NAME", "atun-router")
	viper.SetDefault("SSH_STRICT_HOST_KEY_CHECKING", false) // Strict host key checking is disabled by default for better user experience. Debatable
	viper.SetDefault("AUTO_ALLOCATE_PORT", false)           // Port auto-allocation is disabled by default
	viper.SetDefault("LOG_PLAIN_TEXT", false)               // Set LOG_PLAIN_TEXT to false by default
	viper.SetDefault("TERRAFORM_VERSION", "latest")         // Default to latest Terraform version
	viper.SetDefault("DEMO_MODE", false)                    // Default to false

	// TODO?: Move init a separate file with correct imports of config
	App = &Atun{
		Version: "1",
		Config: &Config{
			Hosts:                       []Endpoint{},
			Env:                         viper.GetString("ENV"),
			SSHKeyPath:                  viper.GetString("SSH_KEY_PATH"),
			SSHStrictHostKeyChecking:    viper.GetBool("SSH_STRICT_HOST_KEY_CHECKING"),
			AWSProfile:                  viper.GetString("AWS_PROFILE"),
			AWSRegion:                   viper.GetString("AWS_REGION"),
			AWSKeyPair:                  viper.GetString("AWS_KEY_PAIR"),
			AWSInstanceType:             viper.GetString("AWS_INSTANCE_TYPE"),
			AWSEndpointUrl:              viper.GetString("AWS_ENDPOINT_URL"),
			AWSMFASharedCredentialsFile: viper.GetString("AWS_MFA_SHARED_CREDENTIALS_FILE"),
			RouterVPCID:                 viper.GetString("ROUTER_VPC_ID"),
			RouterSubnetID:              viper.GetString("ROUTER_SUBNET_ID"),
			RouterHostID:                viper.GetString("ROUTER_HOST_ID"),
			RouterInstanceName:          viper.GetString("ROUTER_INSTANCE_NAME"),
			RouterHostAMI:               viper.GetString("ROUTER_HOST_AMI"),
			RouterHostUser:              viper.GetString("ROUTER_HOST_USER"),
			ConfigFile:                  viper.ConfigFileUsed(),
			AppDir:                      appDir,
			LogLevel:                    viper.GetString("LOG_LEVEL"),
			LogPlainText:                viper.GetBool("LOG_PLAIN_TEXT"),
			AutoAllocatePort:            viper.GetBool("AUTO_ALLOCATE_PORT"),
			TerraformVersion:            viper.GetString("TERRAFORM_VERSION"),
			DemoMode:                    viper.GetBool("DEMO_MODE"),
		},
		Session: nil,
	}

	if err := viper.Unmarshal(&App.Config); err != nil {
		log.Fatalf("Unable to decode initial config into a struct: %v", err)
	}

	// Create Cfg.AppDir if it doesn't exist
	if _, err := os.Stat(App.Config.AppDir); os.IsNotExist(err) {
		if err := os.Mkdir(App.Config.AppDir, os.FileMode(0755)); err != nil {
			logger.Fatal("Error creating app directory", "appDir", App.Config.AppDir, "error", err)
			panic(err)
		}
		pterm.Info.Println("Created app directory:", App.Config.AppDir)
	}

	//
	//pterm.Printfln("Config: %v", App.Config)

	// TODO?: Maybe search for router host id during config stage?
	return nil
}

func SaveConfig() error {
	// Save the config file to the current working directory
	currentDir, err := os.Getwd()
	if err != nil {
		logger.Error("Error getting current directory", "error", err)
		return err
	}

	configFilePath := filepath.Join(currentDir, "atun.toml")

	// TODO: Possibly find a better way to marshal the whole config
	// Add router subnet id to the viper config
	viper.Set("router_subnet_id", App.Config.RouterSubnetID)

	// Add hosts to the the config
	viper.Set("hosts", App.Config.Hosts)
	// Save the main config
	if err := viper.SafeWriteConfigAs(configFilePath); err != nil {
		if os.IsExist(err) {
			logger.Warn("Config file already exists. Please delete it and retry.", "path", configFilePath)
		} else {
			logger.Error("Error writing config file", "error", err)
			return err
		}
	}
	logger.Debug("Saved config file", "path", configFilePath)
	return nil
}
