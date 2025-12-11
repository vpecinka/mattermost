# SznCluster - Custom Mattermost Cluster Implementation

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [How It Works](#how-it-works)
- [SWIM Gossip Protocol](#swim-gossip-protocol)
- [Implementation Details](#implementation-details)
- [Configuration](#configuration)
- [Deployment Examples](#deployment-examples)
- [API Reference](#api-reference)
- [Testing and Verification](#testing-and-verification)
- [Troubleshooting](#troubleshooting)
- [Limitations](#limitations)
- [License](#license)

## Overview

SznCluster is a custom implementation of the Mattermost cluster interface, developed for Seznam.cz, a.s. for internal use. This implementation uses Hashicorp Memberlist for the gossip protocol and provides full clustering functionality required by the Mattermost ClusterInterface **without requiring an enterprise license**.

### Key Features

- **Zero Enterprise License Required** - Full clustering functionality using open-source components
- **SWIM Gossip Protocol** - Scalable Weakly-consistent Infection-style Membership protocol via Hashicorp Memberlist
- **Automatic Node Discovery** - Database-backed discovery with automatic cleanup (10-minute timeout)
- **Fast Recovery** - 3-second initial health check, 20-second periodic checks for quick node recovery
- **NodeID-Based Mapping** - Robust node identification survives IP/hostname changes (via discovery.Id)
- **Database as Source of Truth** - DB discovery table drives reconnection logic for temporarily unavailable nodes
- **Failed Send Tracking** - Monitors send failures for debugging and health monitoring
- **Leader Election** - Deterministic leader selection based on node ID
- **Reliable Message Broadcasting** - Hybrid delivery: immediate send + gossip retransmission for reliability
- **Message Deduplication** - Hash-based deduplication (5-second window) prevents duplicate processing
- **Health Monitoring** - Real-time cluster health tracking using memberlist health scores
- **Docker/NAT Support** - Explicit bind vs advertise address handling for containerized environments
- **Graceful Shutdown** - Proper cleanup and leave notifications
- **Message Routing** - Event-based message routing to registered handlers
- **Integrated Logging** - Memberlist logs forwarded to Mattermost logger with level filtering
- **Cluster Statistics** - Real-time statistics about cluster members and state

### Why SznCluster?

Mattermost's native clustering requires an Enterprise license. For internal deployments where enterprise features are not needed but high availability is required, SznCluster provides:

1. **Cost savings** - No enterprise license fees
2. **Full control** - Complete access to clustering code
3. **Customization** - Ability to modify and extend as needed
4. **Standard protocol** - Uses well-tested Hashicorp Memberlist (used by Consul, Nomad, etc.)

## Architecture

### Components

SznCluster consists of four main Go files:

#### 1. **cluster.go** (771 lines)
Main implementation of the ClusterInterface:
- `SznCluster` struct - implements `einterfaces.ClusterInterface`
- Message broadcasting and routing (with immediate send + gossip retransmission)
- Message deduplication (hash-based cache with periodic cleanup)
- Leader election algorithm
- Cluster discovery service
- Node lifecycle management
- Handler registration and dispatch

#### 2. **delegate.go** (120 lines)
Memberlist delegate implementations:
- `clusterDelegate` - implements `memberlist.Delegate` interface
  - `NodeMeta()` - Node metadata exchange
  - `NotifyMsg()` - Incoming message handler
  - `GetBroadcasts()` - Broadcast queue management
  - `LocalState()` / `MergeRemoteState()` - State synchronization
- `clusterEvents` - implements `memberlist.EventDelegate` interface
  - `NotifyJoin()` - Node join notifications
  - `NotifyLeave()` - Node leave notifications
  - `NotifyUpdate()` - Node update notifications

#### 3. **memberlist.go** (302 lines)
Memberlist initialization and utilities:
- `initializeMemberlist()` - Memberlist configuration and creation
- `joinCluster()` - Cluster join logic
- `discoverNodes()` - Database-based node discovery
- `getLocalIP()` - Network interface detection
- `getHostname()` - Hostname resolution
- `memberlistLogger` - Logger wrapper integrating memberlist logs with Mattermost mlog
- Helper functions for serialization

#### 4. **init.go** (17 lines)
Automatic registration:
- `init()` function - Registers SznCluster with platform on import
- Called automatically when package is imported in `main.go`

### Component Interaction

```
┌─────────────────────────────────────────────────────────────┐
│                      Mattermost Server                       │
│  ┌────────────────────────────────────────────────────────┐ │
│  │              Platform Layer (platform.go)              │ │
│  │                                                        │ │
│  │    ClusterInterface registration point                │ │
│  └────────────────────┬───────────────────────────────────┘ │
│                       │                                      │
│                       │ RegisterClusterInterface()           │
│                       ▼                                      │
│  ┌────────────────────────────────────────────────────────┐ │
│  │                  SznCluster (cluster.go)               │ │
│  │  ┌──────────────────────────────────────────────────┐ │ │
│  │  │  • Message broadcasting                          │ │ │
│  │  │  • Leader election                               │ │ │
│  │  │  • Handler routing                               │ │ │
│  │  │  • Cluster discovery                             │ │ │
│  │  └──────────────────────────────────────────────────┘ │ │
│  │           │                    │                        │ │
│  │           │                    │                        │ │
│  │           ▼                    ▼                        │ │
│  │  ┌──────────────┐    ┌──────────────────┐             │ │
│  │  │  Delegates   │    │  Memberlist      │             │ │
│  │  │ (delegate.go)│◄──►│ (memberlist.go)  │             │ │
│  │  └──────────────┘    └──────────────────┘             │ │
│  └─────────────┬──────────────────┬──────────────────────┘ │
│                │                  │                         │
└────────────────┼──────────────────┼─────────────────────────┘
                 │                  │
                 │                  │
        ┌────────▼──────┐  ┌────────▼──────────┐
        │   Database    │  │  Gossip Network   │
        │  (Discovery)  │  │  (UDP/TCP 8074)   │
        └───────────────┘  └───────────────────┘
                                    │
                     ┌──────────────┼──────────────┐
                     │              │              │
                ┌────▼────┐    ┌────▼────┐   ┌────▼────┐
                │ Node 1  │    │ Node 2  │   │ Node 3  │
                └─────────┘    └─────────┘   └─────────┘
```

## How It Works

### Initialization Flow

1. **Package Import** (`main.go`)
   ```go
   import _ "github.com/mattermost/mattermost/server/v8/custom/szncluster"
   ```
   The underscore import triggers the `init()` function automatically.

2. **Registration** (`init.go`)
   ```go
   func init() {
       platform.RegisterClusterInterface(NewSznCluster)
       mlog.Info("SznCluster: Cluster interface registered")
   }
   ```
   Registers the SznCluster factory function with the platform layer.

3. **Instantiation** (platform layer)
   When Mattermost starts and clustering is enabled, the platform layer calls `NewSznCluster(ps *platform.PlatformService)`.

4. **Startup** (`StartInterNodeCommunication()`)
   - Initialize Memberlist with configuration
   - Discover existing nodes from database
   - Join the cluster
   - Start discovery service (periodic database updates)

### Message Flow

#### Broadcasting Messages

```
Application Code
      │
      ├─► cluster.SendClusterMessage(msg)
      │         │
      │         ├─► Serialize message to JSON
      │         │
      │         ├─► If ClusterSendReliable:
      │         │    └─► Add to broadcast queue (for gossip retransmission)
      │         │              │
      │         │              └─► GetBroadcasts() periodically reads queue
      │         │                        │
      │         │                        └─► Memberlist sends via gossip protocol
      │         │
      │         └─► Send immediately to all nodes (both Reliable & BestEffort)
      │                    │
      │                    └─► SendBestEffort() UDP to each node
      │
      └─► All cluster nodes receive message (with deduplication)
```

#### Receiving Messages

```
Memberlist (UDP packet received)
      │
      ├─► delegate.NotifyMsg(buf)
      │         │
      │         └─► cluster.NotifyMsg(buf)
      │                   │
      │                   ├─► Deserialize JSON
      │                   │
      │                   ├─► Compute message hash (SHA256)
      │                   │
      │                   ├─► Check deduplication cache
      │                   │    ├─► If seen recently (< 2s): DROP
      │                   │    └─► If new: mark as seen, continue
      │                   │
      │                   ├─► Lookup handler for event
      │                   │
      │                   └─► Call handler(msg)
      │                             │
      │                             └─► Application handles message
```

### Cluster Discovery

SznCluster uses the database as a rendezvous point for node discovery:

1. **Discovery Service** (runs every 30 seconds)
   ```go
   func startClusterDiscovery() {
       ticker := time.NewTicker(30 * time.Second)
       for {
           select {
           case <-ticker.C:
               updateClusterDiscovery()  // Write to DB
           case <-shutdownCh:
               cleanupClusterDiscovery() // Remove from DB
               return
           }
       }
   }
   ```

2. **Node Startup**
   - Query `ClusterDiscovery` table for other nodes
   - Extract hostnames/IPs
   - Attempt to join discovered nodes via Memberlist

3. **Advantages**
   - No need for static node list
   - Automatic discovery of new nodes
   - Works across network restarts
   - Survives temporary network partitions

### Leader Election

Simple deterministic algorithm:

```go
func (c *SznCluster) IsLeader() bool {
    members := c.memberlist.Members()
    leaderName := c.nodeID
    
    for _, member := range members {
        if member.Name < leaderName {
            leaderName = member.Name
        }
    }
    
    return leaderName == c.nodeID
}
```

- Node with lexicographically smallest ID becomes leader
- Deterministic - all nodes agree on leader
- Automatic failover when leader node leaves
- No consensus protocol needed (eventual consistency is acceptable)

## SWIM Gossip Protocol

### What is SWIM?

SWIM (Scalable Weakly-consistent Infection-style Membership) is a gossip-based protocol for maintaining cluster membership. It's used by:
- HashiCorp Consul
- HashiCorp Nomad  
- HashiCorp Serf
- Apache Cassandra (similar protocol)

### Key Characteristics

1. **Scalability** - O(log n) detection time
2. **Failure Detection** - Distinguishes between slow and dead nodes
3. **Weak Consistency** - Eventually consistent membership
4. **Low Network Overhead** - Bounded bandwidth usage

### Protocol Mechanics

#### 1. Membership Protocol

```
Every T seconds (probe interval):
  1. Node A randomly selects node B
  2. Node A sends PING to B
  3. If B responds with ACK → B is alive
  4. If B doesn't respond:
     a. Node A asks K random nodes to PING B (indirect probe)
     b. If any responds with ACK from B → B is alive
     c. If no response → B is declared suspicious
  5. After timeout, suspicious nodes are declared dead
```

#### 2. Dissemination Protocol

When membership changes (joins, leaves, failures):
- Gossip the change to random subset of nodes
- Each node forwards to more random nodes
- Information spreads exponentially
- Entire cluster learns about changes in O(log n) time

#### 3. Message Piggybacking

SWIM optimizes bandwidth by piggybacking:
- Membership updates on PING/ACK messages
- User messages on membership messages
- Multiple pieces of information in single packet

### SznCluster SWIM Implementation

```
Port 8074 (UDP + TCP)
├─ UDP: Used for gossip messages
│   ├─ PING/ACK (health checks)
│   ├─ User messages (cluster events)
│   └─ Membership updates
│
└─ TCP: Used for state synchronization
    ├─ Push/Pull full state
    ├─ Join protocol
    └─ Large message transfer
```

**Memberlist Configuration:**
```go
mlConfig := memberlist.DefaultLANConfig()
mlConfig.BindPort = 8074
mlConfig.GossipInterval = 200ms    // How often to gossip
mlConfig.ProbeInterval = 1s        // How often to probe random node
mlConfig.ProbeTimeout = 500ms      // Timeout for probe response
mlConfig.SuspicionMult = 4         // Suspicion timeout multiplier
```

## Implementation Details

### Thread Safety

All shared state is protected by mutexes:

```go
type SznCluster struct {
    // Protected by handlersMu
    handlers   map[model.ClusterEvent]einterfaces.ClusterMessageHandler
    handlersMu sync.RWMutex
    
    // Protected by queueMu
    broadcastQueue [][]byte
    queueMu        sync.Mutex
    
    // Protected by seenMu
    seenMessages map[string]int64  // hash -> timestamp
    seenMu       sync.RWMutex
    
    // Protected by startMu
    started bool
    startMu sync.Mutex
}
```

### Message Serialization

Messages use JSON encoding for simplicity:

```go
// Serialize
data, err := json.Marshal(msg)

// Deserialize
var msg model.ClusterMessage
err := json.Unmarshal(data, &msg)
```

**Future Enhancement:** Could use Protocol Buffers or MessagePack for better performance.

### Broadcast Queue and Message Delivery

Implements hybrid delivery strategy for reliability:

```go
// Send message
func SendClusterMessage(msg *model.ClusterMessage) {
    data := serialize(msg)
    
    if msg.SendType == ClusterSendReliable {
        // For reliable messages:
        // 1. Add to queue for gossip retransmission
        queueMu.Lock()
        broadcastQueue = append(broadcastQueue, data)
        queueMu.Unlock()
        
        // 2. Also send immediately for low latency
        sendToAllNodes(data)
    } else {
        // For best-effort: send once, don't queue
        sendToAllNodes(data)
    }
}

// Memberlist calls this periodically for gossip
func GetBroadcasts(overhead, limit int) [][]byte {
    // Return messages from queue that fit in limit
    // Remove returned messages from queue
    // This provides retransmission for reliable messages
}
```

**Why hybrid delivery?**
- **Immediate send** ensures low latency (messages arrive within milliseconds)
- **Gossip queue** provides redundancy and eventual delivery guarantees
- **Deduplication** prevents processing duplicate messages from both paths

### Message Deduplication

Since reliable messages are sent both immediately and via gossip, deduplication is essential to prevent duplicate processing.

#### Deduplication Strategy

```go
type SznCluster struct {
    seenMessages map[string]int64  // hash -> timestamp
    seenMu       sync.RWMutex
}

// Compute message hash
func (c *SznCluster) messageHash(msg *model.ClusterMessage) string {
    combined := fmt.Sprintf("%s:%s", msg.Event, string(msg.Data))
    hash := sha256.Sum256([]byte(combined))
    return hex.EncodeToString(hash[:])
}

// Check if message was recently seen
func (c *SznCluster) isDuplicate(hash string) bool {
    c.seenMu.RLock()
    defer c.seenMu.RUnlock()
    
    timestamp, exists := c.seenMessages[hash]
    if !exists {
        return false
    }
    
    // Consider duplicates only within 2-second window
    return time.Now().Unix() - timestamp < 2
}

// Mark message as seen
func (c *SznCluster) markAsSeen(hash string) {
    c.seenMu.Lock()
    defer c.seenMu.Unlock()
    c.seenMessages[hash] = time.Now().Unix()
}
```

#### Deduplication Flow

When a message arrives via `NotifyMsg()`:

```
1. Deserialize message
2. Compute SHA256 hash of Event + Data
3. Check if hash exists in seenMessages with timestamp < 2s ago
   └─ If YES: Drop message (log at DEBUG level)
   └─ If NO: Mark as seen and process
```

#### Periodic Cleanup

To prevent unbounded memory growth:

```go
func (c *SznCluster) cleanupSeenMessages() {
    cutoff := time.Now().Unix() - 2
    
    // Two-pass deletion: collect hashes first, then delete
    toDelete := make([]string, 0)
    c.seenMu.RLock()
    for hash, timestamp := range c.seenMessages {
        if timestamp < cutoff {
            toDelete = append(toDelete, hash)
        }
    }
    c.seenMu.RUnlock()
    
    // Delete collected hashes
    if len(toDelete) > 0 {
        c.seenMu.Lock()
        for _, hash := range toDelete {
            delete(c.seenMessages, hash)
        }
        c.seenMu.Unlock()
        mlog.Debug("Cleaned up seen messages cache", 
            mlog.Int("deleted_count", len(toDelete)))
    }
}

// Run cleanup every 10 seconds
func (c *SznCluster) startDeduplicationCleanup() {
    ticker := time.NewTicker(10 * time.Second)
    go func() {
        for range ticker.C {
            c.cleanupSeenMessages()
        }
    }()
}
```

#### Memory Footprint

Each cache entry uses approximately:
- **Hash**: 64 bytes (SHA256 hex string)
- **Timestamp**: 8 bytes (int64)
- **Map overhead**: ~8-16 bytes
- **Total**: ~72-88 bytes per message

For a busy cluster with 100 messages/second:
- 2-second window = ~200 entries = ~14-18 KB
- 10-second cleanup interval ensures low memory usage
- Old entries are automatically purged

### Memberlist Logger Integration

Memberlist uses Go's standard logger, which bypasses Mattermost's log level configuration. A custom logger wrapper filters memberlist logs through Mattermost's `mlog`:

```go
type memberlistLogger struct{}

func (l *memberlistLogger) Write(p []byte) (n int, err error) {
    msg := strings.TrimSpace(string(p))
    
    switch {
    case strings.Contains(msg, "[DEBUG]"):
        mlog.Debug(strings.TrimPrefix(msg, "[DEBUG] memberlist: "))
    case strings.Contains(msg, "[INFO]"):
        mlog.Info(strings.TrimPrefix(msg, "[INFO] memberlist: "))
    case strings.Contains(msg, "[WARN]"):
        mlog.Warn(strings.TrimPrefix(msg, "[WARN] memberlist: "))
    case strings.Contains(msg, "[ERROR]"):
        mlog.Error(strings.TrimPrefix(msg, "[ERROR] memberlist: "))
    }
    
    return len(p), nil
}

// Configure memberlist to use custom logger
mlConfig.Logger = log.New(&memberlistLogger{}, "", 0)
```

**Benefits:**
- Memberlist DEBUG logs respect Mattermost log level settings
- All logs appear in unified JSON format
- Consistent log filtering and routing
- No unwanted DEBUG output in production

### Health Monitoring

Uses Memberlist's built-in health score:

```go
func HealthScore() int {
    // 0 = healthy, increases with issues
    return c.memberlist.GetHealthScore()
}
```

Memberlist health score considers:
- Failed ping attempts
- Slow responses
- Network congestion
- Suspicious node count

### Graceful Shutdown

Proper cleanup sequence:

```go
func StopInterNodeCommunication() {
    close(shutdownCh)                    // Stop discovery service
    memberlist.Leave(timeout)            // Notify cluster of departure
    memberlist.Shutdown()                // Close connections
    cleanupClusterDiscovery()            // Remove from database
}
```

## Configuration

### Minimal Configuration

```json
{
  "ServiceSettings": {
    "SiteURL": "http://mattermost.example.com",
    "ListenAddress": ":8065"
  },
  "ClusterSettings": {
    "Enable": true,
    "ClusterName": "production",
    "UseIPAddress": true,
    "GossipPort": 8074,
    "EnableGossipCompression": true
  }
}
```

### Configuration with Encryption

```json
{
  "ClusterSettings": {
    "Enable": true,
    "ClusterName": "production",
    "UseIPAddress": true,
    "GossipPort": 8074,
    "EnableGossipEncryption": true,
    "EnableGossipCompression": true
  }
}
```

**Note:** Encryption key configuration needs to be added to the implementation.

### Multi-Interface Configuration

For servers with multiple network interfaces:

```json
{
  "ClusterSettings": {
    "Enable": true,
    "ClusterName": "production",
    "BindAddress": "10.0.1.10",           // Internal interface
    "AdvertiseAddress": "192.168.1.10",  // External/advertised address
    "GossipPort": 8074,
    "EnableGossipCompression": true
  }
}
```

### Hostname Override

```json
{
  "ClusterSettings": {
    "Enable": true,
    "ClusterName": "production",
    "OverrideHostname": "mattermost-node-1.example.com",
    "GossipPort": 8074,
    "EnableGossipCompression": true
  }
}
```

### Configuration Options Reference

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `Enable` | bool | false | Enable clustering |
| `ClusterName` | string | "" | Cluster identifier (nodes with same name cluster together) |
| `GossipPort` | int | 8074 | UDP/TCP port for gossip protocol |
| `BindAddress` | string | "0.0.0.0" | IP address to bind to |
| `AdvertiseAddress` | string | "" | IP address to advertise to other nodes |
| `OverrideHostname` | string | "" | Override OS hostname |
| `UseIPAddress` | bool | false | Use IP address instead of hostname |
| `EnableGossipEncryption` | bool | false | Enable gossip encryption (TBD: key config) |
| `EnableGossipCompression` | bool | false | Enable message compression |
| `ClusterDiscoveryDuration` | int | 60000 | Discovery check interval (ms) |

## Deployment Examples

### Two-Node Setup

#### Node 1: node1.example.com
```json
{
  "ServiceSettings": {
    "SiteURL": "http://mattermost.example.com",
    "ListenAddress": ":8065"
  },
  "ClusterSettings": {
    "Enable": true,
    "ClusterName": "production",
    "OverrideHostname": "node1.example.com",
    "UseIPAddress": false,
    "GossipPort": 8074,
    "EnableGossipCompression": true
  },
  "SqlSettings": {
    "DriverName": "postgres",
    "DataSource": "postgres://mmuser:password@db.example.com:5432/mattermost?sslmode=disable"
  }
}
```

#### Node 2: node2.example.com
```json
{
  "ServiceSettings": {
    "SiteURL": "http://mattermost.example.com",
    "ListenAddress": ":8065"
  },
  "ClusterSettings": {
    "Enable": true,
    "ClusterName": "production",
    "OverrideHostname": "node2.example.com",
    "UseIPAddress": false,
    "GossipPort": 8074,
    "EnableGossipCompression": true
  },
  "SqlSettings": {
    "DriverName": "postgres",
    "DataSource": "postgres://mmuser:password@db.example.com:5432/mattermost?sslmode=disable"
  }
}
```

**Requirements:**
- Both nodes must use the **same database**
- `ClusterName` must be **identical** on all nodes
- Firewall must allow TCP/UDP port 8074 between nodes
- Load balancer in front of nodes (HAProxy, nginx, etc.)

## API Reference

### Implemented Methods

All 18 methods of `einterfaces.ClusterInterface` are implemented:

#### Core Methods

- `StartInterNodeCommunication()` - Initializes and starts cluster
- `StopInterNodeCommunication()` - Gracefully stops cluster
- `RegisterClusterMessageHandler(event, handler)` - Registers message handler

#### Information Methods

- `GetClusterId() string` - Returns node ID
- `IsLeader() bool` - Returns true if this node is leader
- `HealthScore() int` - Returns health score (lower = better)
- `GetMyClusterInfo() *model.ClusterInfo` - Returns this node's info
- `GetClusterInfos() ([]*model.ClusterInfo, error)` - Returns all nodes' info

#### Messaging Methods

- `SendClusterMessage(msg)` - Broadcasts message to all nodes
- `SendClusterMessageToNode(nodeID, msg) error` - Sends to specific node

#### Statistics Methods

- `GetClusterStats(rctx) ([]*model.ClusterStats, *model.AppError)` - Returns cluster statistics

#### Partial Implementations (Stubs)

- `GetLogs(rctx, page, perPage)` - Returns cluster status (not full logs)
- `QueryLogs(rctx, page, perPage)` - Returns logs from this node
- `GenerateSupportPacket(rctx, options)` - Returns basic cluster info
- `GetPluginStatuses()` - Returns empty status
- `ConfigChanged(prev, new, sendToOther)` - Broadcasts config change event
- `WebConnCountForUser(userID)` - Returns 0 (requires app.Hub access)
- `GetWSQueues(userID, connID, seqNum)` - Returns empty (requires app.Hub access)

## Testing and Verification

### 1. Check Logs

```bash
tail -f /var/log/mattermost/mattermost.log | grep SznCluster
```

Expected output:
```
[INFO] SznCluster: Cluster interface registered
[INFO] SznCluster: Starting inter-node communication node_id=abc123...
[INFO] SznCluster: Memberlist initialized node_id=abc123 bind_addr=10.0.1.10 bind_port=8074
[INFO] SznCluster: Node joined node_name=def456 addr=10.0.1.11
```

### 2. Database Verification

```sql
SELECT * FROM ClusterDiscovery WHERE ClusterName = 'production';
```

### 3. Network Connectivity

```bash
# Test TCP
nc -zv node2.example.com 8074

# Test UDP
nc -zuv node2.example.com 8074
```

## Troubleshooting

### Nodes Not Joining

**Checks:**
1. Verify firewall allows port 8074 (TCP + UDP)
2. Verify same `ClusterName` in all configs
3. Check database connectivity from all nodes
4. Test network connectivity between nodes
5. Check logs for errors

### High CPU Usage

**Solutions:**
1. Disable compression if CPU-bound
2. Check message broadcast frequency
3. Monitor network latency
4. Check memberlist health scores

### Split Brain

**Recovery:**
1. Identify the partition from logs
2. Check database connectivity from all nodes
3. Verify bidirectional network connectivity
4. Restart minority partition nodes first

### Memory Leaks

**Checks:**
1. Monitor broadcast queue size
2. Check handler registrations aren't repeated
3. Monitor memberlist member count
4. Use pprof for memory profiling

## Limitations

### Current Limitations

1. **WebSocket Queries** - Require app.Hub access (not available at cluster level)
2. **Log Retrieval** - Returns cluster status only (needs log file access)
3. **Plugin Status** - Returns empty (needs plugin manager access)
4. **Support Packets** - Basic info only (needs subsystem access)
5. **Gossip Encryption** - Flag read but key config not implemented

### Architectural Limitations

1. **Eventual Consistency** - SWIM provides eventual consistency (acceptable for most use cases)
2. **No Request/Response** - Fire-and-forget messaging (can be extended)
3. **In-Memory Queue** - Broadcast queue lost on crash (can add persistence)
4. **Simple Leader Election** - Lexicographic ordering (sufficient for non-critical tasks)

### Scale Limitations

- **Recommended:** Up to 50-100 nodes
- **Maximum:** Several hundred nodes (based on Memberlist)
- **Detection time:** O(log n)

## Future Enhancements

1. **Request/Response Pattern** - Add correlation IDs and response handling
2. **Metadata Exchange** - Share versions and capabilities
3. **Advanced Leader Election** - Implement Raft consensus
4. **Metrics Export** - Prometheus metrics endpoint
5. **Encryption Key Rotation** - Automatic key rotation
6. **State Synchronization** - Full state sync implementation
7. **Persistent Queue** - Disk-backed broadcast queue

## License

This code is a derivative work based on Mattermost server (https://github.com/mattermost/mattermost-server), which is licensed under AGPL v3.0.

This derivative work is used exclusively for internal purposes within Seznam.cz, a.s. and is not distributed to third parties. Therefore, the copyleft provisions of AGPL v3.0 do not apply as per the license's distribution requirements.

**Copyright (c) 2024-present Seznam.cz, a.s. All Rights Reserved.**

## Authors

Developed by Seznam.cz, a.s. for internal use - 2024

---

**Note:** This is custom software developed for internal use at Seznam.cz. While the implementation is shared for educational purposes, it is not officially supported or recommended for production use by third parties without thorough testing and understanding of the codebase.
