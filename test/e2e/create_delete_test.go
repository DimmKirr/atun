/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

// Constants for LocalStack and AWS
const (
	localstackImage    = "localstack/localstack-pro:4.0.3"
	localstackPort     = "4566/tcp"
	testAwsProfile     = "localstack"
	testAwsRegion      = "us-east-1"
	mockVpcCidr        = "10.0.0.0/16"
	mockVpcTag         = "mock-vpc-55555"
	mockSubnetTag      = "mock-subnet-55555"
	testAtunConfigFile = "atun.toml"
	localstackReadyLog = "Ready."
)

// testSetup contains the common setup for all tests
type testSetup struct {
	ctx             context.Context
	localstack      testcontainers.Container
	endpoint        string
	ec2Client       *ec2.EC2
	vpcID           string
	subnetID        string
	workDir         string
	configPath      string
	credentialsPath string
}

// setupTestEnvironment initializes the test environment
func setupTestEnvironment(t *testing.T) *testSetup {
	ctx := context.Background()

	// Start LocalStack Container
	localstackAuthToken := getLocalStackAuthToken(t)
	localstackContainer := startLocalStack(ctx, t, localstackAuthToken)

	// Retrieve LocalStack Endpoint
	endpoint := getContainerEndpoint(ctx, t, localstackContainer)
	t.Logf("LocalStack endpoint: %s", endpoint)

	// Setup Temporary AWS Profile
	configPath, credentialsPath := setupAWSProfile(t)
	setAWSEnvVars(configPath, credentialsPath, endpoint)

	ec2Client := createAWSClient(t, endpoint, credentialsPath)
	t.Logf("AWS EC2 client created successfully")

	// Create VPC and Subnet
	vpcID := createVPC(t, ec2Client, mockVpcCidr, mockVpcTag)
	subnetID := createSubnet(t, ec2Client, vpcID, mockSubnetTag)
	t.Logf("VPC and Subnet created successfully %s, %s", vpcID, subnetID)

	// Prepare Work Directory
	amiID := getLatestAMI(t, "al2023-ami-2023")
	workDir := prepareWorkDir(t, subnetID, amiID)

	return &testSetup{
		ctx:             ctx,
		localstack:      localstackContainer,
		endpoint:        endpoint,
		ec2Client:       ec2Client,
		vpcID:           vpcID,
		subnetID:        subnetID,
		workDir:         workDir,
		configPath:      configPath,
		credentialsPath: credentialsPath,
	}
}

// cleanupTestEnvironment cleans up the test environment
func (s *testSetup) cleanupTestEnvironment(t *testing.T) {
	terminateContainer(s.ctx, s.localstack)
}

// runAtunCommand is a helper function to run Atun commands with consistent environment setup
func runAtunCommand(t *testing.T, workDir, command string, interactive bool, envVars map[string]string) {
	cmd := exec.Command("atun", command)
	cmd.Dir = workDir

	// Set up environment variables
	for key, value := range envVars {
		if value == "" {
			os.Unsetenv(key)
		} else {
			os.Setenv(key, value)
		}
	}

	// Set up command environment
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("AWS_PROFILE=%s", os.Getenv("AWS_PROFILE")),
		fmt.Sprintf("AWS_CONFIG_FILE=%s", os.Getenv("AWS_CONFIG_FILE")),
		fmt.Sprintf("AWS_SHARED_CREDENTIALS_FILE=%s", os.Getenv("AWS_SHARED_CREDENTIALS_FILE")),
		fmt.Sprintf("AWS_REGION=%s", os.Getenv("AWS_REGION")),
		fmt.Sprintf("AWS_ENDPOINT_URL=%s", os.Getenv("AWS_ENDPOINT_URL")),
		"ATUN_LOG_LEVEL=debug",
	)

	// Set up non-interactive stdin if needed
	if !interactive {
		cmd.Stdin = strings.NewReader("")
	}

	t.Logf("Running `atun %s` in %s with interactive=%v", command, workDir, interactive)

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("`atun %s` failed: %v\nOutput: %s", command, err, string(output))
	} else {
		t.Logf("`atun %s` ran: \nOutput: %s", command, string(output))
	}
}

// TestAtunCreateDelete tests the create/delete flow in both interactive and non-interactive modes
func TestAtunCreateDelete(t *testing.T) {
	tests := []struct {
		name           string
		interactive    bool
		expectedOutput string
		envVars        map[string]string
	}{
		{
			name:           "Interactive terminal",
			interactive:    true,
			expectedOutput: "Creating Ad-Hoc EC2 Bastion Instance...",
			envVars: map[string]string{
				"TERM":                "xterm-256color",
				"NO_COLOR":            "",
				"CLICOLOR":            "1",
				"CLICOLOR_FORCE":      "1",
				"ATUN_LOG_PLAIN_TEXT": "false",
			},
		},
		{
			name:           "Non-interactive terminal",
			interactive:    false,
			expectedOutput: "Creating Ad-Hoc EC2 Bastion Instance...",
			envVars: map[string]string{
				"TERM":           "",
				"NO_COLOR":       "1",
				"CLICOLOR":       "0",
				"CLICOLOR_FORCE": "0",
				"CI":             "true",
				"GITHUB_ACTIONS": "true",
			},
		},
	}

	setup := setupTestEnvironment(t)
	defer setup.cleanupTestEnvironment(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run create command
			runAtunCommand(t, setup.workDir, "create", tt.interactive, tt.envVars)

			// Verify EC2 Instance Exists
			instanceID := verifyEC2Instance(t, setup.ec2Client, "atun.io/version", "1")
			t.Logf("EC2 instance created successfully with ID: %s", instanceID)

			// Run delete command
			runAtunCommand(t, setup.workDir, "delete", tt.interactive, tt.envVars)

			// Verify EC2 Instance Removed
			verifyInstanceDeleted(t, setup.ec2Client, instanceID)

			t.Logf("%s test completed successfully", tt.name)
		})
	}
}

