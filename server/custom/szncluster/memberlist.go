// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.

package szncluster

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

// initializeMemberlist creates and configures the memberlist instance
func (c *SznCluster) initializeMemberlist() error {
	cfg := c.platform.Config()

	// Create memberlist config
	mlConfig := memberlist.DefaultLANConfig()

	// Set node name (use node ID)
	mlConfig.Name = c.nodeID

	// Configure gossip port
	mlConfig.BindPort = *cfg.ClusterSettings.GossipPort
	mlConfig.AdvertisePort = *cfg.ClusterSettings.GossipPort

	// Configure logger to use Mattermost logger
	// Memberlist expects *log.Logger, but Mattermost uses *mlog.Logger, so we wrap it
	// Log level filtering is handled by Mattermost configuration
	mlConfig.Logger = log.New(&memberlistLogger{}, "", 0)

	// Configure bind address
	if cfg.ClusterSettings.BindAddress != nil && *cfg.ClusterSettings.BindAddress != "" {
		mlConfig.BindAddr = *cfg.ClusterSettings.BindAddress
	} else if cfg.ClusterSettings.UseIPAddress != nil && *cfg.ClusterSettings.UseIPAddress {
		// Try to get IP address
		if ip, err := getLocalIP(); err == nil {
			mlConfig.BindAddr = ip.String()
		}
	}

	// Configure advertise address - CRITICAL for Docker/NAT environments
	// In Docker: bind is typically 172.x.x.x (container network), advertise should be 10.x.x.x (host network)
	// The advertise address is what other nodes will use to contact this node
	if cfg.ClusterSettings.AdvertiseAddress != nil && *cfg.ClusterSettings.AdvertiseAddress != "" {
		mlConfig.AdvertiseAddr = *cfg.ClusterSettings.AdvertiseAddress
		mlog.Info("SznCluster: Using explicit advertise address (recommended for Docker/NAT)",
			mlog.String("advertise_addr", mlConfig.AdvertiseAddr),
			mlog.String("bind_addr", mlConfig.BindAddr))
	} else if mlConfig.BindAddr != "" && mlConfig.BindAddr != "0.0.0.0" {
		// Use bind address as advertise address if bind is specific (non-Docker scenario)
		mlConfig.AdvertiseAddr = mlConfig.BindAddr
		mlog.Warn("SznCluster: Using bind address as advertise address - this may not work in Docker/NAT",
			mlog.String("advertise_addr", mlConfig.AdvertiseAddr),
			mlog.String("bind_addr", mlConfig.BindAddr))
	} else {
		// Try to detect routable IP address (fallback)
		if ip, err := getLocalIP(); err == nil {
			mlConfig.AdvertiseAddr = ip.String()
			mlog.Warn("SznCluster: Auto-detected advertise address - STRONGLY recommend setting AdvertiseAddress explicitly in Docker",
				mlog.String("advertise_addr", mlConfig.AdvertiseAddr),
				mlog.String("bind_addr", mlConfig.BindAddr))
		} else {
			mlog.Error("SznCluster: Failed to determine advertise address - cluster communication WILL FAIL in Docker/NAT",
				mlog.Err(err),
				mlog.String("bind_addr", mlConfig.BindAddr))
		}
	}

	// Configure delegates
	c.delegate = &clusterDelegate{cluster: c}
	mlConfig.Delegate = c.delegate
	mlConfig.Events = &clusterEvents{cluster: c}

	// Configure encryption if enabled
	if cfg.ClusterSettings.EnableGossipEncryption != nil && *cfg.ClusterSettings.EnableGossipEncryption {
		// Try to get encryption key from environment variable
		encryptionKey := os.Getenv("MM_CLUSTER_ENCRYPTION_KEY")
		if encryptionKey != "" {
			// Key must be 16, 24, or 32 bytes for AES
			if len(encryptionKey) == 16 || len(encryptionKey) == 24 || len(encryptionKey) == 32 {
				mlConfig.SecretKey = []byte(encryptionKey)
				mlog.Info("SznCluster: Gossip encryption enabled with key from MM_CLUSTER_ENCRYPTION_KEY")
			} else {
				mlog.Warn("SznCluster: Invalid encryption key length, must be 16, 24, or 32 bytes. Encryption disabled.")
			}
		} else {
			mlog.Info("SznCluster: Encryption enabled in config but MM_CLUSTER_ENCRYPTION_KEY not set. Running without encryption.")
		}
	}

	// Configure compression
	mlConfig.EnableCompression = cfg.ClusterSettings.EnableGossipCompression != nil &&
		*cfg.ClusterSettings.EnableGossipCompression

	// Docker/NAT compatibility: Don't disable TCP pings as fallback for UDP issues
	// This allows cluster to work even when UDP has problems
	mlConfig.DisableTcpPings = false

	// Adjust timeouts for Docker/container environments
	// Shorter timeouts for faster failure detection and recovery
	mlConfig.TCPTimeout = 10 * time.Second
	mlConfig.ProbeTimeout = 2 * time.Second  // Reduced from 3s for faster failure detection
	mlConfig.ProbeInterval = 3 * time.Second // Reduced from 5s for faster recovery

	// Suspicion multiplier - how many failed probes before declaring node dead
	// Default is 4, we keep it for balance between false positives and recovery speed
	mlConfig.SuspicionMult = 4

	// Increase gossip frequency for faster message propagation and recovery
	// Default is 200ms, we use 400ms as compromise between speed and bandwidth
	mlConfig.GossipInterval = 400 * time.Millisecond
	mlConfig.GossipNodes = 3 // Number of random nodes to gossip to per interval

	// Increase retransmit multiplier for better reliability in Docker
	// This increases the number of times a message is retransmitted
	mlConfig.RetransmitMult = 4 // Default is 4, keep it

	// Push/Pull interval for full state sync (useful for recovery)
	// Default is 30s, we reduce to 20s for faster state convergence
	mlConfig.PushPullInterval = 20 * time.Second

	mlog.Debug("SznCluster: Memberlist configured for container environment",
		mlog.Bool("tcp_pings_enabled", !mlConfig.DisableTcpPings),
		mlog.String("tcp_timeout", mlConfig.TCPTimeout.String()),
		mlog.String("probe_timeout", mlConfig.ProbeTimeout.String()),
		mlog.String("probe_interval", mlConfig.ProbeInterval.String()),
		mlog.String("gossip_interval", mlConfig.GossipInterval.String()),
		mlog.Int("gossip_nodes", mlConfig.GossipNodes),
		mlog.String("push_pull_interval", mlConfig.PushPullInterval.String()))

	// Create memberlist
	ml, err := memberlist.Create(mlConfig)
	if err != nil {
		return fmt.Errorf("failed to create memberlist: %w", err)
	}

	c.memberlist = ml

	mlog.Info("SznCluster: Memberlist initialized",
		mlog.String("node_id", c.nodeID),
		mlog.String("bind_addr", mlConfig.BindAddr),
		mlog.Int("bind_port", mlConfig.BindPort),
		mlog.String("advertise_addr", mlConfig.AdvertiseAddr),
		mlog.Int("advertise_port", mlConfig.AdvertisePort))

	return nil
}

