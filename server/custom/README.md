# Seznam.cz Custom Mattermost Modifications

This directory contains custom modifications to Mattermost Server developed by Seznam.cz, a.s. for internal use. All modifications are designed to be **AGPL v3.0 compliant** and maintain compatibility with upstream Mattermost through git rebase strategy.

## 📁 Custom Implementations

### 1. Scheduled Posts
**Status:** ✅ Fully implemented and tested  
**License Required:** ~~Enterprise~~ → None (SZN)  
**Changes:** 1 file - [license.go](./license.go) enables `data_retention` feature  
**What it does:** Allows users to schedule messages to be sent at a later time

Scheduled posts are fully implemented in open source. We provide a fake license with `data_retention` feature enabled.

### 2. Custom User Groups
**Status:** ✅ Fully implemented and tested  
**License Required:** ~~Professional~~ → None (SZN)  
**Changes:** 1 file - [license.go](./license.go) enables `ldap_groups` feature  
**What it does:** Create manual user groups for @mentions, organization, and access control

Custom user groups are 100% implemented in open source. We provide a fake Professional license with `ldap_groups` feature enabled.

### 3. [szncluster/](./szncluster/) - Custom Cluster Implementation
**Status:** ✅ Production-ready  
**License Required:** ~~Enterprise~~ → None (SZN)  
**Changes:** Custom implementation + [license.go](./license.go) enables `cluster` feature  
**What it does:** High-availability clustering without Enterprise license

Full replacement for Mattermost's enterprise clustering using SWIM gossip protocol.

### 4. [sznmetrics/](./sznmetrics/) - Custom Metrics Implementation
**Status:** ✅ Production-ready  
**License Required:** ~~Enterprise~~ → None (SZN)  
**Changes:** Custom implementation + [license.go](./license.go) enables `metrics` feature  
**What it does:** Comprehensive metrics collection and monitoring

Custom Prometheus metrics exporter with fake license support.

### 5. [sznsearch/](./sznsearch/) - Custom Search Implementation
**Status:** 🚧 In development  
**License Required:** ~~Enterprise~~ → None (SZN)  
**Changes:** Custom implementation + [license.go](./license.go) enables `elastic_search` feature  
**What it does:** Enhanced search capabilities without Elasticsearch

Alternative to Elasticsearch-based search for self-hosted deployments.

## 🔧 Maintenance Strategy

All custom modifications follow a **git rebase strategy** to maintain compatibility with upstream Mattermost.

### Core License Strategy

**Central modification:** [server/custom/license.go](./license.go) + [server/channels/app/platform/license.go](../channels/app/platform/license.go)

When no official Mattermost license is installed, we automatically provide a mock Professional license with features:
- `ldap_groups` - Enables custom user groups
- `elastic_search` - Enables custom search
- `metrics` - Enables custom metrics
- `cluster` - Enables custom clustering
- `data_retention` - Enables scheduled posts

To add new features, just edit [license.go](./license.go) - no need to patch individual files.

### Rebase Process

1. **Fetch upstream changes:**
   ```bash
   git fetch upstream
   ```

2. **Rebase custom branch:**
   ```bash
   git rebase upstream/master
   ```

3. **Resolve conflicts** (usually minimal):
   - Check if `platform/license.go` License() method still exists
   - Verify custom implementations in server/custom/ still work
   - Test functionality after rebase

4. **Verify changes:**
   ```bash
   # Check modified files
   git diff upstream/master -- server/custom/
   git diff upstream/master -- server/channels/app/platform/license.go
   ```

### Update Checklist

When upgrading to a new Mattermost version:

- [ ] Read [CHANGELOG.md](../../CHANGELOG.md) for breaking changes
- [ ] Check if modified functions were refactored
- [ ] Test scheduled posts feature
- [ ] Test custom user groups feature
- [ ] Verify cluster functionality (szncluster)
- [ ] Check metrics collection (sznmetrics)
- [ ] Run integration tests
- [ ] Update custom modification docs if needed

## 📝 Documentation Structure

Each custom modification has its own detailed documentation file:

```
server/custom/
├── README.md                    # This file - overview of all modifications
├── SCHEDULED_POSTS.md           # Detailed docs for scheduled posts
├── CUSTOM_USER_GROUPS.md        # Detailed docs for custom user groups
├── szncluster/
│   └── README.md                # Comprehensive cluster implementation docs
├── sznmetrics/
│   └── README.md                # Metrics implementation documentation
└── sznsearch/
    └── README.md                # Search implementation documentation
```

## 🎯 Philosophy

Our modifications follow these principles:

1. **AGPL v3.0 Compliance** - Only modify open source code, never use enterprise-only code
2. **Minimal Changes** - Comment out license checks rather than major refactoring
3. **Upstream Compatibility** - Design for easy rebasing on upstream changes
4. **Documentation** - Every change is documented with rationale
5. **Testing** - All modifications are thoroughly tested
6. **Open Source** - Share knowledge and contribute back when possible

