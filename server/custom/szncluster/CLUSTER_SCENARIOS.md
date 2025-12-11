# SznCluster - Autodiscovery & Recovery Scenarios

This document describes how autodiscovery and recovery work in a 2-node cluster setup under various failure scenarios.

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
   ├─ Health check timer (T+3s initial, then every 20s)
   └─ Deduplication cleanup (every 10s)

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

### **T=3s, T=8s: Initial Health Checks**

```
Node A (T=3s):
checkClusterHealth()
├─ DB query → [node-a-uuid (self)]
├─ Memberlist → [A]
└─ No reconnects needed

Node B (T=8s):
checkClusterHealth()
├─ DB query → [node-a-uuid, node-b-uuid]
├─ Memberlist → [A, B]
└─ All nodes in memberlist ✅
```

---

### **T=15s, T=20s, T=30s: Periodic Updates**

```
Node A every 15s:
updateClusterDiscovery() → UPDATE ClusterDiscovery SET LastPingAt=T15 WHERE Id='node-a-uuid'

Node B every 15s:
updateClusterDiscovery() → UPDATE ClusterDiscovery SET LastPingAt=T20 WHERE Id='node-b-uuid'

Node A every 20s (T=23s, T=43s, ...):
checkClusterHealth()
├─ DB → [A, B] both with fresh LastPingAt
├─ Memberlist → [A, B]
└─ ✅ All good

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
Node A continues:
├─ T=63s: checkClusterHealth()
│  ├─ DB query → [node-a-uuid] (B no longer in DB!)
│  ├─ Memberlist → [A]
│  └─ ✅ No reconnects needed (B not in DB = inactive)
│
├─ T=65s: updateClusterDiscovery()
│  └─ UPDATE LastPingAt=T65 for node-a-uuid
│
├─ If Node A broadcasts message:
│  └─ sendToAllNodes()
│     └─ members = [A]
│     └─ Sends only to self (skip) → NO MESSAGES FOR B ✅
│        (B not in memberlist NOR in DB)
```

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

### **T=210-260s: Node A attempts recovery**

```
T=220s: checkClusterHealth()
├─ DB query → WHERE LastPingAt > (now - 30min)
│  └─ [node-a-uuid (T220), node-b-uuid (T195)]
│     └─ node-b-uuid is 25s old ✅ younger than 30min
├─ Memberlist → [A]
├─ node-b-uuid not in memberlist!
├─ SecondsSinceLastPing = (T220 - T195) = 25s < 90s
│  └─ "Node in DB not reachable, will attempt reconnect"
├─ nodesToReconnect = ['10.1.2.4:8074']
└─ memberlist.Join(['10.1.2.4:8074'])
   └─ ❌ FAILED (B still down)

T=240s: checkClusterHealth()
├─ SecondsSinceLastPing = 45s < 90s
└─ memberlist.Join(['10.1.2.4:8074']) → ❌ FAILED

T=260s: checkClusterHealth()
├─ SecondsSinceLastPing = 65s < 90s
└─ memberlist.Join(['10.1.2.4:8074']) → ❌ FAILED
   (Node B still down)
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
T=280s: checkClusterHealth() on Node A or B
├─ DB query → [node-a-uuid, node-b-uuid (T195), node-b-uuid-NEW (T280)]
├─ node-b-uuid: SecondsSinceLastPing = 85s < 90s
│  └─ Still attempts reconnect (unnecessarily)
│
T=290s+: SecondsSinceLastPing > 90s
├─ "Node hasn't pinged recently, skipping reconnect"
└─ Stops attempting reconnect

After 30 minutes (T=1800s+):
├─ Cleanup() job: DELETE WHERE LastPingAt < (now - 30min)
└─ node-b-uuid (T195) deleted from DB ✅
```

---

## 📊 **Recovery Times Summary**

| Scenario | Shutdown Type | DB Cleanup | Recovery Time | Notes |
|----------|--------------|------------|---------------|-------|
| **10s restart** | Graceful | ✅ Immediate | ~10s | Leave message + DB cleanup → fast recovery |
| **30s restart** | Graceful | ✅ Immediate | ~30s | Same as 10s, just longer downtime |
| **60s crash** | Ungraceful | ❌ Delayed (30min) | ~60s | Stale DB entry, retry attempts every 20s |

---

## 🔑 **Key Autodiscovery Components**

### **Discovery Loop (every 15s)**
```go
updateClusterDiscovery()
└─ UPDATE ClusterDiscovery SET LastPingAt = NOW()
   WHERE Id = <this-node-uuid>
```

### **Health Check (every 20s)**
```go
checkClusterHealth()
├─ dbNodes = SELECT * FROM ClusterDiscovery 
│            WHERE LastPingAt > (now - 30min)
├─ memberlistNodes = memberlist.Members()
│
└─ FOR each dbNode:
   ├─ IF dbNode NOT IN memberlistNodes:
   │  └─ IF (now - dbNode.LastPingAt) < 90s:
   │     └─ Try memberlist.Join(dbNode.address)
   │        ↳ If success → node returns to memberlist
   │        ↳ If fail → retry in 20s
   │
   └─ IF (now - dbNode.LastPingAt) >= 90s:
      └─ Skip (considered dead)
```

### **Cleanup (periodic job)**
```go
Cleanup()
└─ DELETE FROM ClusterDiscovery 
   WHERE LastPingAt < (now - 30 minutes)
```

---

## ⚡ **Timeouts & Intervals**

```
Initial health check:     3s    ← First reconnect attempt after startup
Periodic health check:   20s    ← Regular reconnect attempts
Discovery update:        15s    ← LastPingAt refresh
Memberlist probe:      ~10s    ← Dead node detection (UDP+TCP)
Retry window:           90s    ← Attempts to connect nodes younger than 90s
DB cleanup:          30 min    ← Removal of completely stale entries
Deduplication:         5s     ← Window for duplicate detection
```

---

## 💡 **Key Design Principles**

1. **Database is Source of Truth** - DB discovery table determines which nodes should be active
2. **Memberlist is Reality** - Memberlist shows which nodes are actually communicating
3. **Health Check is Bridge** - Reconciles DB expectations with memberlist reality
4. **Graceful Shutdown Cleans DB** - Prevents stale entries for clean restarts
5. **Crash Recovery via Retry** - Ungraceful shutdowns handled by periodic reconnect attempts
6. **90s Retry Window** - Balances between recovery and giving up on dead nodes
7. **30min Cleanup** - Final cleanup for truly dead nodes that never returned
