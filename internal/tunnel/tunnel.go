/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package tunnel

import (
	"encoding/json"
	"fmt"
	"github.com/automationd/atun/internal/aws"
	"github.com/automationd/atun/internal/config"
	"github.com/automationd/atun/internal/logger"
	"github.com/automationd/atun/internal/ssh"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pterm/pterm"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
)

// GetBastionHostID retrieves the Bastion Host ID from AWS tags.
// It takes a session, tag name, and tag value as parameters and returns the instance ID of the Bastion Host.
func GetBastionHostID() (string, error) {
	// First try to find bastion host id from the running process
	runningTunnels, err := ssh.GetRunningTunnels(config.App)
	logger.Debug("Running tunnels", "tunnels", runningTunnels)

	if len(runningTunnels) > 0 {
		// Get the bastion host ID from the running tunnels
		for _, tunnel := range runningTunnels {
			// Get the bastion host ID from the running tunnel
			bastionHostID := tunnel.BastionHostID
			if bastionHostID != "" {
				return bastionHostID, nil
			}
		}
	}

	logger.Debug("Getting bastion host ID. Looking for atun routers.")

	// Build a map of tags to filter instances
	tags := map[string]string{
		"atun.io/version": config.App.Version,
		"atun.io/env":     config.App.Config.Env,
	}

	instances, err := aws.ListInstancesWithTags(tags)
	if err != nil {
		logger.Debug("Error listing instances with tags", "tags", tags)
		return "", err
	}

	if len(instances) == 0 {
		err = fmt.Errorf("no instances found with required tags and in state RUNNING")
		logger.Debug("Error finding instances", "error", err, "tags", tags)
		return "", err
	}

	logger.Debug("Found instances", "instances", len(instances))

	for _, instance := range instances {
		logger.Debug("Found instance", "instance_id", *instance.InstanceId, "state", *instance.State.Name)

		// Use the first running instance found
		if *instance.InstanceId != "" && *instance.State.Name == "running" {
			return *instance.InstanceId, err
		}
	}

	return "", err
}

// GetBastionHostConfig Gets bastion host tags and unmarshalls it into a struct
func GetBastionHostConfig(bastionHostID string) (config.Atun, error) {
	// TODO:Implement logic:
	// - Get all tags from the host bastionHostID
	// - filter those that have atun.io
	// - unmarshal the tags into a struct

	// Use AWS SDK to get instance tags
	tags, err := aws.GetInstanceTags(bastionHostID)
	if err != nil {
		logger.Error("Error getting instance tags", "instance_id", bastionHostID, "error", err)
		return config.Atun{}, err // Return the error early
	}

	logger.Debug("Instance tags", "tags", tags)

	atun := config.Atun{
		Config: &config.Config{}, // Ensure nested structs are initialized
	}

	sshUser, err := aws.GetInstanceUsername(bastionHostID)
	if err != nil {
		logger.Error("Error getting instance username", "instance_id", bastionHostID, "error", err)
		return config.Atun{}, err
	}

	atun.Config.BastionHostUser = sshUser

	for k, v := range tags {
		// Iterate over the tags and use only atun.io tags
		if strings.HasPrefix(k, "atun.io") {
			// Add case conditional for the k, one is atun.io/version and the other is atun.io/host/*

			switch {
			case k == "atun.io/version":
				atun.Version = v
			case k == "atun.io/env":
				atun.Config.Env = v
			case strings.HasPrefix(k, "atun.io/host/"):

				var host config.Host

				err := json.Unmarshal([]byte(v), &host)
				if err != nil {
					logger.Error("Error unmarshalling host tags", "v", v, "host", host.Name, "error", err)
					continue
				}

				host.Name = strings.TrimPrefix(k, "atun.io/host/")

				// Allocate free local port dynamically if set to 0
				if host.Local == 0 {
					if config.App.Config.AutoAllocatePort {
						port, err := getFreePort()
						if err != nil {
							return config.Atun{}, err
						}
						host.Local = port
					} else {
						err = fmt.Errorf("can't allocate port %d", host.Local)
						return config.Atun{}, err
					}
				}

				// Append the host to the Hosts config
				atun.Config.Hosts = append(atun.Config.Hosts, host)
			}
		}
	}

	return atun, nil

}
func SetAWSCredentials(sess *session.Session) error {
	v, err := sess.Config.Credentials.Get()
	if err != nil {
		return fmt.Errorf("can't set AWS credentials: %w", err)
	}

	err = os.Setenv("AWS_SECRET_ACCESS_KEY", v.SecretAccessKey)
	if err != nil {
		return err
	}
	err = os.Setenv("AWS_ACCESS_KEY_ID", v.AccessKeyID)
	if err != nil {
		return err
	}
	err = os.Setenv("AWS_SESSION_TOKEN", v.SessionToken)
	if err != nil {
		return err
	}

	return nil
}