## 🔍 License Compliance

### What is Legal

✅ **Modifying open source code** - AGPL v3.0 allows modifications  
✅ **Removing artificial restrictions** - License checks in OSS code  
✅ **Custom implementations** - Building features from scratch  
✅ **Commenting out code** - Standard modification technique  

### What is NOT Legal

❌ **Using enterprise directory code** - Code in `server/enterprise/` is proprietary  
❌ **Copying closed-source code** - Cannot use Mattermost Enterprise Edition code  
❌ **Removing license from enterprise builds** - Cannot bypass E0 license  
❌ **Redistributing enterprise features** - Cannot package enterprise code  

### Our Approach

We only modify code in the open source part of the repository:
- `server/channels/` - ✅ Open source (AGPL v3.0)
- `server/public/` - ✅ Open source (AGPL v3.0)
- `server/enterprise/` - ❌ Proprietary (do not touch)

All our custom implementations (`szncluster`, `sznmetrics`, `sznsearch`) are built from scratch using open source libraries and tools.

## 🚀 Quick Start

### 1. Enable Scheduled Posts

Set in `config.json`:
```json
{
  "ServiceSettings": {
    "ScheduledPosts": true
  }
}
```

### 2. Enable Custom User Groups

Set in `config.json` (default is true):
```json
{
  "ServiceSettings": {
    "EnableCustomGroups": true
  }
}
```

### 3. Enable SznCluster

Set in `config.json`:
```json
{
  "ClusterSettings": {
    "Enable": true,
    "ClusterName": "mattermost-cluster",
    "BindAddress": "",
    "AdvertiseAddress": ""
  }
}
```

Import in `server/cmd/mattermost/main.go`:
```go
import _ "github.com/mattermost/mattermost/server/v8/custom/szncluster"
```

### 4. Enable SznMetrics

Set in `config.json`:
```json
{
  "MetricsSettings": {
    "Enable": true,
    "BlockProfileRate": 0,
    "ListenAddress": ":8067"
  }
}
```

Import in `server/cmd/mattermost/main.go`:
```go
import _ "github.com/mattermost/mattermost/server/v8/custom/sznmetrics"
```

## 📊 Feature Comparison

| Feature | Upstream License | SZN Status | Implementation Type |
|---------|------------------|------------|---------------------|
| Scheduled Posts | Enterprise | ✅ Enabled | License check removal |
| Custom User Groups | Professional | ✅ Enabled | License check removal |
| Clustering | Enterprise | ✅ Enabled | Custom implementation |
| Metrics | Enterprise | ✅ Enabled | Custom implementation |
| Enhanced Search | Enterprise (Elasticsearch) | 🚧 In Progress | Custom implementation |
| LDAP Groups | Enterprise | ❌ Not modified | Requires LDAP |
| SAML | Enterprise | ❌ Not modified | Complex integration |
| Compliance Export | Enterprise | ❌ Not modified | Enterprise only |

## 🛠️ Development Guidelines

### Adding a New Custom Modification

1. **Research the feature:**
   - Is it in open source code?
   - What are the license checks?
   - What dependencies exist?

2. **Plan the approach:**
   - License check removal OR custom implementation?
   - Which files need modification?
   - What tests are needed?

3. **Implement the change:**
   - Follow minimal modification principle
   - Add clear comments with "SZN:" prefix
   - Document all changes

4. **Create documentation:**
   - Create `FEATURE_NAME.md` in `server/custom/`
   - Include architecture overview
   - Add configuration examples
   - Document testing checklist

5. **Test thoroughly:**
   - Unit tests if applicable
   - Integration tests
   - Manual testing
   - Performance testing

6. **Update this README:**
   - Add feature to the list
   - Update feature comparison table
   - Update maintenance checklist

## 📞 Support

For questions or issues with custom modifications:

1. **Check documentation** in respective `.md` files
2. **Review commit history** for similar issues
3. **Contact internal team** for Seznam.cz specific questions

## 📜 License

All custom modifications are licensed under **AGPL v3.0**, consistent with Mattermost Server's license.

```
Copyright (c) 2024-2026 Seznam.cz, a.s.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
```

## 🙏 Acknowledgments

- **Mattermost, Inc.** - For creating an excellent open source platform
- **Hashicorp** - For Memberlist library used in szncluster
- **Prometheus** - For metrics standards and libraries
- **Open Source Community** - For tools and inspiration

---

**Maintained by:** Seznam.cz, a.s.  
**Repository:** Internal fork of Mattermost Server  
**Last Updated:** January 2026  
**Mattermost Version:** 10.x compatible