// ---------------- Utility Functions ----------------

// setupAWSProfile creates a temporary AWS profile
func setupAWSProfile(t *testing.T) (string, string) {
	tmpDir, _ := os.MkdirTemp("", "awsconfig")
	credentialsPath := filepath.Join(tmpDir, "credentials")
	configPath := filepath.Join(tmpDir, "config")

	// Write credentials
	_ = os.WriteFile(credentialsPath, []byte(`[localstack]
aws_access_key_id = test
aws_secret_access_key = test`), 0644)
	t.Logf("AWS credentials created successfully in %s", credentialsPath)
	// Write config
	_ = os.WriteFile(configPath, []byte(`[profile localstack]
region = us-east-1
output = json`), 0644)
	t.Logf("AWS profile created successfully in %s", configPath)
	return configPath, credentialsPath
}

// setAWSEnvVars sets AWS environment variables
func setAWSEnvVars(configPath, credentialsPath, endpoint string) {
	os.Setenv("AWS_PROFILE", testAwsProfile)
	os.Setenv("AWS_CONFIG_FILE", configPath)
	os.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsPath)
	os.Setenv("AWS_REGION", testAwsRegion)
	os.Setenv("AWS_ENDPOINT_URL", endpoint)
}

// getLocalStackAuthToken retrieves LocalStack API token
func getLocalStackAuthToken(t *testing.T) string {
	token := os.Getenv("LOCALSTACK_AUTH_TOKEN")
	if token == "" {
		t.Fatalf("LOCALSTACK_AUTH_TOKEN is not set")
	}
	return token
}

// startLocalStack starts the LocalStack container with appropriate port bindings and configurations.
func startLocalStack(ctx context.Context, t *testing.T, authToken string) testcontainers.Container {
	// Define LocalStack ports to expose
	ports := []string{
		"4566/tcp", // LocalStack main port
		"443/tcp",  // HTTPS port
	}

	// Add individual ports for the range 4510-4559
	for i := 4510; i <= 4559; i++ {
		ports = append(ports, fmt.Sprintf("%d/tcp", i))
	}

	// Define port bindings dynamically
	portBindings := make(map[nat.Port][]nat.PortBinding)
	for _, p := range ports {
		port := nat.Port(p)
		portBindings[port] = []nat.PortBinding{
			{HostIP: "127.0.0.1", HostPort: port.Port()},
		}
	}

	// Define cnt request
	req := testcontainers.ContainerRequest{
		Image:        localstackImage,
		ExposedPorts: ports,
		Env: map[string]string{
			"LOCALSTACK_AUTH_TOKEN": authToken,
		},
		Mounts: testcontainers.Mounts(
			testcontainers.BindMount("/var/run/docker.sock", "/var/run/docker.sock"),
		),
		WaitingFor: wait.ForLog(localstackReadyLog),
		HostConfigModifier: func(hc *container.HostConfig) {
			hc.PortBindings = portBindings

		},
	}

	// Start the container
	cnt, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start LocalStack cnt: %v", err)
	}

	return cnt
}

// terminateContainer ensures the container is terminated
func terminateContainer(ctx context.Context, container testcontainers.Container) {
	_ = container.Terminate(ctx)
}

// getContainerEndpoint retrieves LocalStack endpoint
func getContainerEndpoint(ctx context.Context, t *testing.T, container testcontainers.Container) string {
	endpoint, err := container.PortEndpoint(ctx, localstackPort, "http")
	if err != nil {
		t.Fatalf("Failed to get endpoint: %v", err)
	}
	return endpoint
}

// createAWSClient initializes AWS EC2 client
func createAWSClient(t *testing.T, endpoint, credentialsPath string) *ec2.EC2 {
	cfg := &aws.Config{
		Region:      aws.String(testAwsRegion),
		Endpoint:    aws.String(endpoint),
		Credentials: credentials.NewSharedCredentials(credentialsPath, testAwsProfile),
		DisableSSL:  aws.Bool(true),
	}
	sess, _ := session.NewSession(cfg)
	return ec2.New(sess)
}

