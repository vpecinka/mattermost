// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.
//
// This code is a derivative work based on Mattermost server
// (https://github.com/mattermost/mattermost-server)
// Original Mattermost code is licensed under AGPL v3.0.
//
// This derivative work is used exclusively for internal purposes
// within Seznam.cz, a.s. and is not distributed to third parties.
// Therefore, the copyleft provisions of AGPL v3.0 do not apply
// as per the license's distribution requirements.

package szncluster

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/public/shared/request"
	"github.com/mattermost/mattermost/server/v8/channels/app/platform"
	"github.com/mattermost/mattermost/server/v8/einterfaces"
)

// Cluster configuration constants
const (
	// maxBroadcastQueueSize is the maximum number of messages in the broadcast queue
	// If exceeded, oldest messages are dropped to prevent memory exhaustion
	maxBroadcastQueueSize = 10000

	// deduplicationWindowMs is the time window in milliseconds for detecting duplicate messages
	deduplicationWindowMs = 5000

	// cleanupIntervalSec is the interval in seconds for cleaning up the deduplication cache
	cleanupIntervalSec = 10

	// healthCheckIntervalSec is the interval in seconds for periodic health checks
	healthCheckIntervalSec = 20

	// discoveryUpdateIntervalSec is the interval in seconds for updating cluster discovery in DB
	discoveryUpdateIntervalSec = 15

	// initialHealthCheckSec is the delay before the first health check after startup
	initialHealthCheckSec = 3

	// reconnectWindowSec is the maximum age in seconds for attempting to reconnect to a node
	// Nodes older than this are considered dead and not retried
	reconnectWindowSec = 90
)

// SznCluster implements einterfaces.ClusterInterface using Hashicorp Memberlist
type SznCluster struct {
	platform   *platform.PlatformService
	memberlist *memberlist.Memberlist
	delegate   *clusterDelegate
	events     *clusterEvents

	// Node identification
	nodeID   string
	hostname string

	// Message handlers
	handlers   map[model.ClusterEvent]einterfaces.ClusterMessageHandler
	handlersMu sync.RWMutex

	// Broadcast queue for outgoing messages
	broadcastQueue [][]byte
	queueMu        sync.Mutex

	// Deduplication cache for incoming messages
	seenMessages map[string]int64 // hash -> timestamp (milliseconds)
	seenMu       sync.RWMutex

	// Failed send tracking for retry mechanism
	// Tracks nodes that failed to receive messages, with backoff
	failedSends map[string]int64 // nodeID -> last_failed_timestamp
	failedMu    sync.RWMutex

	// State
	started bool
	startMu sync.Mutex

	// Shutdown channel
	shutdownCh chan struct{}

	// Discovery ticker
	discoveryTicker *time.Ticker
}

// generatePersistentNodeID creates a deterministic node ID based on hostname and gossip port
// This ensures the same ID is used across restarts
func generatePersistentNodeID(hostname string, gossipPort int) string {
	// Create a deterministic ID from hostname:port
	// Using SHA256 hash to get a fixed-length ID similar to model.NewId() format
	data := fmt.Sprintf("%s:%d", hostname, gossipPort)
	hash := sha256.Sum256([]byte(data))
	// Convert to hex and take first 26 characters to match model.NewId() length
	hexHash := hex.EncodeToString(hash[:])
	if len(hexHash) > 26 {
		hexHash = hexHash[:26]
	}
	return hexHash
}

// NewSznCluster creates a new cluster instance
func NewSznCluster(ps *platform.PlatformService) einterfaces.ClusterInterface {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	cluster := &SznCluster{
		platform:       ps,
		hostname:       hostname,
		handlers:       make(map[model.ClusterEvent]einterfaces.ClusterMessageHandler),
		broadcastQueue: make([][]byte, 0),
		seenMessages:   make(map[string]int64),
		failedSends:    make(map[string]int64),
		shutdownCh:     make(chan struct{}),
	}

	// Create delegate and events handlers
	cluster.delegate = &clusterDelegate{cluster: cluster}
	cluster.events = &clusterEvents{cluster: cluster}

	// Generate persistent node ID based on advertise address and gossip port
	// This ensures the same ID is used across restarts
	cfg := ps.Config()
	advertiseAddress := cluster.getAdvertiseAddress()
	gossipPort := *cfg.ClusterSettings.GossipPort
	cluster.nodeID = generatePersistentNodeID(advertiseAddress, gossipPort)

	return cluster
}

