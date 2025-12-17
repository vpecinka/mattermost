# SznCluster - Autodiscovery & Recovery Scenarios

This document describes how autodiscovery and recovery work in a 2-3 node cluster setup under various failure scenarios.

**Important**: This cluster uses **memberlist's built-in health checking** (probes every 3s, anti-entropy push/pull every 20s). There is NO custom health check mechanism - the database serves purely as a **seed list** for bootstrap.

## 🎯 **Initial Setup - 2 Nodes Start**

### **T=0: Node A starts (first in cluster)**

```
Node A startup:
├─ StartInterNodeCommunication()
│  ├─ Cleanup() DB (removes entries > 30 min)
│  ├─ initializeMemberlist()
│  │  └─ Bind 0.0.0.0:8074, Advertise 10.1.2.3:8074
│  ├─ joinCluster()
│  │  ├─ discoverNodes() → SELECT FROM ClusterDiscovery WHERE LastPingAt > (now - 30min)
│  │  │  └─ Result: [] (empty, we're first)
│  │  └─ memberlist.Join([]) → no nodes to join
│  └─ startClusterDiscovery()
│     └─ updateClusterDiscovery()
│        └─ INSERT INTO ClusterDiscovery:
│           Id='node-a-uuid'
│           Hostname='10.1.2.3'
│           GossipPort=8074
│           LastPingAt=T0
│
└─ Started goroutines:
   ├─ Discovery ticker (every 15s) → UPDATE LastPingAt
   ├─► Deduplication cleanup (every 10s)
   ├─► Queue pruning (every 30s)
   └─► Metrics logging (every 60s)

DB State:
┌──────────────┬──────────┬───────────┬────────────┐
│ Id           │ Hostname │ GossipPort│ LastPingAt │
├──────────────┼──────────┼───────────┼────────────┤
│ node-a-uuid  │ 10.1.2.3 │ 8074      │ T0         │
└──────────────┴──────────┴───────────┴────────────┘

Memberlist: [A]
```

---

### **T=5s: Node B starts**

```
Node B startup:
├─ StartInterNodeCommunication()
│  ├─ Cleanup() DB
│  ├─ initializeMemberlist()
│  │  └─ Bind 0.0.0.0:8074, Advertise 10.1.2.4:8074
│  ├─ joinCluster()
│  │  ├─ discoverNodes() → SELECT FROM ClusterDiscovery...
│  │  │  └─ Result: [{'Id':'node-a-uuid', 'Hostname':'10.1.2.3', 'GossipPort':8074}]
│  │  └─ memberlist.Join(['10.1.2.3:8074'])
│  │     └─ ✅ UDP handshake with Node A → successful
│  └─ updateClusterDiscovery()
│     └─ INSERT INTO ClusterDiscovery:
│        Id='node-b-uuid'
│        Hostname='10.1.2.4'
│        LastPingAt=T5
│
└─ Node A receives NotifyJoin event:
   └─ "SznCluster: Node joined, node_name=node-b-uuid, addr=10.1.2.4"

DB State:
┌──────────────┬──────────┬───────────┬────────────┐
│ Id           │ Hostname │ GossipPort│ LastPingAt │
├──────────────┼──────────┼───────────┼────────────┤
│ node-a-uuid  │ 10.1.2.3 │ 8074      │ T0         │
│ node-b-uuid  │ 10.1.2.4 │ 8074      │ T5         │
└──────────────┴──────────┴───────────┴────────────┘

Memberlist A: [A, B]
Memberlist B: [A, B]
```

---

### **T=3s - T=30s: Memberlist Health Checks (Automatic)**

