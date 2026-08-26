/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package tunnel

import (
	"testing"
	"time"

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