// joinCluster attempts to join existing cluster nodes
func (c *SznCluster) joinCluster() error {
	// Try to discover existing nodes from database
	nodes, err := c.discoverNodes()
	if err != nil {
		mlog.Warn("SznCluster: Failed to discover nodes from database", mlog.Err(err))
	}

	if len(nodes) == 0 {
		mlog.Info("SznCluster: No existing nodes found, starting as first node")
		return nil
	}

	// Try to join discovered nodes
	mlog.Info("SznCluster: Attempting to join cluster", mlog.Int("node_count", len(nodes)))

	joined, err := c.memberlist.Join(nodes)
	if err != nil {
		return fmt.Errorf("failed to join cluster: %w", err)
	}

	mlog.Info("SznCluster: Successfully joined cluster", mlog.Int("joined_count", joined))

	return nil
}

// discoverNodes discovers other cluster nodes from the database
func (c *SznCluster) discoverNodes() ([]string, error) {
	cfg := c.platform.Config()
	clusterName := ""
	if cfg.ClusterSettings.ClusterName != nil {
		clusterName = *cfg.ClusterSettings.ClusterName
	}

	// Get all cluster discovery entries from database (only active nodes from last 30 minutes)
	discoveries, err := c.platform.Store.ClusterDiscovery().GetAll(model.CDSTypeApp, clusterName)
	if err != nil {
		return nil, err
	}

	mlog.Debug("SznCluster: Found cluster discovery entries",
		mlog.Int("count", len(discoveries)),
		mlog.String("cluster_name", clusterName))

	var nodes []string
	myAdvertiseAddress := c.getAdvertiseAddress()
	for _, discovery := range discoveries {
		// Skip entries without hostname (advertise address)
		if discovery.Hostname == "" {
			continue
		}

		// Skip ourselves (compare advertise addresses)
		if discovery.Hostname == myAdvertiseAddress {
			mlog.Debug("SznCluster: Skipping own discovery entry", mlog.String("advertise_address", myAdvertiseAddress))
			continue
		}

		// Build address using GossipPort from discovery (supports different ports per node)
		gossipPort := discovery.GossipPort
		if gossipPort == 0 {
			// Fallback to config if not set in discovery
			gossipPort = int32(*cfg.ClusterSettings.GossipPort)
		}
		addr := fmt.Sprintf("%s:%d", discovery.Hostname, gossipPort)
		nodes = append(nodes, addr)
		mlog.Debug("SznCluster: Adding node to join list",
			mlog.String("hostname", discovery.Hostname),
			mlog.Int("gossip_port", int(gossipPort)),
			mlog.String("address", addr))
	}

	return nodes, nil
}