```
Memberlist (every 3s):
├─ Probe random member (UDP PING)
├─ Indirect probes if direct fails (ask others to probe)
└─ Suspicion → Dead after 4 failed probes (~12-15s)

Memberlist (every 20s):
Push/Pull anti-entropy
├─ Full member list exchange
├─ State synchronization
└─ Self-healing cluster

Node A & B every 15s:
updateClusterDiscovery() → UPDATE ClusterDiscovery SET LastPingAt=NOW()

DB State (T=30s):
┌──────────────┬──────────┬───────────┬────────────┐
│ Id           │ Hostname │ GossipPort│ LastPingAt │
├──────────────┼──────────┼───────────┼────────────┤
│ node-a-uuid  │ 10.1.2.3 │ 8074      │ T30        │
│ node-b-uuid  │ 10.1.2.4 │ 8074      │ T30        │
└──────────────┴──────────┴───────────┴────────────┘
```

---

## ⚠️ **Scenario 1: Node B restarts for 10 seconds**

### **T=60s: Node B begins restart**

```
T=60s: Node B shutdown initiated
├─ StopInterNodeCommunication()
│  ├─ close(shutdownCh) → stops goroutines
│  ├─ memberlist.Leave(1s) → sends LEAVE message to Node A
│  │  └─ Node A receives NotifyLeave:
│  │     └─ "SznCluster: Node left, node_name=node-b-uuid"
│  ├─ memberlist.Shutdown()
│  └─ cleanupClusterDiscovery()
│     └─ DELETE FROM ClusterDiscovery WHERE Id='node-b-uuid'
│
└─ Node B stopped

DB State:
┌──────────────┬──────────┬───────────┬────────────┐
│ Id           │ Hostname │ GossipPort│ LastPingAt │
├──────────────┼──────────┼───────────┼────────────┤
│ node-a-uuid  │ 10.1.2.3 │ 8074      │ T60        │
└──────────────┴──────────┴───────────┴────────────┘
                           ☠️ Node B gone from DB!

Memberlist A: [A]  ← B removed due to Leave message
```

### **T=61-70s: Node B is down**

```
Node A continues normally:
├─► Memberlist: [A] (B removed due to Leave message)
├─► T=65s: updateClusterDiscovery() → UPDATE LastPingAt=T65
│
├─► If Node A broadcasts message:
│  └─► sendToAllNodes()
│     └─► members = memberlist.Members() = [A]
│     └─► Sends only to self (skip) → NO MESSAGES FOR B ✅
│
└─► No health check attempts - B not in memberlist
    (Memberlist is source of truth, not DB)

### **T=70s: Node B comes back online**

```
Node B startup (second start):
├─ StartInterNodeCommunication()
│  ├─ NEW nodeID! 'node-b-uuid-2' (each restart = new UUID)
│  ├─ joinCluster()
│  │  ├─ discoverNodes() → [node-a-uuid]
│  │  └─ memberlist.Join(['10.1.2.3:8074'])
│  │     └─ ✅ Connection to Node A
│  └─ updateClusterDiscovery()
│     └─ INSERT: Id='node-b-uuid-2', LastPingAt=T70
│
└─ Node A receives NotifyJoin:
   └─ "Node joined, node_name=node-b-uuid-2"

DB State:
┌─────────────────┬──────────┬───────────┬────────────┐
│ Id              │ Hostname │ GossipPort│ LastPingAt │
├─────────────────┼──────────┼───────────┼────────────┤
│ node-a-uuid     │ 10.1.2.3 │ 8074      │ T70        │
│ node-b-uuid-2   │ 10.1.2.4 │ 8074      │ T70        │ ← NEW ID!
└─────────────────┴──────────┴───────────┴────────────┘

Memberlist A: [A, B(new)]
Memberlist B: [A, B(new)]

✅ Cluster recovered! Message loss: ~10 seconds
```

---

## ⚠️ **Scenario 2: Node B restarts for 30 seconds**

### **T=100s: Node B shutdown (graceful)**

```
T=100s: Same as previous scenario
├─ Leave message → Node A knows B is leaving
├─ DELETE from DB
└─ Memberlist A: [A]

