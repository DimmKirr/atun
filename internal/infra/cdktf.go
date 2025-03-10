/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package infra

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/automationd/atun/internal/aws"
	"github.com/automationd/atun/internal/config"
	"github.com/automationd/atun/internal/constraints"
	"github.com/automationd/atun/internal/logger"
	"github.com/aws/jsii-runtime-go"
	awsprovider "github.com/cdktf/cdktf-provider-aws-go/aws/v19/provider"
	"github.com/hashicorp/terraform-cdk-go/cdktf"
)

// createStack defines the CDKTF stack (generates Terraform).
func createStack(c *config.Config) {
	app := cdktf.NewApp(&cdktf.AppConfig{
		Outdir: jsii.String(filepath.Join(c.TunnelDir)), // Set your desired directory here
	})

	stack := cdktf.NewTerraformStack(app, jsii.String(fmt.Sprintf("%s-%s", c.AWSProfile, c.Env)))

	// Configure the local backend to store state in the tunnel directory
	cdktf.NewLocalBackend(stack, &cdktf.LocalBackendConfig{
		Path: jsii.String(filepath.Join(c.TunnelDir, "terraform.tfstate")), // Specify state file path
	})

	awsprovider.NewAwsProvider(stack, jsii.String("AWS"), &awsprovider.AwsProviderConfig{
		Region:  jsii.String(c.AWSRegion),
		Profile: jsii.String(c.AWSProfile),
	})

	// TODO: get hosts from atun.toml and add it to the tags with a loop

	atun := config.Atun{
		Version: "1",
		Config:  c,
	}

	//hostConfigJSON, err := json.Marshal(Host{
	//	Proto:  "ssm",
	//	Remote: "22",
	//	Local:  "10001",
	//})
	//
	//if err != nil {
	//	pterm.Error.Sprintf("Error marshalling host config: %v", err)
	//}
	//
	//tags := map[string]interface{}{
	//	"atun.io/version": "1",
	//	fmt.Sprintf("atun.io/host/%s", "ip-10-30-25-144.ec2.internal"): string(hostConfigJSON),
	//}

	// Create a final map to hold the JSON structure
	tags := make(map[string]interface{})

	// Add the version directly to the final map
	tags["atun.io/version"] = atun.Version

	// Set Env
	tags["atun.io/env"] = atun.Config.Env

	// TODO: Support multiple port configurations per host
	// Group hosts by their Name and create slices for their configurations
	hostConfigs := make(map[string]map[string]interface{})

	// Process each host and add it to the final map using the Name as the key
	for _, host := range atun.Config.Hosts {
		key := fmt.Sprintf("atun.io/host/%s", host.Name)
		hostConfig := map[string]interface{}{
			"proto":  host.Proto,
			"local":  host.Local,
			"remote": host.Remote,
		}

		hostConfigs[key] = hostConfig

	}

	// Marshal each grouped configuration into a JSON string and store in finalMap
	for key, configs := range hostConfigs {
		configsJSON, _ := json.Marshal(configs)
		tags[key] = string(configsJSON)
	}

	//// Convert struct to JSON
	//jsonData, err := json.Marshal(atun)
	//if err != nil {
	//	fmt.Println("Error marshaling to JSON:", err)
	//	return
	//}

	//if err := json.Unmarshal(jsonData, &tags); err != nil {
	//	fmt.Println("Error unmarshaling JSON to map:", err)
	//	return
	//}

	// TODO: Add ability to use other Terraform modules. Maybe use a map of modules and their Parameters, like "module-name": {"param1": "value1", "param2": "value2"}
	// Add the module

	if err := constraints.CheckConstraints(
		constraints.WithSSMPlugin(),
		constraints.WithAWSProfile(),
		constraints.WithAWSRegion(),
		constraints.WithENV(),
	); err != nil {
		logger.Fatal("Error checking constraints", "error", err)
	}

	logger.Debug("All constraints satisfied")

	// Override the default ami if one is provided to atun
	routerHostAmi := ""
	if config.App.Config.RouterHostAMI != "" {
		routerHostAmi = config.App.Config.RouterHostAMI
	}

	_, isPrivate, err := aws.CheckSubnetNetworkAccess(config.App.Config.RouterSubnetID)
	if err != nil {
		logger.Fatal("Error checking subnet network access", "error", err)
	}

	var publicSubnets []string
	var privateSubnets []string
	if isPrivate {
		privateSubnets = append(privateSubnets, config.App.Config.RouterSubnetID)
	} else {
		publicSubnets = append(publicSubnets, config.App.Config.RouterSubnetID)
	}

	// TODO: Add ability to specify other modules
	terraformVariablesModules := map[string]interface{}{
		"env":                 config.App.Config.Env,
		"name":                config.App.Config.RouterInstanceName,
		"ec2_key_pair_name":   config.App.Config.AWSKeyPair,
		"public_subnets":      publicSubnets,
		"private_subnets":     privateSubnets,
		"allowed_cidr_blocks": []string{"0.0.0.0/0"},
		"instance_type":       config.App.Config.AWSInstanceType,
		"instance_ami":        routerHostAmi,

		"vpc_id": config.App.Config.RouterVPCID,
		"tags":   tags,
	}

	logger.Debug("Terraform Variables", "variables", terraformVariablesModules)

	cdktf.NewTerraformHclModule(stack, jsii.String("router"), &cdktf.TerraformHclModuleConfig{
		// TODO: Make an abstraction atun-router module so anyone can fork and switch configs
		Source:  jsii.String("hazelops/ec2-router/aws"),
		Version: jsii.String("~>4.0"),

		Variables: &terraformVariablesModules,
	})

	app.Synth()
}

