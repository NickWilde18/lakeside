package itsmagent

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeSignalEvent(t *testing.T) {
	event := normalizeSignalEvent(TicketDraft{
		UserCode:     "122020255",
		Priority:     "3",
		ServiceLevel: "3",
		Subject:      "道扬书院C1010宿舍WiFi网络差、网页无法打开",
		OthersDesc:   "宿舍所有设备均受影响，网页打不开。",
	})
	if event.Domain != "network" {
		t.Fatalf("unexpected domain: %q", event.Domain)
	}
	if event.Object != "wifi" {
		t.Fatalf("unexpected object: %q", event.Object)
	}
	if event.Scope != scopeBuilding {
		t.Fatalf("unexpected scope: %q", event.Scope)
	}
	if event.LocationScope != "道扬书院" {
		t.Fatalf("unexpected location scope: %q", event.LocationScope)
	}
	if !strings.Contains(event.NormalizedSummary, "domain=network") {
		t.Fatalf("unexpected normalized summary: %q", event.NormalizedSummary)
	}
}

func TestShouldPromoteP1(t *testing.T) {
	if shouldPromoteP1("2", 4, scopeBuilding, 5) {
		t.Fatalf("4 distinct users should not trigger P1")
	}
	if !shouldPromoteP1("2", 5, scopeBuilding, 5) {
		t.Fatalf("5 distinct users at building scope should trigger P1")
	}
	if !shouldPromoteP1("3", 5, scopeArea, 5) {
		t.Fatalf("level 3 should also be promotable to P1")
	}
	if shouldPromoteP1("4", 6, scopeArea, 5) {
		t.Fatalf("level 4 should not be promotable to P1")
	}
}

func TestSummarizeClusterPromotesBuildingBySharedLocation(t *testing.T) {
	now := time.Now().UTC()
	distinctUsers, impactScope := summarizeCluster("cluster-1", []signalEvent{
		{ClusterID: "cluster-1", UserCode: "u1", Scope: scopeSingleUser, LocationScope: "道扬书院", CreatedAt: now.Add(-2 * time.Minute)},
		{ClusterID: "cluster-1", UserCode: "u2", Scope: scopeSingleUser, LocationScope: "道扬书院", CreatedAt: now.Add(-1 * time.Minute)},
	}, signalEvent{ClusterID: "cluster-1", UserCode: "u3", Scope: scopeSingleUser, LocationScope: "道扬书院", CreatedAt: now})
	if distinctUsers != 3 {
		t.Fatalf("unexpected distinct user count: %d", distinctUsers)
	}
	if impactScope != scopeBuilding {
		t.Fatalf("unexpected impact scope: %q", impactScope)
	}
}