DB: only [node-a-uuid]
```

### **T=101-130s: Node B is down**

```
Node A every 20s (T=103s, T=123s):
checkClusterHealth()
├─ DB → [node-a-uuid]
├─ Memberlist → [A]
└─ No action (B not in DB = expected offline)

Node A continues normally, sends messages only to self (skip).
```

### **T=130s: Node B returns**

```
Node B startup:
├─ joinCluster() → discoverNodes() → [node-a-uuid]
├─ Join Node A → ✅ success
└─ INSERT into DB

✅ Recovery same as 10s scenario
Outage: ~30 seconds
```

---

## ⚠️ **Scenario 3: Node B crashes (ungraceful) for 60 seconds**

### **T=200s: Node B crashes WITHOUT graceful shutdown**

```
T=200s: Node B CRASH (kill -9, power off, kernel panic)
├─ ❌ No Leave message!
├─ ❌ No cleanupClusterDiscovery()!
└─ DB remains: [node-a-uuid, node-b-uuid] ← B STILL IN DB!

Memberlist B: destroyed
```

### **T=200-210s: Memberlist timeout detection**

```
Node A memberlist probe mechanism:
├─ T=200-210s: Probe timeout (~10s total)
│  ├─ UDP probes to 10.1.2.4:8074 → timeout
│  ├─ TCP probes to 10.1.2.4:8074 → timeout
│  └─ Suspicion → Dead state
│
└─ T=210s: NotifyLeave triggered
   └─ "Node left/failed, node_name=node-b-uuid"
   ⚠️ BUT cleanupClusterDiscovery() is NOT called!
       (only called on graceful shutdown)

DB State (T=210s):
┌──────────────┬──────────┬───────────┬────────────┐
│ Id           │ Hostname │ GossipPort│ LastPingAt │
├──────────────┼──────────┼───────────┼────────────┤
│ node-a-uuid  │ 10.1.2.3 │ 8074      │ T210       │
│ node-b-uuid  │ 10.1.2.4 │ 8074      │ T195 ⚠️    │ ← STALE!
└──────────────┴──────────┴───────────┴────────────┘
                           (15 seconds old ping)

Memberlist A: [A]  ← B removed (detected dead)
```

### **T=210-260s: Node A waiting (no active recovery attempts)**

```
Node A state:
├─ Memberlist: [A] (B marked as dead)
├─ DB: [node-a-uuid (fresh), node-b-uuid (T195, stale)]
├─ updateClusterDiscovery() every 15s → keeps A alive in DB
└─ NO active reconnection attempts - memberlist handles health

