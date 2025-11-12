# Plugin Signing Guide for Mattermost

This guide explains how to sign custom plugins and configure Mattermost server to accept them.

## Overview

Mattermost requires all prepackaged plugins to be digitally signed. The server verifies plugin signatures using:
1. **Hard-coded public key** - Built-in Mattermost public key for official plugins
2. **Custom public keys** - Admin-configured keys for custom/third-party plugins

## Why Sign Plugins?

- **Security**: Ensures plugins haven't been tampered with
- **Integrity**: Verifies the plugin comes from a trusted source
- **Requirement**: Prepackaged plugins ALWAYS require signatures (cannot be disabled)

## Prerequisites

- GPG (GNU Privacy Guard) installed
- Access to Mattermost server configuration
- Plugin bundle file (e.g., `my-plugin.tar.gz`)

## Part 1: Signing Your Custom Plugin

### Step 1: Generate GPG Key (if you don't have one)

```bash
gpg --gen-key
```

Follow the prompts:
- Enter your real name
- Enter your email address
- Choose a strong passphrase

### Step 2: Sign the Plugin

```bash
# Sign the plugin bundle (creates binary .sig file)
gpg --detach-sign your-plugin.tar.gz
```

This creates: `your-plugin.tar.gz.sig`

**Note:** The `.sig` file can be either:
- **Binary format** (default, same as official Mattermost plugins)
- **ASCII armored format** (add `--armor` flag)

Both formats are accepted by Mattermost server.

#### Alternative: ASCII Armored Signature

```bash
# Create ASCII armored signature (human-readable)
gpg --detach-sign --armor your-plugin.tar.gz
# Creates: your-plugin.tar.gz.asc

# Rename to .sig extension
mv your-plugin.tar.gz.asc your-plugin.tar.gz.sig
```

Or in one command:

```bash
gpg --detach-sign --armor --output your-plugin.tar.gz.sig your-plugin.tar.gz
```

### Step 3: Export Public Key

```bash
# Export your public key (ASCII armored format)
gpg --export --armor your-email@example.com > my-plugin-public-key.pub
```

Replace `your-email@example.com` with the email you used when generating the GPG key.

## Part 2: Configure Mattermost Server

### Step 1: Locate Configuration Directory

Find your Mattermost config directory (usually where `config.json` is located):

### Step 2: Add Public Key File

Copy your exported public key to the config directory:

```bash
cp my-plugin-public-key.pub /opt/mattermost/config/
```

### Step 3: Update config.json

Edit `config.json` and add your public key file to the `SignaturePublicKeyFiles` array:

```json
{
  "PluginSettings": {
    "SignaturePublicKeyFiles": ["my-plugin-public-key.pub"]
  }
}
```

**Multiple Keys Example:**

```json
{
  "PluginSettings": {
    "SignaturePublicKeyFiles": [
      "my-plugin-public-key.pub",
      "team-plugin-key.pub",
      "vendor-plugin-key.pub"
    ]
  }
}
```

### Step 4: Deploy Prepackaged Plugin

Copy both the plugin and signature to the prepackaged plugins directory:

```bash
# Copy plugin bundle and signature
cp your-plugin.tar.gz /opt/mattermost/prepackaged_plugins/
cp your-plugin.tar.gz.sig /opt/mattermost/prepackaged_plugins/
```

### Step 5: Enable the Plugin

Add or modify the plugin state in `config.json`:

```json
{
  "PluginSettings": {
    "PluginStates": {
      "your-plugin-id": {
        "Enable": true
      }
    }
  }
}
```

Replace `your-plugin-id` with the actual plugin ID from the plugin's manifest.

### Step 6: Restart Mattermost Server

```bash
sudo systemctl restart mattermost
```

Or for Docker:

```bash
docker restart mattermost
```

## Verification

Check the Mattermost logs to verify the plugin loaded successfully:

