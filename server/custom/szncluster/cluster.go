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
	"crypto/md5"
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

// clusterBroadcast implements memberlist.Broadcast interface
// for reliable message broadcasting with automatic retransmission and expiration
type clusterBroadcast struct {
	msg    []byte
	notify chan<- struct{}
}

// Invalidates checks if this broadcast invalidates another broadcast
// For cluster messages, we don't invalidate other messages
func (b *clusterBroadcast) Invalidates(other memberlist.Broadcast) bool {
	return false
}

// Message returns the message payload
func (b *clusterBroadcast) Message() []byte {
	return b.msg
}

// Finished is called when the message will no longer be broadcast
// either due to invalidation or reaching the transmit limit
func (b *clusterBroadcast) Finished() {
	if b.notify != nil {
		close(b.notify)
	}
}

// Cluster configuration constants
const (
	// Memberlist gossip protocol configuration
	// These values configure the behavior of Hashicorp Memberlist

	// retransmitMult is the multiplier for message retransmissions
	// Messages are retransmitted RetransmitMult * log(N+1) times before expiry
	// This value MUST match memberlist.Config.RetransmitMult for proper message lifecycle
	// Used by both memberlist gossip protocol and TransmitLimitedQueue
	retransmitMult = 4

	// gossipNodes is the number of random nodes to gossip to per interval
	// Higher values = faster propagation but more bandwidth
	// For 2-3 node clusters, 3 means all nodes are selected each iteration
	gossipNodes = 3

	// gossipIntervalMs is how often gossip messages are sent (in milliseconds)
	// Default is 200ms, we use 400ms as compromise between speed and bandwidth
	gossipIntervalMs = 400

	// probeIntervalSec is how often to probe a random member for health check (in seconds)
	// Shorter interval = faster failure detection but more network traffic
	probeIntervalSec = 3

	// probeTimeoutSec is the timeout waiting for probe ACK (in seconds)
	// Should be less than probeIntervalSec
	probeTimeoutSec = 2

	// suspicionMult is the multiplier for suspicion timeout
	// A node is marked dead after suspicionMult failed probes
	// suspicionMult=4 means ~12-15 seconds to detect failure
	suspicionMult = 4

	// pushPullIntervalSec is the interval for full state synchronization (in seconds)
	// Push/Pull provides anti-entropy and self-healing
	// Default is 30s, we use 20s for faster convergence
	pushPullIntervalSec = 20

	// udpBufferSize is the UDP buffer size in bytes for memberlist gossip protocol
	// Default is 1400 bytes which is too small for WebSocket events with post content
	// Messages larger than this will still be delivered via TCP (sendToAllNodes) but won't
	// benefit from UDP gossip retransmission mechanism
	udpBufferSize = 1400

	// maxUdpBroadcastSize is the maximum size for messages to be queued for UDP gossip
	// Set to smaller as udpBufferSize to account for overhead (compound message header, encryption, etc.)
	// Messages larger than this will skip UDP queue but still be delivered via TCP
	maxUdpBroadcastSize = int(udpBufferSize - 100)

	// Application-level configuration

	// deduplicationWindowMs is the time window in milliseconds for detecting duplicate messages
	// Increased to 30 seconds to account for gossip retransmission delays
	// With retransmitMult=4 and typical cluster size (2-3 nodes), messages can be
	// retransmitted for up to ~20 seconds, so 30s provides safe margin
	deduplicationWindowMs = 30000

	// cleanupIntervalSec is the interval in seconds for cleaning up the deduplication cache
	cleanupIntervalSec = 10

	// queuePruneIntervalSec is the interval in seconds for pruning the broadcast queue
	// This prevents unbounded queue growth under high load
	queuePruneIntervalSec = 30

	// maxQueueRetain is the maximum number of messages to retain in the queue during pruning
	// Older messages beyond this limit will be discarded
	maxQueueRetain = 5000

	// discoveryUpdateIntervalSec is the interval in seconds for updating cluster discovery in DB
	discoveryUpdateIntervalSec = 15

	// seedReconcileIntervalSec is the interval in seconds for reconciling seed list from DB
	// This enables automatic rejoin after long network partitions without process restart
	seedReconcileIntervalSec = 60
)

