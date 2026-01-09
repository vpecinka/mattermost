// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.

package szncluster

import (
	"encoding/json"

	"github.com/hashicorp/memberlist"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

// clusterDelegate implements memberlist.Delegate interface
// This handles the core gossip protocol callbacks
type clusterDelegate struct {
	cluster *SznCluster
}

// NodeMeta is used to retrieve meta-data about the current node
// when broadcasting an alive message. It's length is limited to
// the given byte size. This metadata is available in the Node structure.
func (d *clusterDelegate) NodeMeta(limit int) []byte {
	if d.cluster == nil {
		return []byte{}
	}

	// Get config hash for web UI display
	configHash := computeConfigHash(d.cluster.platform.Config())

	// Get schema version for cluster sync verification
	_, schemaVersion, err := d.cluster.platform.DatabaseTypeAndSchemaVersion()
	if err != nil {
		mlog.Warn("SznCluster: Failed to get schema version for metadata", mlog.Err(err))
		schemaVersion = ""
	}

	// Create metadata with version, node ID, hostname (for display), advertise address (for connection),
	// config hash (for web UI) and schema version (for cluster sync)
	meta := map[string]string{
		"version":           model.CurrentVersion,
		"node_id":           d.cluster.nodeID,
		"hostname":          d.cluster.getHostname(),
		"advertise_address": d.cluster.getAdvertiseAddress(),
		"config_hash":       configHash,
		"schema_version":    schemaVersion,
	}

	data, err := json.Marshal(meta)
	if err != nil {
		mlog.Warn("SznCluster: Failed to marshal node metadata", mlog.Err(err))
		return []byte{}
	}

	if len(data) > limit {
		mlog.Warn("SznCluster: Node metadata exceeds limit", mlog.Int("size", len(data)), mlog.Int("limit", limit))
		return []byte{}
	}

	return data
}

// NotifyMsg is called when a user-data message is received.
// Care should be taken that this method does not block, since doing
// so would block the entire UDP packet receive loop.
func (d *clusterDelegate) NotifyMsg(msg []byte) {
	if d.cluster != nil {
		d.cluster.NotifyMsg(msg)
	}
}

// GetBroadcasts is called when user data messages can be broadcast.
// It can return a list of buffers to send. Each buffer should assume an
// overhead as provided with a limit on the total byte size allowed.
// The total byte size of the resulting data to send must not exceed
// the limit.
// This now delegates to TransmitLimitedQueue which handles automatic
// retransmission counting and message expiration.
func (d *clusterDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	if d.cluster == nil || d.cluster.broadcasts == nil {
		mlog.Debug("GetBroadcasts: cluster or broadcasts is nil")
		return nil
	}

	// TransmitLimitedQueue handles all the complexity:
	// - Tracks retransmission count per message
	// - Prioritizes messages with fewer transmissions (newer messages)
	// - Automatically expires messages after RetransmitMult * log(N+1) transmissions
	// - Calls Finished() on expired broadcasts
	return d.cluster.broadcasts.GetBroadcasts(overhead, limit)
}

// LocalState is used for a TCP Push/Pull. This is sent to
// the remote side in addition to the membership information.
// Any data can be sent here. See MergeRemoteState as well.
// For Mattermost, we don't need full state sync - cluster messages
// are propagated via broadcast queue and handlers.
func (d *clusterDelegate) LocalState(join bool) []byte {
	// Return empty - we handle state via ClusterMessage broadcasts
	mlog.Debug("SznCluster: LocalState requested", mlog.Bool("join", join))
	return []byte{}
}

// MergeRemoteState is invoked after a TCP Push/Pull. This is the
// state received from the remote side and is the result of the
// remote side's LocalState call. The 'join'
// boolean indicates this is for a join instead of a push/pull.
// For Mattermost, we don't need to merge state - cluster messages
// are propagated via broadcast queue and registered handlers.
func (d *clusterDelegate) MergeRemoteState(buf []byte, join bool) {
	// No action needed - we handle state via ClusterMessage broadcasts
	if len(buf) > 0 {
		mlog.Debug("SznCluster: Remote state received but ignored (using broadcast queue)",
			mlog.Int("size", len(buf)), mlog.Bool("join", join))
	}
}

// clusterEvents implements memberlist.EventDelegate interface
// This handles node join/leave events
type clusterEvents struct {
	cluster *SznCluster
}

// NotifyJoin is invoked when a node is detected to have joined.
func (e *clusterEvents) NotifyJoin(node *memberlist.Node) {
	mlog.Info("SznCluster: Node joined",
		mlog.String("node_id", node.Name),
		mlog.String("addr", node.Addr.String()))

	// SZN: Check if leader changed after node join
	if e.cluster != nil {
		e.cluster.checkAndNotifyLeaderChange()
	}
}

// NotifyLeave is invoked when a node is detected to have left.
func (e *clusterEvents) NotifyLeave(node *memberlist.Node) {
	mlog.Warn("SznCluster: Node left or failed",
		mlog.String("node_id", node.Name),
		mlog.String("addr", node.Addr.String()))

	// Don't cleanup from DB immediately - node might come back after temporary network issue
	// Memberlist handles all health checking and recovery via:
	// - Automatic probes (every 3s) for failure detection
	// - Push/Pull anti-entropy (every 20s) for state synchronization
	// - Nodes that restart will use DB as seed list to rejoin
	// The periodic cleanup job will remove stale DB entries after 30 minutes

	// SZN: Check if leader changed after node left
	if e.cluster != nil {
		e.cluster.checkAndNotifyLeaderChange()
	}
}

// NotifyUpdate is invoked when a node is detected to have updated.
func (e *clusterEvents) NotifyUpdate(node *memberlist.Node) {
	mlog.Debug("SznCluster: Node updated",
		mlog.String("node_id", node.Name),
		mlog.String("addr", node.Addr.String()))

	// SZN: Check if leader changed after node update (shouldn't normally happen, but for safety)
	if e.cluster != nil {
		e.cluster.checkAndNotifyLeaderChange()
	}
}