Why no reconnect attempts?
- Memberlist already detected B as dead (~12s after crash)
- DB is seed list only, not source of truth
- Anti-entropy push/pull will sync when B returns
- Stale DB entry cleaned up after 30 minutes
```

### **T=260s: Node B comes back online**

```
Node B startup:
├─ NEW nodeID: 'node-b-uuid-NEW'
├─ joinCluster()
│  ├─ discoverNodes() → [node-a-uuid, node-b-uuid (STALE!)]
│  │  └─ ⚠️ Sees its own old entry!
│  └─ memberlist.Join(['10.1.2.3:8074', '10.1.2.4:8074'])
│     ├─ 10.1.2.3 → ✅ Join Node A successful
│     └─ 10.1.2.4 → ❌ Failed (that's itself, not ready yet)
│
├─ updateClusterDiscovery()
│  ├─ Exists(Id='node-b-uuid-NEW') → false
│  └─ INSERT: Id='node-b-uuid-NEW', LastPingAt=T260
│
└─ Node A NotifyJoin:
   └─ "Node joined, node_name=node-b-uuid-NEW"

DB State (T=260s):
┌──────────────────┬──────────┬───────────┬────────────┐
│ Id               │ Hostname │ GossipPort│ LastPingAt │
├──────────────────┼──────────┼───────────┼────────────┤
│ node-a-uuid      │ 10.1.2.3 │ 8074      │ T260       │
│ node-b-uuid      │ 10.1.2.4 │ 8074      │ T195 ⚠️    │ ← STALE (65s old)
│ node-b-uuid-NEW  │ 10.1.2.4 │ 8074      │ T260       │ ← ACTIVE
└──────────────────┴──────────┴───────────┴────────────┘

Memberlist A: [A, B(new)]
Memberlist B: [A, B(new)]

✅ Cluster recovered!
```

### **T=280s+: Cleanup of old entry**

```
DB State persists with stale entry:
├─ node-a-uuid: LastPingAt=T280 (active)
├─ node-b-uuid: LastPingAt=T195 (stale, 85s old)
└─ node-b-uuid-NEW: LastPingAt=T280 (active)

After 30 minutes (T=1800s+):
├─ Cleanup() job: DELETE WHERE LastPingAt < (now - 30min)
└─ node-b-uuid (T195) deleted from DB ✅

Note: Stale entry harmless - memberlist is source of truth
```

---

## 📊 **Recovery Times Summary**

| Scenario | Shutdown Type | DB Cleanup | Recovery Time | Notes |
|----------|--------------|------------|---------------|-------|
| **10s restart** | Graceful | ✅ Immediate | ~10s | Leave message + DB cleanup → fast recovery |
| **30s restart** | Graceful | ✅ Immediate | ~30s | Same as 10s, just longer downtime |
| **60s crash** | Ungraceful | ❌ Delayed (30min) | ~60s | Memberlist auto-detects death in 12-15s, node rejoins on restart |

---

## 🔑 **Key Autodiscovery Components**

### **Discovery Loop (every 15s)**
```go
updateClusterDiscovery()
└─ UPDATE ClusterDiscovery SET LastPingAt = NOW()
   WHERE Id = <this-node-uuid>
```

### **Memberlist Health Check (automatic, built-in)**
```go
// Runs every 3 seconds
memberlist.Probe()
├─ Select random member
├─ Send UDP PING
├─ Wait for ACK (timeout 2s)
└─ If no ACK → indirect probes via other members
   └─ After 4 failed probes (~12-15s) → mark as Dead

// Runs every 20 seconds
memberlist.PushPull()
├─ Full member list exchange
├─ State synchronization
└─ Self-healing (discovers rejoined nodes)
```

### **Cleanup (periodic job, every 30 minutes)**
```go
Cleanup()
└─ DELETE FROM ClusterDiscovery 
   WHERE LastPingAt < (now - 30 minutes)
```

---

## ⚡ **Timeouts & Intervals**

```
Discovery update:        15s    ← LastPingAt refresh in DB
Memberlist probe:         3s    ← Health check frequency
Probe timeout:            2s    ← Waiting for ACK
Death detection:      12-15s    ← 4 failed probes (SuspicionMult=4)
Push/Pull sync:          20s    ← Full state synchronization (anti-entropy)
DB cleanup:          30 min    ← Removal of completely stale entries
Deduplication:           30s    ← Window for duplicate detection
Queue pruning:           30s    ← Limit queue to 5000 messages
Metrics logging:         60s    ← Queue and cluster metrics
```

---

## 💡 **Key Design Principles**

1. **Memberlist is Source of Truth** - Live cluster state maintained by memberlist in memory
2. **Database is Seed List** - DB provides bootstrap rendezvous point for new nodes
3. **No Custom Health Checking** - Memberlist handles all probing and failure detection
4. **Self-Healing via Anti-Entropy** - Push/Pull synchronization recovers from partitions
5. **Graceful Shutdown Cleans DB** - Prevents stale entries for clean restarts
6. **Crash Recovery Automatic** - Memberlist detects death, node rejoins via DB seed list
7. **30min Cleanup** - Final cleanup for completely dead nodes (won't rejoin)
