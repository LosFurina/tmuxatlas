package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/LosFurina/tmuxatlas/pkg/common"
	"github.com/LosFurina/tmuxatlas/pkg/fleethealth"
	"github.com/LosFurina/tmuxatlas/pkg/peer"
	"github.com/LosFurina/tmuxatlas/pkg/state"
)

const fleetFreshness = 2 * time.Minute

func syncFleetHealth(
	ctx context.Context,
	coordinator *state.Coordinator,
	hosts []peer.HostInfo,
	now time.Time,
) error {
	snapshot, err := coordinator.Snapshot(ctx)
	if err != nil {
		return err
	}
	current := snapshot.State.Health
	next := make(map[state.HostKey]state.Health, len(hosts))
	for _, host := range hosts {
		online := host.Online
		protocol := int(host.RuntimeProtocol)
		minimum, maximum := int(peer.RuntimeProtocolMin), int(peer.RuntimeProtocolMax)
		role := "agent"
		if host.Local {
			role = "standalone"
		}
		record := fleethealth.Evaluate(fleethealth.Facts{
			HostID: host.ID, DisplayName: host.Name, Role: role,
			Online: &online, LastSeen: host.LastSeen, LastStateSync: host.LastSeen,
			Version: host.Version, HubVersion: common.VERSION,
			ProtocolVersion: &protocol, ProtocolMin: &minimum, ProtocolMax: &maximum,
			Deployment: "native",
		}, now, fleetFreshness)
		facts, err := recordMap(record)
		if err != nil {
			return err
		}
		hostKey := state.NewHostKey(host.ID)
		next[hostKey] = state.Health{
			HostKey: hostKey, LastStateSync: host.LastSeen, Facts: facts,
		}
	}

	var operations []state.Operation
	for key := range current {
		if _, ok := next[key]; !ok {
			operations = append(operations, state.Operation{
				Kind: state.OperationRemoveHealth, Key: string(key),
			})
		}
	}
	for key, value := range next {
		previous, ok := current[key]
		if ok && healthJSONEqual(previous, value) {
			continue
		}
		value := value
		operations = append(operations, state.Operation{
			Kind: state.OperationUpsertHealth, Health: &value,
		})
	}
	_, err = coordinator.Commit(ctx, operations...)
	return err
}

func recordMap(record fleethealth.Record) (map[string]any, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("marshal fleet health: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("normalize fleet health: %w", err)
	}
	return result, nil
}

func healthJSONEqual(left, right state.Health) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}
