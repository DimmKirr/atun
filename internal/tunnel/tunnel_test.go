/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package tunnel

import (
	"fmt"
	"testing"
	"time"

	"github.com/DimmKirr/atun/internal/ssh"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func runningInstance(id string, launchTime time.Time) *ec2.Instance {
	return &ec2.Instance{
		InstanceId: aws.String(id),
		State:      &ec2.InstanceState{Name: aws.String("running")},
		LaunchTime: aws.Time(launchTime),
	}
}

func TestSelectRouterInstance_SingleCandidateUnchanged(t *testing.T) {
	instances := []*ec2.Instance{
		runningInstance("i-0000000000000000a", time.Unix(100, 0)),
	}

	id, err := selectRouterInstance(instances, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "i-0000000000000000a" {
		t.Errorf("expected i-0000000000000000a, got %s", id)
	}
}

func TestSelectRouterInstance_PicksOldestRunningCandidate(t *testing.T) {
	instances := []*ec2.Instance{
		runningInstance("i-new0000000000000", time.Unix(200, 0)),
		runningInstance("i-old0000000000000", time.Unix(100, 0)),
	}

	id, err := selectRouterInstance(instances, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "i-old0000000000000" {
		t.Errorf("expected the oldest running instance (proven-healthy over freshly-launched), got %s", id)
	}
}

func TestSelectRouterInstance_PrefersActiveTunnelOverNewerCandidate(t *testing.T) {
	instances := []*ec2.Instance{
		runningInstance("i-active0000000000", time.Unix(100, 0)),
		runningInstance("i-newer00000000000", time.Unix(999, 0)),
	}
	activeTunnelRouterIDs := map[string]bool{"i-active0000000000": true}

	id, err := selectRouterInstance(instances, activeTunnelRouterIDs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "i-active0000000000" {
		t.Errorf("expected the instance with an active local tunnel to win, got %s", id)
	}
}

func TestSelectRouterInstance_NoRunningCandidatesReturnsError(t *testing.T) {
	instances := []*ec2.Instance{
		{
			InstanceId: aws.String("i-stopped000000000"),
			State:      &ec2.InstanceState{Name: aws.String("stopped")},
			LaunchTime: aws.Time(time.Unix(100, 0)),
		},
	}

	_, err := selectRouterInstance(instances, nil)
	if err == nil {
		t.Fatal("expected an error when no running candidates exist")
	}
}

func TestSelectRouterInstance_EmptyInputReturnsError(t *testing.T) {
	_, err := selectRouterInstance(nil, nil)
	if err == nil {
		t.Fatal("expected an error for empty instance list")
	}
}

func TestWaitForTunnelReady_ReadyOnFirstPoll(t *testing.T) {
	endpoints := []ssh.Endpoint{{LocalPort: 1234, Status: true}}
	pollOnce := func() (bool, bool, []ssh.Endpoint, error) {
		return true, true, endpoints, nil
	}

	up, got, err := waitForTunnelReady(pollOnce, 50*time.Millisecond, 5*time.Millisecond, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !up {
		t.Error("expected tunnel to be reported up")
	}
	if len(got) != 1 || got[0].LocalPort != 1234 {
		t.Errorf("expected endpoints to be returned, got: %v", got)
	}
}

func TestWaitForTunnelReady_BecomesReadyAfterStableChecks(t *testing.T) {
	calls := 0
	pollOnce := func() (bool, bool, []ssh.Endpoint, error) {
		calls++
		// Not ready for the first 2 polls, then ready for 2 consecutive polls.
		ready := calls >= 3
		return ready, true, nil, nil
	}

	up, _, err := waitForTunnelReady(pollOnce, time.Second, time.Millisecond, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !up {
		t.Error("expected tunnel to be reported up")
	}
	// Needs polls 3 and 4 to both be ready to satisfy stableChecks=2.
	if calls < 4 {
		t.Errorf("expected at least 4 polls to require 2 consecutive ready results, got %d", calls)
	}
}

func TestWaitForTunnelReady_ResetsDebounceOnFlap(t *testing.T) {
	calls := 0
	pollOnce := func() (bool, bool, []ssh.Endpoint, error) {
		calls++
		// Ready once, then flaps back to not-ready (resetting the debounce counter) before
		// becoming stably ready.
		if calls == 1 {
			return true, true, nil, nil
		}
		if calls == 2 {
			return false, true, nil, nil
		}
		return true, true, nil, nil
	}

	_, _, err := waitForTunnelReady(pollOnce, time.Second, time.Millisecond, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Poll 1 alone would satisfy stableChecks=1 but not stableChecks=2; the flap at poll 2
	// resets the counter, so true readiness only lands after polls 3 and 4 are both ready.
	if calls < 4 {
		t.Errorf("expected the flap at poll 2 to reset the debounce, requiring at least 4 polls, got %d", calls)
	}
}

func TestWaitForTunnelReady_TimesOutReturningLastKnownState(t *testing.T) {
	pollOnce := func() (bool, bool, []ssh.Endpoint, error) {
		return false, false, nil, fmt.Errorf("endpoint not reachable")
	}

	up, _, err := waitForTunnelReady(pollOnce, 20*time.Millisecond, 5*time.Millisecond, 2)
	if err == nil {
		t.Fatal("expected the last error to be returned on timeout")
	}
	if up {
		t.Error("expected tunnel to be reported down after never becoming ready")
	}
}