```bash
tail -f /opt/mattermost/logs/mattermost.log | grep "plugin_id=your-plugin-id"
```

Look for messages like:
- `"Processing prepackaged plugin"`
- `"Plugin signature verified using configured public key"`
- `"Plugin installed successfully"`

## Troubleshooting

### Plugin Not Loading

**Check signature file exists:**
```bash
ls -la /opt/mattermost/prepackaged_plugins/your-plugin.tar.gz*
```

You should see both:
- `your-plugin.tar.gz`
- `your-plugin.tar.gz.sig`

**Verify public key in config:**
```bash
cat /opt/mattermost/config/config.json | grep -A 2 "SignaturePublicKeyFiles"
```

**Check logs for signature errors:**
```bash
grep -i "signature" /opt/mattermost/logs/mattermost.log | tail -20
```

### Common Errors

1. **"Prepackaged plugin missing required signature file"**
   - Solution: Ensure `.sig` file exists and has correct naming

2. **"Unable to read configured signature public key file"**
   - Solution: Check public key file path in config.json
   - Verify file exists in config directory

3. **"Prepackaged plugin signature verification failed"**
   - Solution: Ensure plugin was signed with the correct private key
   - Verify public key matches the private key used for signing

4. **"Plugin not previously enabled"**
   - Solution: Set plugin state to enabled in config.json
   - Or enable via System Console

## Important Notes

- ✅ **Prepackaged plugins** ALWAYS require signatures (cannot be disabled)
- ✅ **File store plugins** respect `RequirePluginSignature` setting
- ✅ Public key files must be in the same directory as `config.json`
- ✅ Signature files must have `.sig` extension (not `.asc`)
- ✅ Both binary and armored signature formats are supported
- ✅ Multiple public keys can be configured for different plugin sources

## Code References

For developers wanting to understand the implementation:

- **Signature verification**: `server/channels/app/plugin_signature.go` - `verifyPlugin()` function
- **Prepackaged processing**: `server/channels/app/plugin.go` - `processPrepackagedPlugins()` function
- **Public key loading**: `server/channels/app/plugin_signature.go` - `AddPublicKey()` function
- **Build process**: `server/build/release.mk` - Shows how official plugins are signed

## Example: Complete Workflow

```bash
# 1. Generate GPG key (one time)
gpg --gen-key

# 2. Sign your plugin
cd /path/to/your/plugin
gpg --detach-sign my-awesome-plugin.tar.gz

# 3. Export public key
gpg --export --armor me@example.com > my-plugin-key.pub

# 4. Deploy to server
scp my-awesome-plugin.tar.gz mattermost-server:/opt/mattermost/prepackaged_plugins/
scp my-awesome-plugin.tar.gz.sig mattermost-server:/opt/mattermost/prepackaged_plugins/
scp my-plugin-key.pub mattermost-server:/opt/mattermost/config/

# 5. Update config.json on server
ssh mattermost-server
sudo vi /opt/mattermost/config/config.json
# Add "my-plugin-key.pub" to SignaturePublicKeyFiles array
# Add plugin to PluginStates with Enable: true

# 6. Restart Mattermost
sudo systemctl restart mattermost

# 7. Verify
tail -f /opt/mattermost/logs/mattermost.log
```

## Security Best Practices

1. **Protect Private Keys**: Never share or commit your private GPG keys
2. **Use Strong Passphrases**: Protect your GPG key with a strong passphrase
3. **Key Management**: Use separate keys for different purposes (personal, team, production)
4. **Backup Keys**: Keep secure backups of your GPG keys
5. **Revocation**: Be prepared to revoke compromised keys
6. **Verification**: Always verify signatures before deploying plugins

## Additional Resources

- [Mattermost Plugin Documentation](https://developers.mattermost.com/integrate/plugins/)
- [GPG Documentation](https://gnupg.org/documentation/)
- [OpenPGP Specification](https://www.openpgp.org/)
