/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package tunnel

import (
	"encoding/json"
	"fmt"
	"github.com/DimmKirr/atun/internal/aws"
	"github.com/DimmKirr/atun/internal/config"
	"github.com/DimmKirr/atun/internal/logger"
	"github.com/DimmKirr/atun/internal/ssh"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"log"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// GetRouterHostIDFromTags retrieves the Router Endpoint ID from AWS tags.
// It takes a session, tag name, and tag value as parameters and returns the instance ID of the Router Endpoint.
func GetRouterHostIDFromTags() (string, error) {
	// First try to find router host id from the running processes, so a router that already
	// has a live local tunnel is always preferred over any other candidate.
	activeSSHTunnels, err := ssh.GetActiveSSHTunnels()
	if err != nil {
		logger.Debug("Error getting running tunnels", "error", err)
		return "", err
	}

	logger.Debug("Running tunnels", "tunnels", activeSSHTunnels)

	activeTunnelRouterIDs := make(map[string]bool, len(activeSSHTunnels))
	for _, v := range activeSSHTunnels {
		activeTunnelRouterIDs[v.RouterHostID] = true
	}

	logger.Debug("Getting router host ID. Looking for atun routers.")

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

	return selectRouterInstance(instances, activeTunnelRouterIDs)
}

// selectRouterInstance deterministically picks one router instance out of the candidates
// returned by AWS tag-based discovery. AWS's DescribeInstances does not guarantee stable
// ordering, so relying on "first in the list" can make repeated `atun up` invocations
// resolve to different physical instances when more than one candidate matches the
// discovery tags. Multiple simultaneous candidates are expected, not just a leak: routers
// can be managed by an ASG for redundancy, so more than one may legitimately be running
// at once, including mid-scaling-event.
//
// Selection order:
//  1. A candidate with an already-active local tunnel always wins, so an in-use router
//     session is never abandoned mid-use in favor of some other matching instance.
//  2. Otherwise, the oldest running candidate wins (deterministic tiebreak). Preferring
//     the oldest over the newest matters specifically for ASG-managed routers: a freshly
//     launched instance can report State=running before its networking (security groups,
//     route tables, ENI attachment) has finished propagating, so preferring the newest
//     instance systematically prefers the least-proven candidate during any scaling event.
//     The oldest instance has demonstrated it works.
//
// If more than one running candidate exists, a warning is logged so operators get
// visibility into which candidates are in play instead of silent, effectively-random selection.
func selectRouterInstance(instances []*ec2.Instance, activeTunnelRouterIDs map[string]bool) (string, error) {
	var running []*ec2.Instance
	for _, instance := range instances {
		if instance.InstanceId == nil || instance.State == nil || instance.State.Name == nil {
			continue
		}
		if *instance.State.Name == "running" && *instance.InstanceId != "" {
			running = append(running, instance)
		}
	}

	if len(running) == 0 {
		return "", fmt.Errorf("no instances found with required tags and in state RUNNING")
	}

	sort.SliceStable(running, func(i, j int) bool {
		ti, tj := running[i].LaunchTime, running[j].LaunchTime
		if ti == nil || tj == nil {
			return false
		}
		return ti.Before(*tj)
	})

	if len(running) > 1 {
		ids := make([]string, len(running))
		for i, instance := range running {
			ids[i] = *instance.InstanceId
		}
		logger.Warn("Multiple router instances match the discovery tags; selecting deterministically", "candidates", ids)
	}

	for _, instance := range running {
		if activeTunnelRouterIDs[*instance.InstanceId] {
			logger.Debug("Preferring instance with an active local tunnel", "instance_id", *instance.InstanceId)
			return *instance.InstanceId, nil
		}
	}

	return *running[0].InstanceId, nil
}

// GetRouterHostConfig Gets router host tags and unmarshalls it into a struct
func GetRouterHostConfig(routerHostID string) (config.Atun, error) {
	// TODO:Implement logic:
	// - Get all tags from the host routerHostID
	// - filter those that have atun.io
	// - unmarshal the tags into a struct

	// Use AWS SDK to get instance tags
	tags, err := aws.GetInstanceTags(routerHostID)
	if err != nil {
		logger.Error("Error getting instance tags", "instance_id", routerHostID, "error", err)
		return config.Atun{}, err // Return the error early
	}

	logger.Debug("Instance tags", "tags", tags)

	atun := config.Atun{
		Config: &config.Config{}, // Ensure nested structs are initialized
	}

	sshUser, err := aws.GetInstanceUsername(routerHostID)
	if err != nil {
		logger.Error("Error getting instance username", "instance_id", routerHostID, "error", err)
		return config.Atun{}, err
	}

	atun.Config.RouterHostUser = sshUser

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

				var endpoint config.Endpoint

				err := json.Unmarshal([]byte(v), &endpoint)
				if err != nil {
					logger.Error("Error unmarshalling host tags", "v", v, "host", endpoint.Name, "error", err)
					continue
				}

				endpoint.Name = strings.TrimPrefix(k, "atun.io/host/")

				// Allocate free local port dynamically if set to 0
				if endpoint.Local == 0 {
					if config.App.Config.AutoAllocatePort {
						port, err := getFreePort()
						if err != nil {
							return config.Atun{}, err
						}
						endpoint.Local = port
					} else {
						err = fmt.Errorf("can't allocate port %d", endpoint.Local)
						return config.Atun{}, err
					}
				}

				// Append the host to the Hosts config
				atun.Config.Hosts = append(atun.Config.Hosts, endpoint)
			}
		}
	}

	return atun, nil

}

