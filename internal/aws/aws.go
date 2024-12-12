/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package aws

import (
	"fmt"
	"github.com/automationd/atun/internal/config"
	"github.com/automationd/atun/internal/constraints"
	"github.com/automationd/atun/internal/logger"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/pterm/pterm"
	"os"
	"strings"
	"time"
)

func InitAWSClients(app *config.Atun) {
	logger.Debug("Authenticating to AWS", "profile", app.Config.AWSProfile, "region", app.Config.AWSRegion)
	// Ensure all constraints are met
	if err := constraints.CheckConstraints(
		constraints.WithAWSProfile(),
		constraints.WithAWSRegion(),
	); err != nil {
		pterm.Error.Println("Failed to check constraints:", err)
		os.Exit(1)
	}

	// Init AWS Session (probably should be moved to a separate function)
	sess, err := GetSession(&SessionConfig{
		Region:      app.Config.AWSRegion,
		Profile:     app.Config.AWSProfile,
		EndpointUrl: app.Config.AWSEndpointUrl,
	})
	if err != nil {
		panic(err)
	}

	logger.Debug("AWS Session initialized")
	app.Session = sess
}

func NewEC2Client(awsConfig aws.Config) (*ec2.EC2, error) {
	logger.Debug("Creating EC2 client.", "AWSProfile", config.App.Config.AWSProfile, "awsRegion", config.App.Config.AWSRegion, "endpointURL", awsConfig.Endpoint)

	sess, err := session.NewSessionWithOptions(session.Options{
		Config:            awsConfig,
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		return nil, err
	}

	ec2Client := ec2.New(sess)

	return ec2Client, nil
}

func NewSTSClient(awsConfig aws.Config) (*sts.STS, error) {
	sess, err := session.NewSessionWithOptions(session.Options{
		Config: awsConfig,
	})
	if err != nil {
		return nil, err
	}

	stsClient := sts.New(sess)

	logger.Debug("Created STS client", "AWSProfile", config.App.Config.AWSProfile, "AWSRegion", config.App.Config.AWSRegion)
	return stsClient, nil
}

// ListInstancesWithTag returns a list of EC2 instances with a specific tag
func ListInstancesWithTags(tags map[string]string) ([]*ec2.Instance, error) {
	ec2Client, err := NewEC2Client(*config.App.Session.Config)
	if err != nil {
		logger.Error("Failed to create EC2 client", "error", err)
		return nil, err
	}

	if len(tags) == 0 {
		return nil, fmt.Errorf("no tags provided for filtering")
	}

	var filters []*ec2.Filter
	for key, value := range tags {
		filters = append(filters,
			&ec2.Filter{
				Name:   aws.String(fmt.Sprintf("tag:%s", key)),
				Values: []*string{aws.String(value)},
			},
			&ec2.Filter{
				Name:   aws.String("instance-state-name"),
				Values: []*string{aws.String("running")},
			})
	}

	input := &ec2.DescribeInstancesInput{
		Filters: filters,
	}

	var instances []*ec2.Instance
	err = ec2Client.DescribeInstancesPages(input, func(page *ec2.DescribeInstancesOutput, lastPage bool) bool {
		for _, reservation := range page.Reservations {
			instances = append(instances, reservation.Instances...)
		}
		return !lastPage
	})
	if err != nil {
		logger.Error("Failed to describe instances", "error", err)
		return nil, err
	}

	logger.Debug(fmt.Sprintf("Found %d instances with matching tags", len(instances)))
	return instances, nil
}

func GetInstanceTags(instanceID string) (map[string]string, error) {
	ec2Client, err := NewEC2Client(*config.App.Session.Config)
	if err != nil {
		logger.Error("Failed to create EC2 client", "error", err)
		return nil, err
	}

	input := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{aws.String(instanceID)},
	}

	result, err := ec2Client.DescribeInstances(input)
	if err != nil {
		logger.Error("Failed to describe instances", "error", err)
		return nil, err
	}

	tags := make(map[string]string)
	for _, reservation := range result.Reservations {
		for _, instance := range reservation.Instances {
			for _, tag := range instance.Tags {
				tags[*tag.Key] = *tag.Value
			}
		}
	}

	if len(tags) == 0 {
		logger.Error("No tags found for instance", "instanceID", instanceID)
		return nil, fmt.Errorf("no tags found for instance %s", instanceID)
	}

	return tags, nil
}

