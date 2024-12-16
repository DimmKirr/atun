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
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/aws/aws-sdk-go/service/opensearchservice"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/pterm/pterm"
	"os"
	"strings"
	"time"
)

func InitAWSClients(app *config.Atun) {
	logger.Debug("Authenticating with AWS", "profile", app.Config.AWSProfile, "region", app.Config.AWSRegion)
	// Ensure all constraints are met
	if err := constraints.CheckConstraints(
		constraints.WithAWSProfile(),
		//constraints.WithAWSRegion(),
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
		logger.Fatal("Failed to initialize AWS session", "error", err)
	}
	if config.App.Config.AWSRegion == "" {
		logger.Debug("AWS Region not set. Setting it to the default value", "region", *sess.Config.Region)
		config.App.Config.AWSRegion = *sess.Config.Region
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

func NewRDSClient(awsConfig aws.Config) (*rds.RDS, error) {
	logger.Debug("Creating RDS client.", "AWSProfile", config.App.Config.AWSProfile, "awsRegion", config.App.Config.AWSRegion, "endpointURL", awsConfig.Endpoint)

	sess, err := session.NewSessionWithOptions(session.Options{
		Config:            awsConfig,
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		return nil, err
	}

	rdsClient := rds.New(sess)

	return rdsClient, nil
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

func SendSSHPublicKey(instanceID string, publicKey string, bastionHostUser string) error {
	// This command is executed in the bastion host and it checks if our public publicKey is present. If it's not it uploads it to the authorized_keys file.
	// If the bastionHostUser is "root" then set bastionHostUserDirectory to /root otherwise set it to /home/bastionHostUser
	bastionHostUserDirectory := fmt.Sprintf("/home/%s", bastionHostUser)
	if bastionHostUser == "root" {
		bastionHostUserDirectory = "/root"
	}

	command := fmt.Sprintf(
		`bash -c 'mkdir -p %s/.ssh && grep --qR "%s" %s/.ssh/authorized_keys || echo "%s" >> %s/.ssh/authorized_keys'`,
		bastionHostUserDirectory,
		strings.TrimSpace(publicKey),
		bastionHostUserDirectory,
		strings.TrimSpace(publicKey),
		bastionHostUserDirectory,
	)

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

// GetAvailableSubnets returns a list of available subnets in AWS Account (in all VPCs)
func GetAvailableSubnets() ([]*ec2.Subnet, error) {
	ec2Client, err := NewEC2Client(*config.App.Session.Config)
	if err != nil {
		return nil, err
	}

	input := &ec2.DescribeSubnetsInput{}

	var subnets []*ec2.Subnet
	err = ec2Client.DescribeSubnetsPages(input, func(page *ec2.DescribeSubnetsOutput, lastPage bool) bool {
		subnets = append(subnets, page.Subnets...)
		return !lastPage
	})
	if err != nil {
		return nil, err
	}

	return subnets, nil
}

// GetAvailableKeyPairs returns a list of available key pairs in AWS Account
func GetAvailableKeyPairs() ([]*ec2.KeyPairInfo, error) {
	ec2Client, err := NewEC2Client(*config.App.Session.Config)
	if err != nil {
		return nil, err
	}

	input := &ec2.DescribeKeyPairsInput{}

	result, err := ec2Client.DescribeKeyPairs(input)
	if err != nil {
		return nil, err
	}

	return result.KeyPairs, nil
}

// InferPortByHost finds the remote port of a service (RDS, ElastiCache, OpenSearch) by matching its endpoint hostname.
func InferPortByHost(host string) (int, error) {
	// Check RDS clusters
	if port, err := inferPortFromRDS(host); err == nil {
		return port, nil
	}

	// Check ElastiCache clusters (Redis/Memcached)
	if port, err := inferPortFromElastiCache(host); err == nil {
		return port, nil
	}

	// Check OpenSearch clusters
	if port, err := inferPortFromOpenSearch(host); err == nil {
		return port, nil
	}

	return 0, fmt.Errorf("no matching service found with endpoint hostname: %s", host)
}

// inferPortFromRDS checks RDS clusters for a matching endpoint and returns its port.
func inferPortFromRDS(host string) (int, error) {
	rdsClient, err := NewRDSClient(*config.App.Session.Config)
	if err != nil {
		return 0, fmt.Errorf("failed to create RDS client: %v", err)
	}

	var clusters []*rds.DBCluster
	err = rdsClient.DescribeDBClustersPages(&rds.DescribeDBClustersInput{},
		func(page *rds.DescribeDBClustersOutput, lastPage bool) bool {
			clusters = append(clusters, page.DBClusters...)
			return !lastPage
		})
	if err != nil {
		return 0, fmt.Errorf("failed to describe RDS clusters: %v", err)
	}

	for _, cluster := range clusters {
		if cluster.Endpoint != nil && strings.EqualFold(*cluster.Endpoint, host) {
			return int(*cluster.Port), nil
		}
	}
	return 0, fmt.Errorf("no RDS cluster found with endpoint hostname: %s", host)
}

// inferPortFromElastiCache checks ElastiCache clusters (Redis and Memcached) for a matching endpoint and returns its port.
func inferPortFromElastiCache(host string) (int, error) {
	elastiCacheClient := elasticache.New(session.New()) // Initialize ElastiCache client
	input := &elasticache.DescribeCacheClustersInput{
		ShowCacheNodeInfo: aws.Bool(true),
	}

	var clusters []*elasticache.CacheCluster
	err := elastiCacheClient.DescribeCacheClustersPages(input,
		func(page *elasticache.DescribeCacheClustersOutput, lastPage bool) bool {
			clusters = append(clusters, page.CacheClusters...)
			return !lastPage
		})
	if err != nil {
		return 0, fmt.Errorf("failed to describe ElastiCache clusters: %v", err)
	}

	for _, cluster := range clusters {
		for _, node := range cluster.CacheNodes {
			if node.Endpoint != nil && strings.EqualFold(*node.Endpoint.Address, host) {
				return int(*node.Endpoint.Port), nil
			}
		}
	}
	return 0, fmt.Errorf("no ElastiCache cluster found with endpoint hostname: %s", host)
}

// inferPortFromOpenSearch checks OpenSearch domains for a matching endpoint and returns the default port (443 for HTTPS).
func inferPortFromOpenSearch(host string) (int, error) {
	osClient := opensearchservice.New(session.New()) // Initialize OpenSearch client

	var domains []*opensearchservice.DomainStatus
	input := &opensearchservice.DescribeDomainsInput{
		DomainNames: []*string{}, // Will fetch all domains in the next step
	}

	// Get all OpenSearch domain names first
	domainsList, err := osClient.ListDomainNames(&opensearchservice.ListDomainNamesInput{})
	if err != nil {
		return 0, fmt.Errorf("failed to list OpenSearch domains: %v", err)
	}
	for _, domain := range domainsList.DomainNames {
		input.DomainNames = append(input.DomainNames, domain.DomainName)
	}

	// Describe each domain to find endpoint information
	output, err := osClient.DescribeDomains(input)
	if err != nil {
		return 0, fmt.Errorf("failed to describe OpenSearch domains: %v", err)
	}

	domains = output.DomainStatusList
	for _, domain := range domains {
		if domain.Endpoint != nil && strings.EqualFold(*domain.Endpoint, host) {
			// OpenSearch usually listens on HTTPS (port 443)
			return 443, nil
		}
	}
	return 0, fmt.Errorf("no OpenSearch domain found with endpoint hostname: %s", host)
}

func WaitForInstanceReady(instanceID string) error {
	ec2Client, err := NewEC2Client(*config.App.Session.Config)
	if err != nil {
		return err
	}

	input := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{&instanceID},
	}

	err = ec2Client.WaitUntilInstanceRunning(input)
	if err != nil {
		return err
	}

	ssmClient := ssm.New(config.App.Session)
	ssmInput := &ssm.DescribeInstanceInformationInput{
		InstanceInformationFilterList: []*ssm.InstanceInformationFilter{
			{
				Key:      aws.String("InstanceIds"),
				ValueSet: []*string{aws.String(instanceID)},
			},
		},
	}

	ssmOutput, err := ssmClient.DescribeInstanceInformation(ssmInput)
	if err != nil {
		return fmt.Errorf("error describing instance information: %w", err)
	}

	if len(ssmOutput.InstanceInformationList) == 0 || *ssmOutput.InstanceInformationList[0].PingStatus != "Online" {
		return fmt.Errorf("SSM agent is not active or inaccessible on instance %s. Does the subnet have Internet (via NAT)?", instanceID)
	}

	return nil
}