// StartInterNodeCommunication initializes and starts the cluster communication
func (c *SznCluster) StartInterNodeCommunication() {
	c.startMu.Lock()
	defer c.startMu.Unlock()

	if c.started {
		mlog.Warn("SznCluster: Inter-node communication already started")
		return
	}

	mlog.Info("SznCluster: Starting inter-node communication",
		mlog.String("node_id", c.nodeID))

	// Cleanup old cluster discovery entries
	if err := c.platform.Store.ClusterDiscovery().Cleanup(); err != nil {
		mlog.Warn("SznCluster: Failed to cleanup old cluster discovery entries", mlog.Err(err))
	}

	// Initialize memberlist
	if err := c.initializeMemberlist(); err != nil {
		mlog.Error("SznCluster: Failed to initialize memberlist", mlog.Err(err))
		return
	}

	// Join the cluster
	if err := c.joinCluster(); err != nil {
		mlog.Error("SznCluster: Failed to join cluster", mlog.Err(err))
		// Continue anyway - we might be the first node
	}

	// Start cluster discovery service
	go c.startClusterDiscovery()

	// Start periodic cleanup of deduplication cache
	go c.startDeduplicationCleanup()

	c.started = true
	mlog.Info("SznCluster: Inter-node communication started successfully")
}

// StopInterNodeCommunication stops the cluster communication
func (c *SznCluster) StopInterNodeCommunication() {
	c.startMu.Lock()
	defer c.startMu.Unlock()

	if !c.started {
		return
	}

	mlog.Info("SznCluster: Stopping inter-node communication", mlog.String("node_id", c.nodeID))

	// Stop cluster discovery
	close(c.shutdownCh)

	// Leave the cluster gracefully
	if c.memberlist != nil {
		if err := c.memberlist.Leave(1 * 1e9); err != nil {
			mlog.Warn("SznCluster: Error leaving cluster", mlog.Err(err))
		}

		if err := c.memberlist.Shutdown(); err != nil {
			mlog.Warn("SznCluster: Error shutting down memberlist", mlog.Err(err))
		}
	}

	c.started = false
	mlog.Info("SznCluster: Inter-node communication stopped")
}

// RegisterClusterMessageHandler registers a handler for cluster events
func (c *SznCluster) RegisterClusterMessageHandler(event model.ClusterEvent, handler einterfaces.ClusterMessageHandler) {
	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()

	c.handlers[event] = handler
	mlog.Debug("SznCluster: Registered handler", mlog.String("event", string(event)))
}

// GetClusterId returns the unique identifier for this node
func (c *SznCluster) GetClusterId() string {
	return c.nodeID
}

// IsLeader returns true if this node is the cluster leader
// Uses simple algorithm: node with lexicographically smallest ID is the leader
func (c *SznCluster) IsLeader() bool {
	if !c.started || c.memberlist == nil {
		return false
	}

	members := c.memberlist.Members()
	if len(members) == 0 {
		return true // We're the only node
	}

	// Find node with smallest name (ID)
	leaderName := c.nodeID
	for _, member := range members {
		if member.Name < leaderName {
			leaderName = member.Name
		}
	}

	return leaderName == c.nodeID
}

// HealthScore returns a health score for this node (lower is better)
func (c *SznCluster) HealthScore() int {
	if !c.started {
		return 100 // Unhealthy if not started
	}

	if c.memberlist == nil {
		return 50
	}

	// Use memberlist health score
	score := c.memberlist.GetHealthScore()
	// Memberlist health score is 0 when healthy, increases with issues
	return score
}