// createVPC creates a VPC with tags
func createVPC(t *testing.T, ec2Client *ec2.EC2, cidr, tag string) string {
	vpc, err := ec2Client.CreateVpc(&ec2.CreateVpcInput{
		CidrBlock: aws.String(cidr),
		TagSpecifications: []*ec2.TagSpecification{{
			ResourceType: aws.String("vpc"),
			Tags:         []*ec2.Tag{{Key: aws.String("Name"), Value: aws.String(tag)}},
		}},
	})
	if err != nil {
		t.Fatalf("Failed to create VPC: %v", err)
	}
	return *vpc.Vpc.VpcId
}

// createSubnet creates a subnet
func createSubnet(t *testing.T, ec2Client *ec2.EC2, vpcID, tag string) string {
	subnet, err := ec2Client.CreateSubnet(&ec2.CreateSubnetInput{
		VpcId:     aws.String(vpcID),
		CidrBlock: aws.String("10.0.1.0/24"),
		TagSpecifications: []*ec2.TagSpecification{{
			ResourceType: aws.String("subnet"),
			Tags:         []*ec2.Tag{{Key: aws.String("Name"), Value: aws.String(tag)}},
		}},
	})
	if err != nil {
		t.Fatalf("Failed to create subnet: %v", err)
	}
	return *subnet.Subnet.SubnetId
}

// prepareWorkDir generates a TOML file for Atun
func prepareWorkDir(t *testing.T, subnetID string, amiID string) string {
	tmpDir, _ := os.MkdirTemp("", "atun")
	content := fmt.Sprintf(`
aws_profile = "localstack"
bastion_subnet_id = "%s"
bastion_host_ami = "%s"
[[hosts]]
name = "ipconfig.io"
proto = "ssm"
remote = "80"
local = "10080"

[[hosts]]
name = "icanhazip.com"
proto = "ssm"
remote = "80"
local = "10081"
	`, subnetID, amiID)
	filePath := filepath.Join(tmpDir, testAtunConfigFile)
	_ = os.WriteFile(filePath, []byte(content), 0644)
	return tmpDir
}

// verifyEC2Instance checks that an EC2 instance with the specified tag exists and returns its instance ID.
func verifyEC2Instance(t *testing.T, ec2Client *ec2.EC2, tagKey, tagValue string) string {
	input := &ec2.DescribeInstancesInput{}

	instances, err := ec2Client.DescribeInstances(input)
	if err != nil {
		t.Fatalf("Failed to describe instances: %v", err)
	}

	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			for _, tag := range instance.Tags {
				if aws.StringValue(tag.Key) == tagKey && aws.StringValue(tag.Value) == tagValue {
					t.Logf("EC2 instance found with tag %s:%s, Instance ID: %s", tagKey, tagValue, *instance.InstanceId)
					return *instance.InstanceId
				}
			}
		}
	}

	t.Fatalf("EC2 instance with tag %s:%s not found", tagKey, tagValue)
	return ""
}

// verifyInstanceDeleted ensures the EC2 instance with the specified ID no longer exists.
func verifyInstanceDeleted(t *testing.T, ec2Client *ec2.EC2, instanceID string) {
	input := &ec2.DescribeInstancesInput{}

	instances, err := ec2Client.DescribeInstances(input)
	if err != nil {
		t.Fatalf("Failed to describe instances: %v", err)
	}

	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if aws.StringValue(instance.InstanceId) == instanceID && aws.StringValue(instance.State.Name) == ec2.InstanceStateNameRunning {
				t.Fatalf("EC2 instance with ID %s still exists when it should have been removed", instanceID)
			}
		}
	}

	t.Logf("EC2 instance with ID %s has been successfully removed", instanceID)
}

// getLatestAMI fetches the latest AMI ID for a given family
func getLatestAMI(t *testing.T, family string) string {
	// Create EC2 client
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(testAwsRegion),
		Endpoint:    aws.String(os.Getenv("AWS_ENDPOINT_URL")),
		Credentials: credentials.NewSharedCredentials(os.Getenv("AWS_SHARED_CREDENTIALS_FILE"), testAwsProfile),
		DisableSSL:  aws.Bool(true),
	}))
	ec2Client := ec2.New(sess)

	// Detect current architecture
	arch := "x86_64"
	if runtime.GOARCH == "arm64" {
		arch = "arm64"
	}

	// Describe images with filters
	input := &ec2.DescribeImagesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("name"),
				Values: []*string{aws.String("al2023-ami-*")},
			},
			{
				Name:   aws.String("state"),
				Values: []*string{aws.String("available")},
			},
			{
				Name:   aws.String("architecture"),
				Values: []*string{aws.String(arch)},
			},
		},
		Owners: []*string{aws.String("amazon")},
	}

	result, err := ec2Client.DescribeImages(input)
	if err != nil {
		t.Fatalf("Failed to describe images: %v", err)
	}

	if len(result.Images) == 0 {
		t.Fatalf("No images found for family: %s and architecture: %s", family, arch)
	}

	// Sort images by creation date to get the latest
	sort.Slice(result.Images, func(i, j int) bool {
		timeI, _ := time.Parse(time.RFC3339, *result.Images[i].CreationDate)
		timeJ, _ := time.Parse(time.RFC3339, *result.Images[j].CreationDate)
		return timeI.After(timeJ)
	})

	return *result.Images[0].ImageId
}
