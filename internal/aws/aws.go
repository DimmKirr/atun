/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package aws

import (
	"fmt"
	"os/exec"

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
	logger.Debug("Creating EC2 client.",
		"AWSProfile", config.App.Config.AWSProfile,
		"awsRegion", config.App.Config.AWSRegion,
		"endpointURL", aws.StringValue(awsConfig.Endpoint),
	)

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

// GetSSMWhoAmI checks if the SSH public key is present in the instance
func GetSSMWhoAmI(instanceID string, routerHostUser string) (string, error) {
	command := fmt.Sprintf(
		`bash -c 'whoami'`,
	)

	logger.Debug("Sending SSM command", "command", command)

	sendCommandOutput, err := ssm.New(config.App.Session).SendCommand(&ssm.SendCommandInput{
		InstanceIds:  []*string{&instanceID},
		DocumentName: aws.String("AWS-RunShellScript"),
		Comment:      aws.String("Check if SSH public key is present in authorized_keys"),
		Parameters: map[string][]*string{
			"commands": {&command},
		},
	})
	if err != nil {
		return "", fmt.Errorf("can't send command: %w", err)
	}

	commandID := *sendCommandOutput.Command.CommandId
	logger.Debug("SSM command sent. Waiting for completion", "commandID", commandID)

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
		return "", fmt.Errorf("can't get command invocation: %w", err)
	}

	if strings.Contains(config.App.Config.AWSEndpointUrl, "localhost") || strings.Contains(config.App.Config.AWSEndpointUrl, "127.0.0.1") {
		if *output.Status != "Success" {
			return "", fmt.Errorf("command failed %s: %s", *output.StandardOutputContent, *output.StandardErrorContent)
		}
		return "localstack", nil
	}

	if *output.ResponseCode != 0 && *output.Status != "Success" {
		return "", fmt.Errorf("command failed with exit code %d: %s", *output.ResponseCode, *output.StandardErrorContent)
	}

	whoamiResponse := *output.StandardOutputContent

	return whoamiResponse, nil
}

func EnsureSSHPublicKeyPresent(instanceID string, publicKey string, routerHostUser string) error {
	// This command is executed in the router host and it checks if our public publicKey is present. If it's not it uploads it to the authorized_keys file.
	// If the routerHostUser is "root" then set routerHostUserDirectory to /root otherwise set it to /home/routerHostUser
	routerHostUserDirectory := fmt.Sprintf("/home/%s", routerHostUser)
	if routerHostUser == "root" {
		routerHostUserDirectory = "/root"
	}

	command := fmt.Sprintf(
		`bash -c 'mkdir -p %s/.ssh && grep --qR "%s" %s/.ssh/authorized_keys || echo "%s" >> %s/.ssh/authorized_keys'`,
		routerHostUserDirectory,
		strings.TrimSpace(publicKey),
		routerHostUserDirectory,
		strings.TrimSpace(publicKey),
		routerHostUserDirectory,
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
	logger.Debug("SSM command sent. Waiting for completion", "commandID", commandID)

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

// CheckSubnetNetworkAccess checks if the subnet has network access by checking routes
func CheckSubnetNetworkAccess(subnetID string) (bool, bool, error) {
	ec2Client, err := NewEC2Client(*config.App.Session.Config)
	if err != nil {
		return false, false, fmt.Errorf("failed to create EC2 client: %w", err)
	}

	// Get the route table associated with the subnet
	routeTablesOutput, err := ec2Client.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("association.subnet-id"),
				Values: []*string{aws.String(subnetID)},
			},
		},
	})
	if err != nil {
		return false, false, fmt.Errorf("error describing route tables: %w", err)
	}

	// Check if there is a route to an internet gateway or a NAT gateway
	for _, routeTable := range routeTablesOutput.RouteTables {
		for _, route := range routeTable.Routes {
			if route.GatewayId != nil && strings.HasPrefix(*route.GatewayId, "igw-") {
				return true, false, nil // Route to an internet gateway, public subnet
			}
			if route.NatGatewayId != nil {
				return true, true, nil // Route to a NAT gateway, private subnet
			}
		}
	}

	return false, false, nil
}