// GetMyClusterInfo returns information about this node
func (c *SznCluster) GetMyClusterInfo() *model.ClusterInfo {
	// Use same logic as for remote peers:
	// - IPAddress: advertise address (where others should connect)
	// - Hostname: display name (for debugging/info)
	return &model.ClusterInfo{
		Id:        c.nodeID,
		IPAddress: c.getAdvertiseAddress(), // Where to connect
		Hostname:  c.getHostname(),         // Display name
		Version:   model.CurrentVersion,
	}
}

// GetClusterInfos returns information about all nodes in the cluster
func (c *SznCluster) GetClusterInfos() ([]*model.ClusterInfo, error) {
	if !c.started || c.memberlist == nil {
		return []*model.ClusterInfo{c.GetMyClusterInfo()}, nil
	}

	members := c.memberlist.Members()
	infos := make([]*model.ClusterInfo, 0, len(members))

	for _, member := range members {
		if member.Name == c.nodeID {
			// Add our own info
			infos = append(infos, c.GetMyClusterInfo())
		} else {
			// Parse metadata to get version, node ID, hostname and advertise address
			version := model.CurrentVersion
			nodeID := member.Name
			hostname := member.Addr.String()         // Default to IP if not in metadata
			advertiseAddress := member.Addr.String() // Default to IP if not in metadata

			if len(member.Meta) > 0 {
				var meta map[string]string
				if err := json.Unmarshal(member.Meta, &meta); err == nil {
					if v, ok := meta["version"]; ok {
						version = v
					}
					if id, ok := meta["node_id"]; ok {
						nodeID = id
					}
					if h, ok := meta["hostname"]; ok {
						hostname = h
					}
					if addr, ok := meta["advertise_address"]; ok {
						advertiseAddress = addr
					}
				}
			}

			// Add info from other nodes
			infos = append(infos, &model.ClusterInfo{
				Id:        nodeID,
				IPAddress: advertiseAddress, // Use advertise address (where to connect)
				Hostname:  hostname,         // Use hostname (for display/debug)
				Version:   version,
			})
		}
	}

	return infos, nil
}

// SendClusterMessage sends a message to all nodes in the cluster
func (c *SznCluster) SendClusterMessage(msg *model.ClusterMessage) {
	if !c.started {
		mlog.Debug("SznCluster: Cannot send message, cluster not started")
		return
	}

	mlog.Debug("SznCluster: Broadcasting message", mlog.String("event", string(msg.Event)))

	// Serialize the message
	data, err := json.Marshal(msg)
	if err != nil {
		mlog.Error("SznCluster: Failed to serialize message", mlog.Err(err))
		return
	}

	// For ClusterSendReliable messages, add to broadcast queue for gossip-based retransmission
	// Only fill queue if there are other nodes that could receive the messages
	// For ClusterSendBestEffort messages, skip queue (send once only)
	if msg.SendType == model.ClusterSendReliable && c.hasOtherNodes() {
		c.queueMu.Lock()
		// Enforce queue size limit to prevent memory exhaustion
		if len(c.broadcastQueue) >= maxBroadcastQueueSize {
			// Drop oldest message
			c.broadcastQueue = c.broadcastQueue[1:]
			mlog.Warn("SznCluster: Broadcast queue overflow, dropping oldest message",
				mlog.Int("queue_size", maxBroadcastQueueSize))
		}
		c.broadcastQueue = append(c.broadcastQueue, data)
		c.queueMu.Unlock()
		mlog.Debug("SznCluster: Message added to broadcast queue",
			mlog.String("event", string(msg.Event)),
			mlog.Int("queue_size", len(c.broadcastQueue)))
	} else if msg.SendType == model.ClusterSendReliable {
		mlog.Debug("SznCluster: Skipping broadcast queue, no other nodes in cluster",
			mlog.String("event", string(msg.Event)))
	}

	// Send immediately to all nodes via direct messaging (both Reliable and BestEffort)
	c.sendToAllNodes(data)

	// Log cluster members for debugging
	if c.memberlist != nil {
		members := c.memberlist.Members()
		mlog.Debug("SznCluster: Current cluster members",
			mlog.Int("count", len(members)),
			mlog.String("event", string(msg.Event)))
	}
}

