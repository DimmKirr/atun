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
	"log"
	"net"
	"os"
	"strings"
)

// GetBastionHostID retrieves the Bastion Host ID from AWS tags.
// It takes a session, tag name, and tag value as parameters and returns the instance ID of the Bastion Host.
func GetBastionHostID() (string, error) {
	logger.Debug("Getting bastion host ID. Looking for atun routers.")

	// Build a map of tags to filter instances
	tags := map[string]string{
		"atun.io/version": config.App.Version,
		"atun.io/env":     config.App.Config.Env,
	}

	instances, err := aws.ListInstancesWithTags(tags)
	if err != nil {
		logger.Error("Error listing instances with tags", "tags", tags)
		return "", err
	}

	if len(instances) == 0 {
		err = fmt.Errorf("no instances found with required tags and in state RUNNING")
		logger.Error("Error finding instances", "error", err, "tags", tags)
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

func StartTunnel(app *config.Atun) (string, error) {
	logger.Debug("Starting tunnel", "bastion", app.Config.BastionHostID, "SSHKeyPath", app.Config.SSHKeyPath, "SSHConfigFile", app.Config.SSHConfigFile, "env", app.Config.Env)

	if err := SetAWSCredentials(app.Session); err != nil {
		return "", fmt.Errorf("can't start tunnel: %w", err)
	}

	// Check if tunnel already exists
	tunnelIsUp, err := ssh.GetTunnelStatus(app)
	if err != nil {
		return "", fmt.Errorf("can't check tunnel: %w", err)
	}

	// If tunnel is not up
	if !tunnelIsUp {
		// Check if SSMPlugin is running
		ssmPluginIsRunning, err := ssh.GetSSMPluginStatus(app)
		if err != nil {
			return "", fmt.Errorf("can't check SSM plugin: %w", err)
		}

		if ssmPluginIsRunning {
			return "", fmt.Errorf("Tunnel is down but SSM plugin is already running with bastion host %s", app.Config.BastionHostID)
		}

		// If tunnel exists show message it's already up and show path to socket and forwarding config

		// If tunnel doesn't exist, start a new one.

		// Get the SSH command arguments
		args := ssh.GetSSHCommandArgs(app)

		// TODO: Refactor without RunSSH, but instead have a dedicated fnction
		// Run the SSH command
		err = ssh.RunSSH(app, args)
		if err != nil {
			return "", err
		}
	}

	var connectionInfo string

	for _, v := range config.App.Config.Hosts {
		logger.Debug("Host", "name", v.Name, "proto", v.Proto, "remote", v.Remote, "local", v.Local)
		connectionInfo += fmt.Sprintf("%s:%d ➡ 127.0.0.1:%d\n", v.Name, v.Remote, v.Local)
	}

	return connectionInfo, nil
}

// TODO: Fix auto-assign port logic
func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
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