func StartTunnel(app *config.Atun) (bool, [][]string, error) {
	logger.Debug("Starting tunnel", "bastion", app.Config.BastionHostID, "SSHKeyPath", app.Config.SSHKeyPath, "SSHConfigFile", app.Config.SSHConfigFile, "env", app.Config.Env)

	if err := SetAWSCredentials(app.Session); err != nil {
		return false, nil, fmt.Errorf("can't start tunnel: %w", err)
	}

	// Check if tunnel already exists
	tunnelIsUp, connections, err := ssh.GetSSHTunnelStatus(app)
	if err != nil {
		return false, nil, fmt.Errorf("can't check tunnel: %w", err)
	}

	// If tunnel is not up
	if !tunnelIsUp {
		// Check if SSMPlugin is running
		ssmPluginIsRunning, err := ssh.GetSSMPluginStatus(app)
		if err != nil {
			return tunnelIsUp, nil, fmt.Errorf("can't check SSM plugin: %w", err)
		}

		if ssmPluginIsRunning {
			return tunnelIsUp, nil, fmt.Errorf("Tunnel is down but SSM plugin is already running with bastion host %s", app.Config.BastionHostID)
		}

		// Start SSH Tunnel
		err = ssh.StartSSHTunnel(app)
		if err != nil {
			return tunnelIsUp, nil, err
		}
	}
	// Check for status and collect connections again
	tunnelIsUp, connections, err = ssh.GetSSHTunnelStatus(app)
	return tunnelIsUp, connections, nil
}

// RenderTunnelStatusTable creates and renders a custom table with a given header and rows.
// It can be reused throughout the project.
func RenderTunnelStatusTable(status bool, rows [][]string) error {
	header := []string{"Remote (Cloud)", "Local", "Status"}
	// Print table title (optional)

	// If status is true set statusIcon to🟢else 🔴
	statusIcon := "🔴"
	if status {
		statusIcon = "🟢"
	}
	pterm.DefaultSection.Println("Tunnel " + statusIcon)
	if !status {
		pterm.FgGray.Println("Down")
		return nil
	}
	// Create a new table
	table := pterm.DefaultTable.WithHasHeader(true) // Table with a header row

	// Set table data (header + rows)
	data := append([][]string{header}, rows...) // Add header to the rows

	// Render the table
	err := table.WithData(data).Render()
	if err != nil {
		return err
	}

	return nil
}

// TODO: Fix auto-assign port logic
func getFreePort() (int, error) {
	// TODO: start from 50000 and find first free port
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("127.0.0.1:%d", 0))
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer func(l *net.TCPListener) {
		err := l.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(l)
	return l.Addr().(*net.TCPAddr).Port, nil
}

// CalculateLocalPort converts a remote port to a 5-digit local port.
// It prefixes "1" or "10" based on the port number.
// Calculate default Local port from defaultRemotePort. So 3306 would become 13306 and 5006 would become 15006. Take the port number and concat 1 or 10 to it so it becomes 5-digit
// 3306 -> 13306
// 5006 -> 15006
// 22 -> 10022
// 80 -> 10080
func CalculateLocalPort(remotePort int) (int, error) {
	if remotePort <= 0 || remotePort > 65535 {
		return 0, fmt.Errorf("invalid port number: %d", remotePort)
	}

	// Convert to string to check the length
	remotePortStr := strconv.Itoa(remotePort)

	// Add prefixes to make it a 5-digit number
	if len(remotePortStr) == 4 {
		// Prefix "1" for 4-digit ports
		return strconv.Atoi("1" + remotePortStr)
	} else if len(remotePortStr) <= 3 {
		// Prefix "100" for 3-digit or smaller ports
		return strconv.Atoi("100" + remotePortStr)
	}

	// If already 5 digits or larger (unlikely), return as is
	return remotePort, nil
}

// TODO: Fix checkPort logic
//func checkPort(port int, dir string) error {
//	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("127.0.0.1:%d", port))
//	if err != nil {
//		return fmt.Errorf("can't check address %s: %w", fmt.Sprintf("127.0.0.1:%d", port), err)
//	}
//
//	l, err := net.ListenTCP("tcp", addr)
//	if err != nil {
//		command := fmt.Sprintf("lsof -i tcp:%d | grep LISTEN | awk '{print $1, $2}'", port)
//		stdout, stderr, code, err := term.New(term.WithStdout(io.Discard), term.WithStderr(io.Discard)).Run(exec.Command("bash", "-c", command))
//		if err != nil {
//			return fmt.Errorf("can't run command '%s': %w", command, err)
//		}
//		if code == 0 {
//			stdout = strings.TrimSpace(stdout)
//			processName := strings.Split(stdout, " ")[0]
//			processPid, err := strconv.Atoi(strings.Split(stdout, " ")[1])
//			if err != nil {
//				return fmt.Errorf("can't get pid: %w", err)
//			}
//			pterm.Info.Printfln("Can't start tunnel on port %d. It seems like it's take by a process '%s'.", port, processName)
//			proc, err := os.FindProcess(processPid)
//			if err != nil {
//				return fmt.Errorf("can't find process: %w", err)
//			}
//
//			_, err = os.Stat(filepath.Join(dir, "bastion.sock"))
//			if processName == "ssh" && os.IsNotExist(err) {
//				return fmt.Errorf("it could be another ize tunnel, but we can't find a socket. Something went wrong. We suggest terminating it and starting it up again")
//			}
//			isContinue := false
//			if terminal.IsTerminal(int(os.Stdout.Fd())) {
//				isContinue, err = pterm.DefaultInteractiveConfirm.WithDefaultText("Would you like to terminate it?").Show()
//				if err != nil {
//					return err
//				}
//			} else {
//				isContinue = true
//			}
//
//			if !isContinue {
//				return fmt.Errorf("destroying was canceled")
//			}
//			err = proc.Kill()
//			if err != nil {
//				return fmt.Errorf("can't kill process: %w", err)
//			}
//
//			pterm.Info.Printfln("Process '%s' (pid %d) was killed", processName, processPid)
//
//			return nil
//		}
//		return fmt.Errorf("error during run command: %s (exit code: %d, stderr: %s)", command, code, stderr)
//	}
//
//	err = l.Close()
//	if err != nil {
//		return err
//	}
//
//	return nil
//}