// SendClusterMessageToNode sends a message to a specific node
func (c *SznCluster) SendClusterMessageToNode(nodeID string, msg *model.ClusterMessage) error {
	if !c.started {
		return model.NewAppError("SznCluster.SendClusterMessageToNode", "cluster.not_started", nil, "", 500)
	}

	if c.memberlist == nil {
		return model.NewAppError("SznCluster.SendClusterMessageToNode", "cluster.memberlist_nil", nil, "", 500)
	}

	mlog.Debug("SznCluster: Sending message to node",
		mlog.String("event", string(msg.Event)),
		mlog.String("target_node", nodeID))

	// Serialize the message
	data, err := json.Marshal(msg)
	if err != nil {
		return model.NewAppError("SznCluster.SendClusterMessageToNode", "cluster.serialize_failed", nil, err.Error(), 500)
	}

	// Find target node
	members := c.memberlist.Members()
	var targetNode *memberlist.Node
	for _, member := range members {
		if member.Name == nodeID {
			targetNode = member
			break
		}
	}

	if targetNode == nil {
		return model.NewAppError("SznCluster.SendClusterMessageToNode", "cluster.node_not_found", nil, "nodeID="+nodeID, 404)
	}

	// Send to specific node
	if err := c.memberlist.SendBestEffort(targetNode, data); err != nil {
		return model.NewAppError("SznCluster.SendClusterMessageToNode", "cluster.send_failed", nil, err.Error(), 500)
	}

	return nil
}

// NotifyMsg is called by memberlist when a message is received
func (c *SznCluster) NotifyMsg(buf []byte) {
	mlog.Debug("SznCluster: Received message", mlog.Int("size", len(buf)))

	// Deserialize the message
	var msg model.ClusterMessage
	if err := json.Unmarshal(buf, &msg); err != nil {
		mlog.Error("SznCluster: Failed to deserialize message", mlog.Err(err))
		return
	}

	// Check for duplicates and mark as seen atomically
	hash := c.messageHash(&msg)
	if c.isDuplicateOrMarkSeen(hash) {
		mlog.Debug("SznCluster: Dropping duplicate message",
			mlog.String("event", string(msg.Event)),
			mlog.String("hash", hash[:16])) // Log first 16 chars of hash
		return
	}

	// Message is new and already marked as seen - process it

	// Route to appropriate handler
	c.handlersMu.RLock()
	handler, ok := c.handlers[msg.Event]
	c.handlersMu.RUnlock()

	if ok && handler != nil {
		handler(&msg)
	} else {
		mlog.Debug("SznCluster: No handler for event", mlog.String("event", string(msg.Event)))
	}
}

// GetClusterStats returns statistics about the cluster
func (c *SznCluster) GetClusterStats(rctx request.CTX) ([]*model.ClusterStats, *model.AppError) {
	if !c.started {
		return []*model.ClusterStats{}, nil
	}

	stats := &model.ClusterStats{
		Id: c.nodeID,
	}

	if c.memberlist != nil {
		members := c.memberlist.Members()
		stats.TotalWebsocketConnections = len(members)
		stats.TotalReadDbConnections = len(members)
		stats.TotalMasterDbConnections = len(members)
	}

	return []*model.ClusterStats{stats}, nil
}

// GetLogs returns logs from this node
// This is a stub - actual log retrieval would require access to log files
func (c *SznCluster) GetLogs(rctx request.CTX, page, perPage int) ([]string, *model.AppError) {
	// In a full implementation, this would read from log files
	// For now, return cluster status info
	if !c.started {
		return []string{"Cluster not started"}, nil
	}

	logs := []string{
		"SznCluster Status:",
		"Node ID: " + c.nodeID,
		"Hostname: " + c.hostname,
	}

	if c.memberlist != nil {
		members := c.memberlist.Members()
		logs = append(logs, fmt.Sprintf("Cluster Members: %d", len(members)))
		for _, member := range members {
			logs = append(logs, fmt.Sprintf("  - %s (%s)", member.Name, member.Addr.String()))
		}
	}

	return logs, nil
}