// getLocalIP returns the non-loopback local IP of the host
func getLocalIP() (net.IP, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP, nil
			}
		}
	}

	return nil, fmt.Errorf("no local IP address found")
}

// getHostname returns the hostname for this node for informational/debug purposes
// This is NOT used for communication, only for display in cluster info
func (c *SznCluster) getHostname() string {
	cfg := c.platform.Config()

	// Priority 1: OverrideHostname (for custom display name)
	if cfg.ClusterSettings.OverrideHostname != nil && *cfg.ClusterSettings.OverrideHostname != "" {
		return *cfg.ClusterSettings.OverrideHostname
	}

	// Priority 2: IP address if explicitly requested via UseIPAddress
	if cfg.ClusterSettings.UseIPAddress != nil && *cfg.ClusterSettings.UseIPAddress {
		if ip, err := getLocalIP(); err == nil {
			return ip.String()
		}
	}

	// Priority 3: OS hostname (default)
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		return hostname
	}

	return "unknown"
}

// getAdvertiseAddress returns the advertise address for this node
// This is the address that other nodes will use to connect to this node
// It's used for:
//  1. Generating persistent node ID (must be stable across restarts)
//  2. Storing in ClusterDiscovery table (so other nodes know where to connect)
//  3. Identifying ourselves when reading from ClusterDiscovery
//
// IMPORTANT: For persistent node ID, this must return a stable value across pod restarts
func (c *SznCluster) getAdvertiseAddress() string {
	cfg := c.platform.Config()

	// Priority 1: Explicit AdvertiseAddress (recommended for Docker/Kubernetes)
	if cfg.ClusterSettings.AdvertiseAddress != nil && *cfg.ClusterSettings.AdvertiseAddress != "" {
		return *cfg.ClusterSettings.AdvertiseAddress
	}

	// Priority 2: OverrideHostname (for custom setups)
	if cfg.ClusterSettings.OverrideHostname != nil && *cfg.ClusterSettings.OverrideHostname != "" {
		return *cfg.ClusterSettings.OverrideHostname
	}

	// Priority 3: IP address if explicitly requested via UseIPAddress
	// WARNING: In Kubernetes, pod IP changes on restart, breaking persistent ID!
	if cfg.ClusterSettings.UseIPAddress != nil && *cfg.ClusterSettings.UseIPAddress {
		if ip, err := getLocalIP(); err == nil {
			return ip.String()
		}
	}

	// Priority 4: Fall back to OS hostname (stable in Kubernetes StatefulSets)
	// This is the best default for persistent node ID across pod restarts
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		return hostname
	}

	return "unknown"
}

// memberlistLogger is an io.Writer adapter that bridges memberlist's *log.Logger
// to Mattermost's *mlog.Logger. Memberlist expects standard Go logger, but Mattermost
// uses its own structured logger, so we need this wrapper to convert between them.
// Mattermost's log level configuration will handle filtering (DEBUG, INFO, WARN, ERROR).
type memberlistLogger struct{}

func (l *memberlistLogger) Write(p []byte) (n int, err error) {
	msg := string(p)

	// Parse memberlist log level and forward to appropriate mlog method
	// Mattermost configuration will handle log level filtering
	if len(msg) > 7 {
		switch {
		case msg[0:7] == "[DEBUG]":
			mlog.Debug("SznCluster: " + msg[8:])
		case msg[0:6] == "[INFO]":
			mlog.Info("SznCluster: " + msg[7:])
		case msg[0:6] == "[WARN]":
			mlog.Warn("SznCluster: " + msg[7:])
		case msg[0:7] == "[ERROR]":
			mlog.Error("SznCluster: " + msg[8:])
		default:
			// Fallback for messages without level prefix
			mlog.Info("SznCluster: " + msg)
		}
	}

	return len(p), nil
}
