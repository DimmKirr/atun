/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package constraints

import (
	"errors"
	"fmt"
	"github.com/Masterminds/semver"
	"github.com/automationd/atun/internal/config"
	"github.com/pterm/pterm"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
	"strings"
)

type constraints struct {
	configFile    bool
	ssmplugin     bool
	structure     bool
	nvm           bool
	awsProfile    bool
	awsRegion     bool
	env           bool
	hostConfig    bool
	bastionHostID bool
}

// CheckConstraints checks if the constraints are met
func CheckConstraints(options ...Option) error {
	r := constraints{}
	for _, opt := range options {
		opt(&r)
	}

	if r.ssmplugin {
		if err := checkSessionManagerPlugin(); err != nil {
			return err
		}
	}

	if r.awsProfile {
		if err := validateAwsProfile(config.App.Config); err != nil {
			return err
		}
	}

	if r.awsRegion {
		if err := validateAwsRegion(config.App.Config); err != nil {
			return err
		}
	}

	if r.env {
		if err := validateEnv(config.App.Config); err != nil {
			return err
		}
	}

	if r.hostConfig {
		if err := validateHostConfig(config.App); err != nil {
			return err
		}
	}

	if r.bastionHostID {
		if err := validateBastionHostConfigID(config.App.Config); err != nil {
			return err
		}
	}

	if len(viper.ConfigFileUsed()) == 0 && r.configFile {
		return fmt.Errorf("this command requires a config file. Please add atun.toml to %s", config.App.Config.AppDir)
	}

	return nil
}

type Option func(*constraints)

func WithAtunStructure() Option {
	return func(r *constraints) {
		r.structure = true
	}
}

func WithConfigFile() Option {
	return func(r *constraints) {
		r.configFile = true
	}
}

func WithSSMPlugin() Option {
	return func(r *constraints) {
		r.ssmplugin = true
	}
}

func WithNVM() Option {
	return func(r *constraints) {
		r.nvm = true
	}
}

func checkNVM() error {
	if len(os.Getenv("NVM_DIR")) == 0 {
		return errors.New("nvm is not installed (visit https://github.com/nvm-sh/nvm)")
	}

	return nil
}

func WithAWSProfile() Option {
	return func(r *constraints) {
		r.awsProfile = true
	}
}

func WithAWSRegion() Option {
	return func(r *constraints) {
		r.awsRegion = true
	}
}

func WithENV() Option {
	return func(r *constraints) {
		r.env = true
	}
}

func WithHostConfig() Option {
	return func(r *constraints) {
		r.hostConfig = true
	}
}

func WithBastionHostID() Option {
	return func(r *constraints) {
		r.bastionHostID = true
	}
}

// ValidateAwsProfile checks if AWS_PROFILE is set in the config
func validateAwsProfile(cfg *config.Config) error {
	if cfg.AWSProfile == "" {
		return errors.New("AWS_PROFILE is not set. Please set it via command line or environment variable.")
	}
	return nil
}

// ValidateAwsRegion checks if AWS_REGION is set in the config
func validateAwsRegion(cfg *config.Config) error {
	if cfg.AWSRegion == "" {
		return errors.New("AWS_REGION is not set. Please set it via command line or environment variable.")
	}
	return nil
}

// ValidateEnv checks if ENV is set in the config
func validateEnv(cfg *config.Config) error {
	if cfg.Env == "" {
		return errors.New("ENV is not set. Please set it via command line or environment variable.")
	}
	return nil
}

// ValidateEnv checks if ENV is set in the config
func validateHostConfig(cfg *config.Atun) error {
	if len(cfg.Config.Hosts) == 0 {
		//logger.Debug("Elements found in host config. Checking contents)

		return errors.New("Host Config is not set. Please set it via command line or environment variables.")
	}

	// Check if cfg.Hosts has all required fields according to the Host struct
	for _, host := range cfg.Config.Hosts {
		if host.Name == "" {
			return errors.New("Host Name is not set. Please set it via config file.")
		}

		// Check if host.Remote (integer) is not more than 0
		if host.Remote <= 0 {
			return errors.New("Host Remote port is not set. Please set it via config file.")
		}

		if host.Local < 0 {
			return errors.New("Host Local port is not set. Please set it via config file.")
		}

		if host.Proto == "" {
			{
				return errors.New("Host Protocol is not set. Please set it via config file.")
			}

		}
	}

	return nil
}

func validateBastionHostConfigID(cfg *config.Config) error {
	if cfg.BastionHostID == "" {
		return errors.New("Bastion Host ID is not set.")
	}
	return nil
}

func checkDocker() error {
	exist, _ := CheckCommand("docker", []string{"info"})
	if !exist {
		return errors.New("docker is not running or is not installed (visit https://www.docker.com/get-started)")
	}

	return nil
}

func isStructured() bool {
	var isStructured = false

	cwd, err := os.Getwd()
	if err != nil {
		logrus.Fatalln("can't initialize config: %w", err)
	}

	_, err = os.Stat(filepath.Join(cwd, ".ize"))
	if !os.IsNotExist(err) {
		isStructured = true
	}

	_, err = os.Stat(filepath.Join(cwd, ".infra"))
	if !os.IsNotExist(err) {
		isStructured = true
	}

	return isStructured
}

func checkSessionManagerPlugin() error {
	exist, _ := CheckCommand("session-manager-plugin", []string{})
	if !exist {
		pterm.Warning.Println("SSM Agent plugin is not installed. Trying to install SSM Agent plugin")

		var pyVersion string

		exist, pyVersion := CheckCommand("python3", []string{"--version"})
		if !exist {
			exist, pyVersion = CheckCommand("python", []string{"--version"})
			if !exist {
				return errors.New("python is not installed")
			}

			c, err := semver.NewConstraint("<= 2.6.5")
			if err != nil {
				return err
			}

			v, err := semver.NewVersion(strings.TrimSpace(strings.Split(pyVersion, " ")[1]))
			if err != nil {
				return err
			}

			if c.Check(v) {
				return fmt.Errorf("python version %s below required %s", v.String(), "2.6.5")
			}
			return errors.New("python is not installed")
		}

		c, err := semver.NewConstraint("<= 3.3.0")
		if err != nil {
			return err
		}

		v, err := semver.NewVersion(strings.TrimSpace(strings.Split(pyVersion, " ")[1]))
		if err != nil {
			return err
		}

		if c.Check(v) {
			return fmt.Errorf("python version %s below required %s", v.String(), "3.3.0")
		}

		pterm.DefaultSection.Println("Installing SSM Agent plugin")

		err = downloadSSMAgentPlugin()
		if err != nil {
			return fmt.Errorf("download SSM Agent plugin error: %v (visit https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html)", err)
		}

		pterm.Success.Println("Downloading SSM Agent plugin")

		err = installSSMAgent()
		if err != nil {
			return fmt.Errorf("install SSM Agent plugin error: %v (visit https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html)", err)
		}

		pterm.Success.Println("Installing SSM Agent plugin")

		err = cleanupSSMAgent()
		if err != nil {
			return fmt.Errorf("cleanup SSM Agent plugin error: %v (visit https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html)", err)
		}

		pterm.Success.Println("Cleanup Session Manager plugin installation package")

		exist, _ = CheckCommand("session-manager-plugin", []string{})
		if !exist {
			return fmt.Errorf("check SSM Agent plugin error: %v (visit https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html)", err)
		}
	}

	return nil
}