// QueryLogs returns logs from all nodes in the cluster
// This is a stub - actual implementation would use gossip messaging
func (c *SznCluster) QueryLogs(rctx request.CTX, page, perPage int) (map[string][]string, *model.AppError) {
	if !c.started {
		return map[string][]string{}, nil
	}

	// In a full implementation, this would:
	// 1. Send ClusterGossipEventRequestGetLogs to all nodes
	// 2. Collect responses with ClusterGossipEventResponseGetLogs
	// 3. Return aggregated logs from all nodes

	// For now, just return our own logs
	logs, err := c.GetLogs(rctx, page, perPage)
	if err != nil {
		return map[string][]string{}, err
	}

	return map[string][]string{
		c.nodeID: logs,
	}, nil
}

// GenerateSupportPacket generates support package data from all nodes
// This is a stub - actual implementation would use gossip messaging
func (c *SznCluster) GenerateSupportPacket(rctx request.CTX, options *model.SupportPacketOptions) (map[string][]model.FileData, error) {
	if !c.started {
		return map[string][]model.FileData{}, nil
	}

	// In a full implementation, this would:
	// 1. Send ClusterGossipEventRequestGenerateSupportPacket to all nodes
	// 2. Collect responses with ClusterGossipEventResponseGenerateSupportPacket
	// 3. Return aggregated support packet data from all nodes

	// For now, return basic cluster info
	clusterInfo := []model.FileData{
		{
			Filename: "cluster_info.txt",
			Body:     []byte(fmt.Sprintf("Node ID: %s\nHostname: %s\nStarted: %v", c.nodeID, c.hostname, c.started)),
		},
	}

	return map[string][]model.FileData{
		c.nodeID: clusterInfo,
	}, nil
}

// GetPluginStatuses returns plugin statuses from all nodes
// This is a stub - actual implementation would use gossip messaging
func (c *SznCluster) GetPluginStatuses() (model.PluginStatuses, *model.AppError) {
	if !c.started {
		return model.PluginStatuses{}, nil
	}

	// In a full implementation, this would:
	// 1. Send ClusterGossipEventRequestGetPluginStatuses to all nodes
	// 2. Collect responses with ClusterGossipEventResponseGetPluginStatuses
	// 3. Return aggregated plugin statuses from all nodes

	// For now, return empty status as we don't have access to plugin manager
	return model.PluginStatuses{}, nil
}

// ConfigChanged notifies the cluster about configuration changes
func (c *SznCluster) ConfigChanged(previousConfig *model.Config, newConfig *model.Config, sendToOtherServer bool) *model.AppError {
	if !sendToOtherServer {
		return nil
	}

	if !c.started {
		return nil
	}

	mlog.Debug("SznCluster: Config changed, notifying cluster")

	// Broadcast config change event
	msg := &model.ClusterMessage{
		Event:    model.ClusterGossipEventRequestSaveConfig,
		SendType: model.ClusterSendReliable,
	}

	c.SendClusterMessage(msg)
	return nil
}

// WebConnCountForUser returns the number of web connections for a user
// This is a stub - actual implementation would require app.Hub integration
func (c *SznCluster) WebConnCountForUser(userID string) (int, *model.AppError) {
	if !c.started {
		return 0, nil
	}

	// In a full implementation, this would:
	// 1. Send ClusterGossipEventRequestWebConnCount to all nodes
	// 2. Collect responses with ClusterGossipEventResponseWebConnCount
	// 3. Sum up connection counts from all nodes
	// For now, return 0 as we don't have access to app.Hub here
	return 0, nil
}

// GetWSQueues returns websocket queue information
// This is a stub - actual implementation would require app.Hub integration
func (c *SznCluster) GetWSQueues(userID, connectionID string, seqNum int64) (map[string]*model.WSQueues, error) {
	if !c.started {
		return map[string]*model.WSQueues{}, nil
	}

	// In a full implementation, this would:
	// 1. Send ClusterGossipEventRequestWSQueues to all nodes with userID, connectionID, seqNum
	// 2. Collect responses with ClusterGossipEventResponseWSQueues
	// 3. Aggregate queue information from all nodes
	// For now, return empty map as we don't have access to app.Hub here
	return map[string]*model.WSQueues{}, nil
}

