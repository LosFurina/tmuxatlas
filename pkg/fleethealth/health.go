package fleethealth

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Check struct {
	Name    string `json:"name"`
	OK      *bool  `json:"ok,omitempty"`
	Message string `json:"message,omitempty"`
	Command string `json:"command,omitempty"`
}

type UpdateOutcome struct {
	SourceVersion   string    `json:"source_version,omitempty"`
	TargetVersion   string    `json:"target_version,omitempty"`
	RestoredVersion string    `json:"restored_version,omitempty"`
	Outcome         string    `json:"outcome,omitempty"`
	At              time.Time `json:"at,omitempty"`
	Error           string    `json:"error,omitempty"`
}

type Facts struct {
	HostID          string         `json:"host_id"`
	DisplayName     string         `json:"display_name"`
	Role            string         `json:"role,omitempty"`
	Platform        string         `json:"platform,omitempty"`
	Online          *bool          `json:"online,omitempty"`
	LastSeen        time.Time      `json:"last_seen,omitempty"`
	LastStateSync   time.Time      `json:"last_state_sync,omitempty"`
	Version         string         `json:"version,omitempty"`
	Commit          string         `json:"commit,omitempty"`
	HubVersion      string         `json:"hub_version,omitempty"`
	ProtocolVersion *int           `json:"protocol_version,omitempty"`
	ProtocolMin     *int           `json:"protocol_min,omitempty"`
	ProtocolMax     *int           `json:"protocol_max,omitempty"`
	Checks          []Check        `json:"checks,omitempty"`
	Deployment      string         `json:"deployment,omitempty"`
	ImageTag        string         `json:"image_tag,omitempty"`
	ImageDigest     string         `json:"image_digest,omitempty"`
	LastUpdate      *UpdateOutcome `json:"last_update,omitempty"`
}

type Reason struct {
	Code        string `json:"code"`
	Severity    int    `json:"severity"`
	Message     string `json:"message"`
	Evidence    string `json:"evidence,omitempty"`
	Remediation string `json:"remediation,omitempty"`
}

type Record struct {
	Facts     Facts     `json:"facts"`
	Summary   string    `json:"summary"`
	Reasons   []Reason  `json:"reasons"`
	Evaluated time.Time `json:"evaluated_at"`
}

func Evaluate(facts Facts, now time.Time, freshness time.Duration) Record {
	var reasons []Reason
	if facts.Online == nil {
		reasons = append(reasons, reason("unknown", 1, "Online state is unknown.", "", diagnosticCommand(facts)))
	} else if !*facts.Online {
		reasons = append(reasons, reason(
			"offline", 4, "The host is offline.", formatTime(facts.LastSeen), diagnosticCommand(facts),
		))
	} else if facts.LastStateSync.IsZero() {
		reasons = append(reasons, reason("unknown", 1, "Last state sync is unknown.", "", diagnosticCommand(facts)))
	} else if now.Sub(facts.LastStateSync) > freshness {
		reasons = append(reasons, reason(
			"stale", 3, "The host is online but its state is stale.",
			now.Sub(facts.LastStateSync).Round(time.Second).String(), diagnosticCommand(facts),
		))
	}

	if hostVersion, ok := parseVersion(facts.Version); !ok {
		reasons = append(reasons, reason("unknown", 1, "Host version is unknown.", facts.Version, diagnosticCommand(facts)))
	} else if hubVersion, ok := parseVersion(facts.HubVersion); !ok {
		reasons = append(reasons, reason("unknown", 1, "Hub version is unknown.", facts.HubVersion, "tmuxatlas --version"))
	} else {
		switch compareVersion(hostVersion, hubVersion) {
		case -1:
			reasons = append(reasons, reason(
				"version-behind", 2, "The host version is behind the Hub.",
				fmt.Sprintf("%s < %s", facts.Version, facts.HubVersion), updateCommand(facts),
			))
		case 1:
			reasons = append(reasons, reason(
				"version-ahead", 2, "The host version is ahead of the Hub.",
				fmt.Sprintf("%s > %s", facts.Version, facts.HubVersion), updateCommand(facts),
			))
		}
	}

	if facts.ProtocolVersion == nil || facts.ProtocolMin == nil || facts.ProtocolMax == nil {
		reasons = append(reasons, reason(
			"unknown", 1, "Protocol compatibility is unknown.", "", diagnosticCommand(facts),
		))
	} else if *facts.ProtocolVersion < *facts.ProtocolMin || *facts.ProtocolVersion > *facts.ProtocolMax {
		reasons = append(reasons, reason(
			"incompatible", 5, "The host explicitly reports an incompatible protocol.",
			fmt.Sprintf("%d not in [%d,%d]", *facts.ProtocolVersion, *facts.ProtocolMin, *facts.ProtocolMax),
			updateCommand(facts),
		))
	}

	checksKnown := len(facts.Checks) > 0
	for _, check := range facts.Checks {
		if check.OK == nil {
			reasons = append(reasons, reason("unknown", 1, check.Name+" check is unknown.", check.Message, check.Command))
		} else if !*check.OK {
			reasons = append(reasons, reason("check-failed", 3, check.Name+" check failed.", check.Message, check.Command))
		}
	}
	if !checksKnown {
		reasons = append(reasons, reason("unknown", 1, "Agent and hook checks are unknown.", "", diagnosticCommand(facts)))
	}
	if facts.LastUpdate != nil && facts.LastUpdate.Outcome == "rolled-back" {
		reasons = append(reasons, reason(
			"update-rolled-back", 3, "The most recent update was rolled back.",
			facts.LastUpdate.Error, updateCommand(facts),
		))
	}

	summary := "healthy"
	highest := 0
	for _, item := range reasons {
		if item.Severity > highest {
			highest = item.Severity
			summary = item.Code
		}
	}
	if highest == 0 {
		reasons = []Reason{reason("healthy", 0, "The host is online, fresh, compatible, and all checks pass.", "", "")}
	}
	return Record{Facts: facts, Summary: summary, Reasons: reasons, Evaluated: now}
}

func reason(code string, severity int, message, evidence, remediation string) Reason {
	return Reason{Code: code, Severity: severity, Message: message, Evidence: evidence, Remediation: remediation}
}

var semverPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:[-+].*)?$`)

func parseVersion(value string) ([3]int, bool) {
	match := semverPattern.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return [3]int{}, false
	}
	var result [3]int
	for index := range result {
		number, err := strconv.Atoi(match[index+1])
		if err != nil {
			return [3]int{}, false
		}
		result[index] = number
	}
	return result, true
}

func compareVersion(left, right [3]int) int {
	for index := range left {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	return 0
}

func diagnosticCommand(facts Facts) string {
	if facts.Deployment == "docker" {
		return "docker compose exec tmuxatlas tmuxatlas doctor"
	}
	return "tmuxatlas doctor"
}

func updateCommand(facts Facts) string {
	if facts.Deployment == "docker" {
		return "docker compose pull && docker compose up -d"
	}
	return "tmuxatlas update"
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