// SetAWSCredentials sets AWS credentials as environment variables
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

const (
	// tunnelReadinessTimeout bounds how long ActivateTunnel waits for a freshly-started tunnel's
	// endpoints to become genuinely reachable before giving up and returning whatever state was
	// last observed. SSM session establishment (API call, WebSocket handshake, session-manager-
	// plugin cold start, sshd negotiation on the router) typically takes a few seconds but can
	// stretch further on a cold CI runner.
	tunnelReadinessTimeout = 15 * time.Second
	// tunnelReadinessPollInterval is how often readiness is re-checked while waiting.
	tunnelReadinessPollInterval = 500 * time.Millisecond
	// tunnelReadinessStableChecks requires readiness to hold across this many consecutive polls
	// before declaring the tunnel ready, since a half-negotiated channel can briefly appear open.
	tunnelReadinessStableChecks = 2
	// tunnelReadinessProbeHold is how long ProbeEndpointReachability holds each connection open
	// waiting to see if it gets torn down by a failed remote channel open.
	tunnelReadinessProbeHold = 300 * time.Millisecond
)

// ActivateTunnel starts the SSH tunnel and SSM plugin
func ActivateTunnel(app *config.Atun) (bool, []ssh.Endpoint, error) {
	logger.Debug("Starting tunnel", "router", app.Config.RouterHostID, "SSHKeyPath", app.Config.SSHKeyPath, "SSHConfigFile", app.Config.SSHConfigFile, "env", app.Config.Env)

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
			return tunnelIsUp, nil, fmt.Errorf("Tunnel is down but SSM plugin is already running with router host %s", app.Config.RouterHostID)
		}

		// Start SSH Tunnel
		err = ssh.StartSSHTunnel(app)
		if err != nil {
			return tunnelIsUp, nil, err
		}

		// The local LocalForward socket binds almost immediately, well before the SSM data
		// channel and the router's own connection to the real remote target (e.g. an RDS
		// instance inside a VPC) have finished negotiating. Poll until every endpoint's SSH
		// channel is genuinely open instead of trusting the very first check.
		return waitForTunnelReady(func() (bool, bool, []ssh.Endpoint, error) {
			up, endpoints, statusErr := ssh.GetSSHTunnelStatus(app)
			if statusErr != nil {
				return false, up, endpoints, statusErr
			}
			ready := up && allEndpointsReachable(endpoints)
			return ready, up, endpoints, nil
		}, tunnelReadinessTimeout, tunnelReadinessPollInterval, tunnelReadinessStableChecks)
	}
	// Check for status and collect connections again
	tunnelIsUp, connections, err = ssh.GetSSHTunnelStatus(app)
	return tunnelIsUp, connections, nil
}

// allEndpointsReachable verifies every locally-listening endpoint has a genuinely working SSH
// channel to its remote destination, using ProbeEndpointReachability rather than trusting the
// plain local-port check already folded into each Endpoint's Status.
func allEndpointsReachable(endpoints []ssh.Endpoint) bool {
	if len(endpoints) == 0 {
		return false
	}
	for _, endpoint := range endpoints {
		if !endpoint.Status {
			return false
		}
		reachable, err := ssh.ProbeEndpointReachability(endpoint.LocalPort, tunnelReadinessProbeHold)
		if err != nil || !reachable {
			return false
		}
	}
	return true
}

// waitForTunnelReady polls pollOnce until it reports ready for stableChecks consecutive polls, or
// timeout elapses. Readiness must hold for consecutive polls (not just once) because a
// half-negotiated SSH channel can briefly appear open before being torn down. Returns the last
// known state either way on timeout — the caller's existing status reporting surfaces any
// endpoints that never became ready, rather than this function treating a timeout as fatal.
func waitForTunnelReady(
	pollOnce func() (ready bool, up bool, endpoints []ssh.Endpoint, err error),
	timeout time.Duration,
	pollInterval time.Duration,
	stableChecks int,
) (bool, []ssh.Endpoint, error) {
	deadline := time.Now().Add(timeout)
	consecutive := 0

	for {
		ready, up, endpoints, err := pollOnce()

		if err == nil && ready {
			consecutive++
			if consecutive >= stableChecks {
				return up, endpoints, nil
			}
		} else {
			consecutive = 0
		}

		if time.Now().After(deadline) {
			return up, endpoints, err
		}
		time.Sleep(pollInterval)
	}
}

// DeactivateTunnel stops the SSH tunnel and SSM plugin
func DeactivateTunnel(app *config.Atun) (bool, error) {
	tunnelActive, err := ssh.StopSSHTunnel(app)
	if err != nil {
		return false, err
	}

	err = ssh.TerminateSSHProcessesWithRouterHostID(app.Config.RouterHostID)
	if err != nil {
		logger.Debug("Can't terminate SSH processes", "error", err)
	}

	err = ssh.TerminateSSMProcessesWithRouterHostID(app.Config.RouterHostID)
	if err != nil {
		logger.Debug("Can't terminate SSM processes", "error", err)
	}

	// Re-check status
	tunnelActive, _, err = ssh.GetSSHTunnelStatus(app)

	return tunnelActive, nil
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
//			_, err = os.Stat(filepath.Join(dir, "router.sock"))
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