// startClusterDiscovery periodically updates cluster discovery information
func (c *SznCluster) startClusterDiscovery() {
	cfg := c.platform.Config()
	clusterName := ""
	if cfg.ClusterSettings.ClusterName != nil {
		clusterName = *cfg.ClusterSettings.ClusterName
	}

	// Update immediately on start
	c.updateClusterDiscovery(clusterName)

	// Perform initial health check after startup
	// This helps nodes that started after others to discover and join them quickly
	initialHealthCheck := time.NewTimer(initialHealthCheckSec * time.Second)
	defer initialHealthCheck.Stop()

	// Create ticker for periodic updates
	// Shortened for faster cluster state convergence
	c.discoveryTicker = time.NewTicker(discoveryUpdateIntervalSec * time.Second)
	defer c.discoveryTicker.Stop()

	// Create ticker for health checks
	// Shortened for faster recovery of temporarily unavailable nodes
	healthTicker := time.NewTicker(healthCheckIntervalSec * time.Second)
	defer healthTicker.Stop()

	for {
		select {
		case <-initialHealthCheck.C:
			mlog.Info("SznCluster: Running initial health check")
			c.checkClusterHealth(clusterName)
		case <-c.discoveryTicker.C:
			c.updateClusterDiscovery(clusterName)
		case <-healthTicker.C:
			c.checkClusterHealth(clusterName)
		case <-c.shutdownCh:
			// Cleanup on shutdown
			c.cleanupClusterDiscovery()
			return
		}
	}
}

// updateClusterDiscovery updates this node's entry in the cluster discovery table
func (c *SznCluster) updateClusterDiscovery(clusterName string) {
	advertiseAddress := c.getAdvertiseAddress()
	cfg := c.platform.Config()

	discovery := &model.ClusterDiscovery{
		Id:          c.nodeID, // Store nodeID as Id for mapping
		ClusterName: clusterName,
		Type:        model.CDSTypeApp,
		Hostname:    advertiseAddress, // Store advertise address (where other nodes connect)
		GossipPort:  int32(*cfg.ClusterSettings.GossipPort),
		LastPingAt:  model.GetMillis(),
	}

	// Check if entry exists
	exists, err := c.platform.Store.ClusterDiscovery().Exists(discovery)
	if err != nil {
		mlog.Warn("SznCluster: Failed to check cluster discovery existence", mlog.Err(err))
		return
	}

	if !exists {
		// Create new entry
		if err := c.platform.Store.ClusterDiscovery().Save(discovery); err != nil {
			mlog.Warn("SznCluster: Failed to save cluster discovery", mlog.Err(err))
		} else {
			mlog.Info("SznCluster: Cluster discovery entry created",
				mlog.String("advertise_address", advertiseAddress),
				mlog.Int("gossip_port", int(discovery.GossipPort)))
		}
	} else {
		// Update existing entry
		if err := c.platform.Store.ClusterDiscovery().SetLastPingAt(discovery); err != nil {
			mlog.Warn("SznCluster: Failed to update cluster discovery", mlog.Err(err))
		} else {
			mlog.Debug("SznCluster: Cluster discovery updated",
				mlog.String("advertise_address", advertiseAddress),
				mlog.Int("gossip_port", int(discovery.GossipPort)))
		}
	}
}

// cleanupClusterDiscovery removes this node's entry from cluster discovery table
func (c *SznCluster) cleanupClusterDiscovery() {
	advertiseAddress := c.getAdvertiseAddress()

	discovery := &model.ClusterDiscovery{
		Id:       c.nodeID, // Use nodeID for proper identification
		Type:     model.CDSTypeApp,
		Hostname: advertiseAddress,
	}

	if _, err := c.platform.Store.ClusterDiscovery().Delete(discovery); err != nil {
		mlog.Warn("SznCluster: Failed to cleanup cluster discovery", mlog.Err(err))
	}
}