// ApplyCDKTF performs the 'apply' of theCDKTF stack
func ApplyCDKTF(c *config.Config) error {
	logger.Debug("Applying CDKTF stack.", "profile", c.AWSProfile, "region", c.AWSRegion)

	createStack(c)
	// Change to the synthesized directory
	synthDir := filepath.Join(c.TunnelDir, "stacks", fmt.Sprintf("%s-%s", c.AWSProfile, c.Env))
	logger.Debug("Synthesized directory", "dir", synthDir)

	// Ensure correct Terraform version is installed
	if err := CheckTerraformVersion(); err != nil {
		logger.Debug("Installing required Terraform version", "version", c.TerraformVersion)
		if err := InstallTerraform(c.TerraformVersion); err != nil {
			return fmt.Errorf("failed to install terraform: %w", err)
		}
	}

	// Get the path to the Terraform binary
	terraformPath, err := GetTerraformPath()
	if err != nil {
		return fmt.Errorf("failed to get terraform path: %w", err)
	}

	// Initialize Terraform
	cmd := exec.Command(terraformPath, "init")
	cmd.Dir = synthDir
	if c.LogLevel == "info" || c.LogLevel == "debug" {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to initialize terraform: %w", err)
	}

	// Apply Terraform
	cmd = exec.Command(terraformPath, "apply", "-auto-approve")
	logger.Debug("Running terraform apply", "cmd", cmd)
	cmd.Dir = synthDir
	// Only show Terraform if LogPlainText is enabled (EndUser doesn't need to see Terraform output)
	if c.LogPlainText && (c.LogLevel == "info" || c.LogLevel == "debug") {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	return cmd.Run()
}

func DestroyCDKTF(c *config.Config) error {
	createStack(c)
	// Change to the synthesized directory
	synthDir := filepath.Join(c.TunnelDir, "stacks", fmt.Sprintf("%s-%s", c.AWSProfile, c.Env))

	// Ensure correct Terraform version is installed
	if err := CheckTerraformVersion(); err != nil {
		logger.Info("Installing required Terraform version", "version", c.TerraformVersion)
		if err := InstallTerraform(c.TerraformVersion); err != nil {
			return fmt.Errorf("failed to install terraform: %w", err)
		}
	}

	// Get the path to the Terraform binary
	terraformPath, err := GetTerraformPath()
	if err != nil {
		return fmt.Errorf("failed to get terraform path: %w", err)
	}

	// Initialize Terraform
	cmd := exec.Command(terraformPath, "init")
	cmd.Dir = synthDir
	if c.LogLevel == "info" || c.LogLevel == "debug" {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to initialize terraform: %w", err)
	}

	// Destroy Terraform
	cmd = exec.Command(terraformPath, "destroy", "-auto-approve")
	cmd.Dir = synthDir
	if c.LogLevel == "info" || c.LogLevel == "debug" {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}
