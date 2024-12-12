/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package e2e

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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

// TestAtunCreateDelete performs the full lifecycle test
func TestAtunCreateDelete(t *testing.T) {
	ctx := context.Background()

	// Start LocalStack Container
	localstackAuthToken := getLocalStackAuthToken(t)
	localstackContainer := startLocalStack(ctx, t, localstackAuthToken)
	defer terminateContainer(ctx, localstackContainer)

	// Retrieve LocalStack Endpoint
	endpoint := getContainerEndpoint(ctx, t, localstackContainer)
	t.Logf("LocalStack endpoint: %s", endpoint)

	// Setup Temporary AWS Profile
	configPath, credentialsPath := setupAWSProfile(t)
	setAWSEnvVars(configPath, credentialsPath, endpoint)

	ec2Client := createAWSClient(t, endpoint, credentialsPath)
	t.Logf("AWS EC2 client created successfully")

	// Step 4: Create VPC and Subnet
	vpcID := createVPC(t, ec2Client, mockVpcCidr, mockVpcTag)
	subnetID := createSubnet(t, ec2Client, vpcID, mockSubnetTag)
	t.Logf("VPC and Subnet created successfully %s, %s", vpcID, subnetID)

	// Prepare Work Directory
	workDir := prepareWorkDir(t, subnetID)

	// Run `atun create`
	runAtunCommand(t, workDir, "create")

	// Verify EC2 Instance Exists
	instanceID := verifyEC2Instance(t, ec2Client, "atun.io/version", "1")
	t.Logf("EC2 instance created successfully with ID: %s", instanceID)
	//
	// Run `atun delete`
	runAtunCommand(t, workDir, "delete")

	// Verify EC2 Instance Removed
	verifyInstanceDeleted(t, ec2Client, instanceID)

	t.Log("Test completed successfully")
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
func prepareWorkDir(t *testing.T, subnetID string) string {
	tmpDir, _ := os.MkdirTemp("", "atun")
	content := fmt.Sprintf(`
aws_profile = "localstack"
bastion_subnet_id = "%s"
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
	`, subnetID)
	filePath := filepath.Join(tmpDir, testAtunConfigFile)
	_ = os.WriteFile(filePath, []byte(content), 0644)
	return tmpDir
}

// runAtunCommand runs the Atun CLI
func runAtunCommand(t *testing.T, workDir, command string) {

	cmd := exec.Command("atun", command)
	cmd.Dir = workDir
	t.Logf("Running `atun %s` in %s", command, workDir)

	cmd.Env = append(os.Environ(),
		fmt.Sprintf("AWS_PROFILE=%s", os.Getenv("AWS_PROFILE")),
		fmt.Sprintf("AWS_CONFIG_FILE=%s", os.Getenv("AWS_CONFIG_FILE")),
		fmt.Sprintf("AWS_SHARED_CREDENTIALS_FILE=%s", os.Getenv("AWS_SHARED_CREDENTIALS_FILE")),
		fmt.Sprintf("AWS_REGION=%s", os.Getenv("AWS_REGION")),
		fmt.Sprintf("AWS_ENDPOINT_URL=%s", os.Getenv("AWS_ENDPOINT_URL")),
		"ATUN_LOG_LEVEL=debug",
	)

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("`atun %s` failed: %v\nOutput: %s", command, err, string(output))
	} else {
		t.Logf("`atun %s` ran: \nOutput: %s", command, string(output))
	}

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