func GetAccountId() string {
	stsClient, err := NewSTSClient(*config.App.Session.Config)
	if err != nil {
		logger.Error("Error creating STS client", "error", err)
		return ""
	}

	result, err := stsClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		logger.Error("Error getting account ID", "error", err)
		return ""
	}

	return *result.Account
}

func SendSSHPublicKey(instanceID string, publicKey string) error {
	// This command is executed in the bastion host and it checks if our public publicKey is present. If it's not it uploads it to the authorized_keys file.
	command := fmt.Sprintf(
		`mkdir -p /home/ec2-user/.ssh/ && touch /home/ec2-user/.ssh/authorized_keys && grep -qR "%s" /home/ec2-user/.ssh/authorized_keys || echo "%s" | tee -a /home/ec2-user/.ssh/authorized_keys`,
		strings.TrimSpace(publicKey), strings.TrimSpace(publicKey),
	)

	//command := fmt.Sprintf(
	//	`whoami`,
	//)

	//command := `whoami`

	logger.Debug("Sending command", "command", command)

	sendCommandOutput, err := ssm.New(config.App.Session).SendCommand(&ssm.SendCommandInput{
		InstanceIds:  []*string{&instanceID},
		DocumentName: aws.String("AWS-RunShellScript"),
		Comment:      aws.String("Add an SSH public publicKey to authorized_keys"),
		Parameters: map[string][]*string{
			"commands": {&command},
		},
	})
	if err != nil {
		return fmt.Errorf("can't send SSH public publicKey: %w", err)
	}

	commandID := *sendCommandOutput.Command.CommandId
	logger.Debug("Command sent. Waiting for completion", "commandID", commandID)

	var output *ssm.GetCommandInvocationOutput
	for i := 0; i < 5; i++ {
		output, err = ssm.New(config.App.Session).GetCommandInvocation(&ssm.GetCommandInvocationInput{
			CommandId:  aws.String(commandID),
			InstanceId: aws.String(instanceID),
		})
		if err == nil && *output.Status == ssm.CommandInvocationStatusSuccess && output.ResponseCode != nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return fmt.Errorf("can't get command invocation: %w", err)
	}

	//

	// Separate success criteria localstack ssm implementation bug where ResponseCode == nil)
	if strings.Contains(config.App.Config.AWSEndpointUrl, "localhost") || strings.Contains(config.App.Config.AWSEndpointUrl, "127.0.0.1") {
		if *output.Status != "Success" {
			return fmt.Errorf("command failed %s: %s, ", *output.StandardOutputContent, *output.StandardErrorContent)
		}

		return nil
	}

	// Non-localstack success criteria
	if *output.ResponseCode != 0 && *output.Status != "Success" {
		return fmt.Errorf("command failed with exit code %d: %s, ", *output.ResponseCode, *output.StandardErrorContent)
	}

	return nil
}

// GetVPCIDFromSubnet returns the VPC ID for a given subnet ID
func GetVPCIDFromSubnet(subnetID string) (string, error) {
	ec2Client, err := NewEC2Client(*config.App.Session.Config)
	if err != nil {
		return "", err
	}

	logger.Debug("Getting VPC ID for subnet", "subnetID", subnetID, "endpoint", ec2Client.Endpoint)

	input := &ec2.DescribeSubnetsInput{
		SubnetIds: []*string{aws.String(subnetID)},
	}

	result, err := ec2Client.DescribeSubnets(input)
	if err != nil {
		return "", err
	}

	if len(result.Subnets) == 0 {
		return "", fmt.Errorf("no subnets found for ID %s", subnetID)
	}

	return *result.Subnets[0].VpcId, nil
}