// checkClusterHealth verifies that nodes in DB are reachable via memberlist
// and attempts to reconnect to nodes that are alive in DB but not in memberlist
func (c *SznCluster) checkClusterHealth(clusterName string) {
	if !c.started || c.memberlist == nil {
		return
	}

	// Get nodes from DB
	discoveries, err := c.platform.Store.ClusterDiscovery().GetAll(model.CDSTypeApp, clusterName)
	if err != nil {
		mlog.Warn("SznCluster: Failed to get cluster discoveries for health check", mlog.Err(err))
		return
	}

	// Get current memberlist members - build map by nodeID (Name)
	members := c.memberlist.Members()
	memberMap := make(map[string]*memberlist.Node) // nodeID -> Node
	for _, member := range members {
		memberMap[member.Name] = member
	}

	cfg := c.platform.Config()

	// Track nodes that need reconnection
	nodesToReconnect := make([]string, 0)

	// Check each DB entry
	for _, discovery := range discoveries {
		// Skip ourselves (compare by nodeID stored in discovery.Id)
		if discovery.Id == c.nodeID {
			continue
		}

		// Check if this node is in memberlist by nodeID
		member, foundInMemberlist := memberMap[discovery.Id]

		// Build address for this node
		gossipPort := discovery.GossipPort
		if gossipPort == 0 {
			gossipPort = int32(*cfg.ClusterSettings.GossipPort)
		}
		addr := fmt.Sprintf("%s:%d", discovery.Hostname, gossipPort)

		if !foundInMemberlist {
			// Node is in DB but not in memberlist - check if it's recently alive
			secondsSinceLastPing := int((model.GetMillis() - discovery.LastPingAt) / 1000)

			// Only try to reconnect if node was alive within reconnect window
			// This prevents reconnecting to truly dead nodes
			if secondsSinceLastPing < reconnectWindowSec {
				mlog.Warn("SznCluster: Node in DB is not reachable via memberlist, will attempt reconnect",
					mlog.String("node_id", discovery.Id),
					mlog.String("hostname", discovery.Hostname),
					mlog.Int("last_ping_seconds_ago", secondsSinceLastPing))

				nodesToReconnect = append(nodesToReconnect, addr)
			} else {
				mlog.Debug("SznCluster: Node in DB is not reachable and hasn't pinged recently, skipping reconnect",
					mlog.String("node_id", discovery.Id),
					mlog.String("hostname", discovery.Hostname),
					mlog.Int("last_ping_seconds_ago", secondsSinceLastPing))
			}
		} else {
			// Node is in both DB and memberlist - verify address matches
			memberAddr := fmt.Sprintf("%s:%d", member.Addr.String(), member.Port)
			if memberAddr != addr {
				mlog.Debug("SznCluster: Node address mismatch between DB and memberlist",
					mlog.String("node_id", discovery.Id),
					mlog.String("db_addr", addr),
					mlog.String("memberlist_addr", memberAddr))
			}
		}
	}

	// Attempt to reconnect to missing nodes
	if len(nodesToReconnect) > 0 {
		mlog.Info("SznCluster: Attempting to reconnect to nodes",
			mlog.Int("node_count", len(nodesToReconnect)),
			mlog.Any("addresses", nodesToReconnect))

		joined, err := c.memberlist.Join(nodesToReconnect)
		if err != nil {
			mlog.Warn("SznCluster: Failed to rejoin nodes during health check",
				mlog.Err(err),
				mlog.Int("attempted", len(nodesToReconnect)))
		} else if joined > 0 {
			mlog.Info("SznCluster: Successfully reconnected to nodes",
				mlog.Int("reconnected_count", joined))
		}
	}

	// Cleanup failed sends for nodes that are back in memberlist
	c.failedMu.Lock()
	for nodeID := range c.failedSends {
		if _, ok := memberMap[nodeID]; ok {
			delete(c.failedSends, nodeID)
		}
	}
	c.failedMu.Unlock()

	mlog.Debug("SznCluster: Health check completed",
		mlog.Int("db_nodes", len(discoveries)),
		mlog.Int("memberlist_nodes", len(members)),
		mlog.Int("reconnect_attempted", len(nodesToReconnect)))
}