// SznCluster implements einterfaces.ClusterInterface using Hashicorp Memberlist
//
// Thread Safety:
// - All public methods are thread-safe and can be called concurrently
// - broadcasts (TransmitLimitedQueue) is internally thread-safe
// - seenMessages and handlers are protected by RW mutexes
// - memberlist operations are thread-safe per memberlist documentation
//
// Message Delivery Guarantees:
//   - ClusterSendReliable: Uses gossip protocol with automatic retransmission
//     Messages are delivered with high probability but order is NOT guaranteed
//     RetransmitMult * log(N+1) retransmissions ensure eventual delivery
//     Each gossip iteration sends to gossipNodes random members, not all members
//     Same node may receive message multiple times (deduplication handles this)
//   - ClusterSendBestEffort: Single UDP send, may be lost, no retransmission
//
// Gossip Protocol Behavior:
// - Broadcasts are NOT sent to all nodes at once
// - Every gossipIntervalMs, messages are sent to gossipNodes random members
// - Over retransmitMult * log(N+1) iterations, probabilistically reaches all nodes
// - Same node may be selected multiple times across iterations (by design)
// - Messages expire after max retransmit count, typically 5-15 seconds
//
// Deduplication:
// - Messages are deduplicated within 30-second window based on hash
// - Essential because gossip may deliver same message to a node multiple times
// - Prevents processing duplicate messages from different gossip paths
// - Cache is automatically cleaned every 10 seconds
//
// Database Discovery:
// - ClusterDiscovery table is primarily a SEED LIST for bootstrap
// - Not a source of truth for current cluster state (memberlist is)
// - Updated every 15s for system console visibility and node discovery
// - Used by new nodes to find existing cluster members on startup
// - System admins can monitor cluster state via system console
type SznCluster struct {
	platform   *platform.PlatformService
	memberlist *memberlist.Memberlist
	delegate   *clusterDelegate
	events     *clusterEvents

	// Node identification
	nodeID   string
	hostname string

	// Leader tracking (protected by leaderMu)
	lastKnownLeader string
	leaderMu        sync.RWMutex

	// Message handlers (protected by handlersMu)
	handlers   map[model.ClusterEvent]einterfaces.ClusterMessageHandler
	handlersMu sync.RWMutex

	// Broadcast queue for outgoing messages using memberlist's TransmitLimitedQueue
	// This automatically handles retransmission limits and message expiration
	// Thread-safe: TransmitLimitedQueue is internally synchronized
	broadcasts *memberlist.TransmitLimitedQueue

	// Deduplication cache for incoming messages (protected by seenMu)
	seenMessages map[string]int64 // hash -> timestamp (milliseconds)
	seenMu       sync.RWMutex

	// State (protected by startMu)
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
		platform:     ps,
		hostname:     hostname,
		handlers:     make(map[model.ClusterEvent]einterfaces.ClusterMessageHandler),
		seenMessages: make(map[string]int64),
		shutdownCh:   make(chan struct{}),
	}

	// Initialize TransmitLimitedQueue for broadcasts
	// This handles automatic retransmission and expiration of messages
	cluster.broadcasts = &memberlist.TransmitLimitedQueue{
		NumNodes: func() int {
			if cluster.memberlist == nil {
				return 1
			}
			return cluster.memberlist.NumMembers()
		},
		RetransmitMult: retransmitMult, // Shared constant with memberlist config
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

	// Start periodic seed-list reconciliation for partition recovery
	go c.startSeedReconciliation()

	// Start periodic cleanup of deduplication cache
	go c.startDeduplicationCleanup()

	c.started = true

	// Initialize leader tracking after cluster is started
	c.leaderMu.Lock()
	c.lastKnownLeader = c.getCurrentLeader()
	isInitialLeader := c.lastKnownLeader == c.nodeID
	c.leaderMu.Unlock()

	memberCount := 0
	if c.memberlist != nil {
		memberCount = c.memberlist.NumMembers()
	}

	mlog.Info("SznCluster: Inter-node communication started successfully",
		mlog.String("initial_leader", c.lastKnownLeader),
		mlog.Bool("this_node_is_leader", isInitialLeader),
		mlog.String("this_node_id", c.nodeID),
		mlog.Int("member_count", memberCount))

	// CRITICAL: If we are the leader on startup, invoke listeners immediately
	// This ensures that jobs start on the leader node even when cluster starts
	// without any leader changes. Without this, jobs would never start if the
	// leader node starts and never loses leadership.
	// IMPORTANT: Must run in goroutine to avoid deadlock during startup
	if isInitialLeader {
		mlog.Info("SznCluster: This node is initial leader, invoking listeners for job startup")
		go c.platform.InvokeClusterLeaderChangedListeners()
	}
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
// Uses cached leader information to avoid race conditions with memberlist updates
func (c *SznCluster) IsLeader() bool {
	if !c.started || c.memberlist == nil {
		return false
	}

	// Use cached leader information to ensure consistency
	// This avoids race conditions where memberlist.Members() might return
	// different results between checkAndNotifyLeaderChange() and listener callbacks
	c.leaderMu.RLock()
	currentLeader := c.lastKnownLeader
	c.leaderMu.RUnlock()

	// If no leader tracked yet (e.g., during startup before cluster join),
	// fall back to computing from memberlist
	if currentLeader == "" {
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

	return currentLeader == c.nodeID
}

// getCurrentLeader returns the current leader node ID
func (c *SznCluster) getCurrentLeader() string {
	if !c.started || c.memberlist == nil {
		return c.nodeID // We're the only node
	}

	members := c.memberlist.Members()
	if len(members) == 0 {
		return c.nodeID // We're the only node
	}

	// Find node with smallest name (ID)
	leaderName := c.nodeID
	for _, member := range members {
		if member.Name < leaderName {
			leaderName = member.Name
		}
	}

	return leaderName
}

// checkAndNotifyLeaderChange checks if the leader has changed and notifies listeners
func (c *SznCluster) checkAndNotifyLeaderChange() {
	currentLeader := c.getCurrentLeader()

	c.leaderMu.Lock()
	previousLeader := c.lastKnownLeader
	c.lastKnownLeader = currentLeader
	c.leaderMu.Unlock()

	// If leader changed, notify platform to invoke listeners
	// IMPORTANT: Must run in goroutine to avoid deadlock when called from memberlist callbacks
	if previousLeader != "" && previousLeader != currentLeader {
		mlog.Info("SznCluster: Leader changed",
			mlog.String("previous_leader", previousLeader),
			mlog.String("new_leader", currentLeader),
			mlog.Bool("this_node_is_leader", currentLeader == c.nodeID))
		go c.platform.InvokeClusterLeaderChangedListeners()
	}
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

	// Compute config hash for web UI display (similar to enterprise implementation)
	// This is used to detect configuration drift between cluster nodes
	configHash := computeConfigHash(c.platform.Config())

	// Get schema version for cluster sync verification
	_, schemaVersion, err := c.platform.DatabaseTypeAndSchemaVersion()
	if err != nil {
		mlog.Warn("SznCluster: Failed to get schema version", mlog.Err(err))
		schemaVersion = ""
	}

	return &model.ClusterInfo{
		Id:            c.nodeID,
		IPAddress:     c.getAdvertiseAddress(), // Where to connect
		Hostname:      c.getHostname(),         // Display name
		Version:       model.CurrentVersion,
		ConfigHash:    configHash,
		SchemaVersion: schemaVersion,
	}
}

// computeConfigHash computes an MD5 hash of the configuration for drift detection
// This matches the behavior of the enterprise cluster implementation
func computeConfigHash(config *model.Config) string {
	// Use ToJSON to get a stable representation of the config
	jsonConfig, err := json.Marshal(config)
	if err != nil {
		mlog.Warn("SznCluster: Failed to marshal config for hash", mlog.Err(err))
		return ""
	}

	// Compute MD5 hash (matches enterprise implementation)
	hash := md5.Sum(jsonConfig)
	return fmt.Sprintf("%x", hash)
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
			// Parse metadata to get version, node ID, hostname, advertise address, config hash and schema version
			version := model.CurrentVersion
			nodeID := member.Name
			hostname := member.Addr.String()         // Default to IP if not in metadata
			advertiseAddress := member.Addr.String() // Default to IP if not in metadata
			configHash := ""
			schemaVersion := ""

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
					if hash, ok := meta["config_hash"]; ok {
						configHash = hash
					}
					if schema, ok := meta["schema_version"]; ok {
						schemaVersion = schema
					}
				}
			}

			// Add info from other nodes
			infos = append(infos, &model.ClusterInfo{
				Id:            nodeID,
				IPAddress:     advertiseAddress, // Use advertise address (where to connect)
				Hostname:      hostname,         // Use hostname (for display/debug)
				Version:       version,
				ConfigHash:    configHash,
				SchemaVersion: schemaVersion,
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

	// Measure request duration for metrics
	startTime := time.Now()
	defer func() {
		if metrics := c.platform.Metrics(); metrics != nil {
			metrics.IncrementClusterRequest()
			metrics.ObserveClusterRequestDuration(time.Since(startTime).Seconds())
		}
	}()

	mlog.Debug("SznCluster: Broadcasting message", mlog.String("event", string(msg.Event)))

	// Serialize the message
	data, err := json.Marshal(msg)
	if err != nil {
		mlog.Error("SznCluster: Failed to serialize message", mlog.Err(err))
		return
	}

	// Choose delivery strategy based on SendType:
	// - ClusterSendReliable: Hybrid "eager + reliable" approach
	//   Queue for gossip retransmission (if size allows) + send immediately via TCP
	// - ClusterSendBestEffort: Send once directly via TCP, no queuing

	// For reliable messages, queue for UDP gossip retransmission if size allows
	// Messages larger than maxUdpBroadcastSize would never be transmitted by memberlist
	// (GetBroadcasts skips them), so we skip queueing to avoid filling the queue
	if msg.SendType == model.ClusterSendReliable && c.memberlist != nil && c.memberlist.NumMembers() > 1 {
		if len(data) <= maxUdpBroadcastSize {
			c.broadcasts.QueueBroadcast(&clusterBroadcast{
				msg: data,
			})
			mlog.Debug("SznCluster: Message queued for UDP gossip retransmit",
				mlog.String("event", string(msg.Event)),
				mlog.Int("queue_size", c.broadcasts.NumQueued()),
				mlog.Int("message_size_bytes", len(data)))
		} else {
			mlog.Debug("SznCluster: Message too large for UDP gossip, TCP only",
				mlog.String("event", string(msg.Event)),
				mlog.Int("message_size_bytes", len(data)),
				mlog.Int("max_udp_size", maxUdpBroadcastSize))
		}
	}

	// Always send immediately to all nodes for instant delivery
	// (both Reliable and BestEffort)
	c.sendToAllNodes(data)
	mlog.Debug("SznCluster: Message sent immediately",
		mlog.String("event", string(msg.Event)),
		mlog.String("send_type", msg.SendType))
}

// SendClusterMessageToNode sends a message to a specific node
func (c *SznCluster) SendClusterMessageToNode(nodeID string, msg *model.ClusterMessage) error {
	// Measure request duration for metrics
	startTime := time.Now()
	defer func() {
		if metrics := c.platform.Metrics(); metrics != nil {
			metrics.IncrementClusterRequest()
			metrics.ObserveClusterRequestDuration(time.Since(startTime).Seconds())
		}
	}()

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
	if err := c.memberlist.SendReliable(targetNode, data); err != nil {
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

	// Report cluster event type metric
	if metrics := c.platform.Metrics(); metrics != nil {
		metrics.IncrementClusterEventType(msg.Event)
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

	// Create ticker for periodic updates
	c.discoveryTicker = time.NewTicker(discoveryUpdateIntervalSec * time.Second)
	defer c.discoveryTicker.Stop()

	for {
		select {
		case <-c.discoveryTicker.C:
			c.updateClusterDiscovery(clusterName)
		case <-c.shutdownCh:
			// Cleanup on shutdown
			c.cleanupClusterDiscovery()
			return
		}
	}
}

// startSeedReconciliation periodically attempts to rejoin nodes discovered from DB seed list.
// This complements memberlist by handling long-lived split-brain scenarios where both sides
// consider themselves single-node clusters and no membership events are generated anymore.
func (c *SznCluster) startSeedReconciliation() {
	ticker := time.NewTicker(seedReconcileIntervalSec * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.reconcileSeedList()
		case <-c.shutdownCh:
			return
		}
	}
}

// reconcileSeedList attempts to join peers from ClusterDiscovery when this node is isolated.
// DB filtering (CDSOfflineAfterMillis) is applied by discoverNodes/GetAll.
func (c *SznCluster) reconcileSeedList() {
	if c.memberlist == nil {
		return
	}

	// Only reconcile when isolated or effectively single-node.
	// If we already have peers, avoid unnecessary join traffic.
	if c.memberlist.NumMembers() > 1 {
		return
	}

	nodes, err := c.discoverNodes()
	if err != nil {
		mlog.Warn("SznCluster: Seed reconciliation failed to discover nodes", mlog.Err(err))
		return
	}

	if len(nodes) == 0 {
		mlog.Debug("SznCluster: Seed reconciliation found no candidate nodes")
		return
	}

	joined, err := c.memberlist.Join(nodes)
	if err != nil {
		mlog.Debug("SznCluster: Seed reconciliation join attempt failed",
			mlog.Int("candidate_count", len(nodes)),
			mlog.Err(err))
		return
	}

	if joined > 0 {
		mlog.Info("SznCluster: Seed reconciliation joined nodes",
			mlog.Int("joined_count", joined),
			mlog.Int("candidate_count", len(nodes)))
	}
}

// updateClusterDiscovery updates this node's entry in the cluster discovery table
//
// Purpose:
//  1. SEED LIST: Provides bootstrap discovery for new nodes joining the cluster
//  2. VISIBILITY: Allows system admins to monitor cluster state via system console
//  3. PERSISTENCE: Maintains cluster membership info across restarts
//
// Note: This is NOT the source of truth for current cluster state.
// Memberlist maintains the authoritative live member list in memory.
// The DB serves as a seed list and monitoring interface.
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

// Note: hasOtherNodes() has been removed as it was redundant.
// Use c.memberlist.NumMembers() > 1 directly for better performance.

// sendToAllNodes sends data to all nodes in the cluster
// Sends only to nodes currently in memberlist
// Memberlist automatically handles node failure detection and recovery via probes and anti-entropy
func (c *SznCluster) sendToAllNodes(data []byte) {
	if c.memberlist == nil {
		return
	}

	// Send to all live nodes in memberlist
	// Note: memberlist.Members() only returns nodes that are currently alive
	// Failed/dead nodes are automatically excluded by memberlist's health checking
	members := c.memberlist.Members()
	successCount := 0
	failCount := 0

	for _, member := range members {
		if member.Name == c.nodeID {
			continue // Skip ourselves
		}

		if err := c.memberlist.SendReliable(member, data); err != nil {
			// Note: This is rare since Members() only returns live nodes
			// Could happen due to network issues or node failure between Members() call and send
			mlog.Warn("SznCluster: Failed to send to node",
				mlog.String("node_id", member.Name),
				mlog.Err(err))
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
// and prunes the broadcast queue to prevent unbounded growth
func (c *SznCluster) startDeduplicationCleanup() {
	cleanupTicker := time.NewTicker(cleanupIntervalSec * time.Second)
	defer cleanupTicker.Stop()

	pruneTicker := time.NewTicker(queuePruneIntervalSec * time.Second)
	defer pruneTicker.Stop()

	metricsTicker := time.NewTicker(60 * time.Second) // Log metrics every minute
	defer metricsTicker.Stop()

	mlog.Info("SznCluster: Started maintenance routines",
		mlog.Int("cleanup_interval_sec", cleanupIntervalSec),
		mlog.Int("prune_interval_sec", queuePruneIntervalSec))

	for {
		select {
		case <-cleanupTicker.C:
			c.cleanupSeenMessages()

		case <-pruneTicker.C:
			c.pruneQueueIfNeeded()

		case <-metricsTicker.C:
			c.logQueueMetrics()

		case <-c.shutdownCh:
			mlog.Info("SznCluster: Stopping maintenance routines")
			return
		}
	}
}

// pruneQueueIfNeeded prunes the broadcast queue if it exceeds the retention limit
func (c *SznCluster) pruneQueueIfNeeded() {
	if c.broadcasts == nil {
		return
	}

	queueSize := c.broadcasts.NumQueued()
	if queueSize > maxQueueRetain {
		c.broadcasts.Prune(maxQueueRetain)
		mlog.Warn("SznCluster: Pruned broadcast queue",
			mlog.Int("old_size", queueSize),
			mlog.Int("new_size", c.broadcasts.NumQueued()),
			mlog.Int("max_retain", maxQueueRetain))
	}
}

// logQueueMetrics logs queue and cluster metrics for monitoring
func (c *SznCluster) logQueueMetrics() {
	if !c.started {
		return
	}

	queueSize := 0
	if c.broadcasts != nil {
		queueSize = c.broadcasts.NumQueued()
	}

	c.seenMu.RLock()
	seenCount := len(c.seenMessages)
	c.seenMu.RUnlock()

	memberCount := 0
	if c.memberlist != nil {
		memberCount = c.memberlist.NumMembers()
	}

	mlog.Info("SznCluster: Queue metrics",
		mlog.Int("broadcast_queue_size", queueSize),
		mlog.Int("dedup_cache_size", seenCount),
		mlog.Int("cluster_members", memberCount),
		mlog.Bool("is_leader", c.IsLeader()),
		mlog.Int("health_score", c.HealthScore()))
}
