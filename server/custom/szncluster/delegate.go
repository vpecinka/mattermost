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

	// Create metadata with version, node ID, hostname (for display) and advertise address (for connection)
	meta := map[string]string{
		"version":           model.CurrentVersion,
		"node_id":           d.cluster.nodeID,
		"hostname":          d.cluster.getHostname(),
		"advertise_address": d.cluster.getAdvertiseAddress(),
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
func (d *clusterDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	if d.cluster == nil {
		return nil
	}

	d.cluster.queueMu.Lock()
	defer d.cluster.queueMu.Unlock()

	if len(d.cluster.broadcastQueue) == 0 {
		return nil
	}

	// Calculate how many messages we can send
	broadcasts := make([][]byte, 0)
	totalSize := 0

	for i := 0; i < len(d.cluster.broadcastQueue); i++ {
		msg := d.cluster.broadcastQueue[i]
		msgSize := len(msg) + overhead

		if totalSize+msgSize > limit {
			break
		}

		broadcasts = append(broadcasts, msg)
		totalSize += msgSize
	}

	// Remove sent messages from queue
	if len(broadcasts) > 0 {
		d.cluster.broadcastQueue = d.cluster.broadcastQueue[len(broadcasts):]
	}

	return broadcasts
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

	// Clear any failed send tracking for this node
	if e.cluster != nil {
		e.cluster.failedMu.Lock()
		delete(e.cluster.failedSends, node.Name)
		e.cluster.failedMu.Unlock()
	}
}

// NotifyLeave is invoked when a node is detected to have left.
func (e *clusterEvents) NotifyLeave(node *memberlist.Node) {
	mlog.Warn("SznCluster: Node left or failed",
		mlog.String("node_id", node.Name),
		mlog.String("addr", node.Addr.String()))

	// Don't cleanup from DB immediately - node might come back after temporary network issue
	// The checkClusterHealth() function (runs every 60s) will attempt to reconnect
	// to nodes that are still alive in DB but not in memberlist
	// The periodic cleanup will handle stale entries after 30 minutes
}

// NotifyUpdate is invoked when a node is detected to have updated.
func (e *clusterEvents) NotifyUpdate(node *memberlist.Node) {
	mlog.Debug("SznCluster: Node updated",
		mlog.String("node_id", node.Name),
		mlog.String("addr", node.Addr.String()))
}