// hasOtherNodes checks if there are any other nodes in the cluster (besides this node)
// Returns true if there are other nodes that could potentially receive messages
func (c *SznCluster) hasOtherNodes() bool {
	if c.memberlist == nil {
		return false
	}

	members := c.memberlist.Members()
	// If we have more than 1 member (including ourselves), there are other nodes
	return len(members) > 1
}

// sendToAllNodes sends data to all nodes in the cluster
// Sends only to nodes currently in memberlist
// Recovery of temporarily unavailable nodes is handled by checkClusterHealth()
func (c *SznCluster) sendToAllNodes(data []byte) {
	if c.memberlist == nil {
		return
	}

	now := model.GetMillis()
	successCount := 0
	failCount := 0

	// Send to all nodes in memberlist
	members := c.memberlist.Members()
	for _, member := range members {
		if member.Name == c.nodeID {
			continue // Skip ourselves
		}

		if err := c.memberlist.SendBestEffort(member, data); err != nil {
			mlog.Warn("SznCluster: Failed to send to node",
				mlog.String("node_id", member.Name),
				mlog.Err(err))

			// Track failed send for monitoring
			c.failedMu.Lock()
			c.failedSends[member.Name] = now
			c.failedMu.Unlock()

			failCount++
		} else {
			successCount++
		}
	}

	if failCount > 0 {
		mlog.Warn("SznCluster: Send completed with failures",
			mlog.Int("success", successCount),
			mlog.Int("failed", failCount),
			mlog.Int("total_members", len(members)))
	}
}

// messageHash creates a hash of the cluster message for deduplication
func (c *SznCluster) messageHash(msg *model.ClusterMessage) string {
	h := sha256.New()
	h.Write([]byte(msg.Event))
	h.Write(msg.Data)
	return hex.EncodeToString(h.Sum(nil))
}

// isDuplicateOrMarkSeen atomically checks if message is duplicate and marks it as seen if not.
// Returns true if message is a duplicate (seen within last 5 seconds), false if new.
// If false is returned, the message is automatically marked as seen.
func (c *SznCluster) isDuplicateOrMarkSeen(hash string) bool {
	c.seenMu.Lock()
	defer c.seenMu.Unlock()

	now := model.GetMillis()
	if timestamp, exists := c.seenMessages[hash]; exists {
		// Message is duplicate if seen within deduplication window
		if now-timestamp < deduplicationWindowMs {
			return true
		}
	}

	// Not a duplicate - mark as seen and return false
	c.seenMessages[hash] = now
	return false
}

// cleanupSeenMessages removes old entries from the deduplication cache
// This should be called periodically (e.g., every 10 seconds)
func (c *SznCluster) cleanupSeenMessages() {
	c.seenMu.Lock()
	defer c.seenMu.Unlock()

	cutoff := model.GetMillis() - deduplicationWindowMs
	deleted := 0

	// Direct deletion during iteration is safe in Go
	for hash, timestamp := range c.seenMessages {
		if timestamp < cutoff {
			delete(c.seenMessages, hash)
			deleted++
		}
	}

	if deleted > 0 {
		mlog.Debug("SznCluster: Cleaned up seen messages cache",
			mlog.Int("deleted", deleted),
			mlog.Int("remaining", len(c.seenMessages)))
	}
}

// startDeduplicationCleanup runs periodic cleanup of the deduplication cache
func (c *SznCluster) startDeduplicationCleanup() {
	ticker := time.NewTicker(cleanupIntervalSec * time.Second)
	defer ticker.Stop()

	mlog.Info("SznCluster: Started deduplication cleanup routine")

	for {
		select {
		case <-ticker.C:
			c.cleanupSeenMessages()
		case <-c.shutdownCh:
			mlog.Info("SznCluster: Stopping deduplication cleanup routine")
			return
		}
	}
}