func GetSubnetsWithSSM() ([]*ec2.Subnet, error) {
	ec2Client, err := NewEC2Client(*config.App.Session.Config)
	if err != nil {
		return nil, err
	}

	input := &ec2.DescribeSubnetsInput{}

	var subnets []*ec2.Subnet
	err = ec2Client.DescribeSubnetsPages(input, func(page *ec2.DescribeSubnetsOutput, lastPage bool) bool {
		for _, subnet := range page.Subnets {
			hasAccess, _, err := CheckSubnetNetworkAccess(*subnet.SubnetId)
			if err != nil {
				logger.Error("Failed to check network access for subnet", "subnetID", *subnet.SubnetId, "error", err)
				continue
			}
			if hasAccess {
				subnets = append(subnets, subnet)
			}
		}
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
	if strings.Contains(config.App.Config.AWSEndpointUrl, "localhost") {
		logger.Debug("Skipping actual checking the instance to be ready in localstack, since it doesn't support it.")
		// wait 1 second to simulate the instance to be ready

		logger.Debug("Just Waiting 5 seconds for the instance to be ready")
		time.Sleep(5 * time.Second)
		return nil
	}

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

	timeout := time.After(5 * time.Minute)
	tick := time.Tick(10 * time.Second)

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for instance %s to be ready", instanceID)
		case <-tick:
			ssmOutput, err := ssmClient.DescribeInstanceInformation(ssmInput)
			if err != nil {
				return fmt.Errorf("error describing instance information: %w", err)
			}

			if len(ssmOutput.InstanceInformationList) > 0 && *ssmOutput.InstanceInformationList[0].PingStatus == "Online" {
				return nil
			}
		}
	}
}

// GetInstanceUsername retrieves the default SSH username for an EC2 instance.
func GetInstanceUsername(instanceID string) (string, error) {
	ec2Client, err := NewEC2Client(*config.App.Session.Config)
	if err != nil {
		logger.Error("Failed to create EC2 client", "error", err)
		return "", err
	}

	input := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{aws.String(instanceID)},
	}

	result, err := ec2Client.DescribeInstances(input)
	if err != nil {
		logger.Error("Failed to describe instance", "error", err)
		return "", err
	}

	if len(result.Reservations) == 0 || len(result.Reservations[0].Instances) == 0 {
		return "", fmt.Errorf("no instance found for ID %s", instanceID)
	}

	instance := result.Reservations[0].Instances[0]

	// Get AMI ID
	if instance.ImageId == nil {
		return "", fmt.Errorf("instance %s has no AMI ID", instanceID)
	}
	amiID := *instance.ImageId

	// Get AMI details
	amiInput := &ec2.DescribeImagesInput{
		ImageIds: []*string{aws.String(amiID)},
	}

	amiResult, err := ec2Client.DescribeImages(amiInput)
	if err != nil || len(amiResult.Images) == 0 {
		return "", fmt.Errorf("failed to retrieve AMI details for %s", amiID)
	}

	amiName := *amiResult.Images[0].Name

	// Determine default username based on AMI name
	username := detectUsernameFromAMI(amiName)
	if username == "" {
		return "", fmt.Errorf("could not determine default username for AMI %s", amiName)
	}

	return username, nil
}

// detectUsernameFromAMI infers the default SSH username based on AMI name.
func detectUsernameFromAMI(amiName string) string {
	switch {
	case contains(amiName, "ubuntu"):
		return "ubuntu"
	case contains(amiName, "amzn") || contains(amiName, "amazon"):
		return "ec2-user"
	case contains(amiName, "al2023") || contains(amiName, "amazon"):
		return "ec2-user"
	case contains(amiName, "rhel") || contains(amiName, "centos"):
		return "ec2-user"
	case contains(amiName, "debian"):
		return "admin" // Sometimes "debian"
	case contains(amiName, "suse") || contains(amiName, "opensuse"):
		return "ec2-user"
	case contains(amiName, "fedora"):
		return "fedora"
	default:
		return ""
	}
}

// contains checks if a string contains a substring (case-insensitive).
func contains(str, substr string) bool {
	return strings.Contains(strings.ToLower(str), strings.ToLower(substr))
}

// ConnectToSSMConsole connects to an EC2 instance using SSM opens an interactive shell
func ConnectToSSMConsole(instanceID string) error {
	// TODO: refactor to use only AWS SDK

	sessionCommand := exec.Command(
		"aws", "ssm", "start-session",
		"--target", instanceID,
		"--document-name", "AWS-StartInteractiveCommand",
		"--parameters", "command=/bin/bash",
	)

	sessionCommand.Stdout = os.Stdout
	sessionCommand.Stderr = os.Stderr
	sessionCommand.Stdin = os.Stdin

	if err := sessionCommand.Run(); err != nil {
		return fmt.Errorf("failed to start SSM session: %w", err)
	}

	return nil
}
