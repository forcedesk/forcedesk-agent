# Security Documentation

This document outlines the security features and considerations for the ForceDesk Agent.

## Security Features

### 1. Secure Credential Storage

**API Keys and Passwords:**
- Sensitive credentials (API keys, passwords) are stored using secure memory management
- In-memory credentials are zeroed out when no longer needed using the `secure.String` type
- Config file (`config.toml`) is created with restrictive permissions (0600 - owner read/write only)
- Credentials are never logged in debug output

**Database Encryption:**
- Database encryption key is generated on first run and stored in `.dbkey` with 0600 permissions
- 256-bit AES encryption key derived using SHA-256
- Key is unique per installation and stored securely on disk

### 2. Network Security

**TLS/SSL:**
- SSL certificate verification is enabled by default
- Warning logged if SSL verification is disabled (`VerifySSL: false`)
- Risk: Disabling SSL verification makes connections vulnerable to Man-in-the-Middle attacks

**Rate Limiting:**
- Built-in rate limiting on all HTTP requests to tenant server
- Maximum 600 requests per minute (100 token bucket, refill 1 token/100ms)
- Prevents abuse if agent is compromised
- Automatic throttling with warning logs when limit is reached

**HTTP Request Timeouts:**
- All HTTP requests use context with 15-30 second timeouts
- Prevents hung connections and resource exhaustion

### 3. Command Execution Security

**SSH Command Allowlist:**
- Device query commands are validated against a strict allowlist (internal/tasks/devicequery.go:14-27)
- Only pre-approved network device commands can be executed
- Rejects any command not explicitly permitted

**Hostname Validation:**
- Ping and network monitoring validates hostnames to prevent command injection
- Only alphanumeric characters, dots, hyphens, colons, and brackets allowed
- Maximum hostname length: 253 characters
- Invalid hostnames are rejected and logged

### 4. SSH Configuration

**Known Limitations:**
- ⚠️ SSH host key verification is DISABLED (`InsecureIgnoreHostKey`)
- Risk: Vulnerable to Man-in-the-Middle attacks on SSH connections
- Reason: Required for automated device management across many network devices

**Cryptographic Algorithms:**
- Modern mode uses secure algorithms (curve25519-sha256, AES-CTR, HMAC-SHA2)
- Legacy mode enables weaker algorithms (DH-group1-sha1, 3DES-CBC, HMAC-SHA1) for compatibility
- ⚠️ Legacy mode should only be used when connecting to old network devices
- Risk: Weaker algorithms are susceptible to cryptographic attacks

### 5. File Permissions

**Restrictive Permissions:**
- Config file: 0600 (owner read/write only)
- Database encryption key: 0600 (owner read/write only)
- Database file: Default system permissions
- Log files: 0644 (owner read/write, others read)
- Data directory: 0755

### 6. Logging Security

**Sensitive Data Protection:**
- API keys, passwords, and credentials are NEVER logged
- Request bodies are not logged (only size is logged in debug mode)
- SSH command output may contain sensitive information - review logs carefully

## Security Best Practices

### Configuration

1. **Always enable SSL verification in production:**
   ```toml
   [tenant]
   verify_ssl = true
   ```

2. **Use strong API keys:**
   - Minimum 32 characters
   - Mix of alphanumeric and special characters
   - Rotate keys regularly

3. **Run as dedicated service account:**
   - Windows: Runs as LocalSystem by default
   - Consider using a dedicated service account with minimum required privileges

### Network

1. **Firewall configuration:**
   - Only allow outbound HTTPS to tenant server
   - Restrict SSH access to network devices to agent's IP

2. **Monitor rate limiting:**
   - Check logs for "rate limit reached" warnings
   - May indicate abuse or misconfiguration

### SSH Security

1. **Use modern SSH algorithms when possible:**
   - Only enable legacy mode (`is_cisco_legacy`) for devices that require it
   - Document which devices use legacy mode

2. **Secure device credentials:**
   - Use strong passwords for network devices
   - Rotate credentials regularly
   - Limit credentials to read-only commands where possible

## Incident Response

### If the agent is compromised:

1. **Immediately:**
   - Stop the agent service
   - Rotate all API keys in the tenant admin panel
   - Change all network device passwords

2. **Investigation:**
   - Review agent logs for suspicious activity
   - Check for unauthorized command executions
   - Review network device configurations for changes

3. **Recovery:**
   - Delete `config.toml` and `.dbkey`
   - Reinstall agent
   - Reconfigure with new credentials

## Vulnerability Reporting

If you discover a security vulnerability:

1. **DO NOT** create a public GitHub issue
2. Email security concerns to the ForceDesk team
3. Include detailed steps to reproduce
4. Allow time for patch development before public disclosure

## Security Audit Log

| Date | Change | Impact |
|------|--------|--------|
| 2026-02-22 | Added hostname validation for ping | Prevents command injection |
| 2026-02-22 | Implemented secure password storage | Protects credentials in memory |
| 2026-02-22 | Added database encryption | Protects data at rest |
| 2026-02-22 | Implemented rate limiting | Prevents abuse/amplification |
| 2026-02-22 | Secured config file permissions (0600) | Prevents unauthorized access |
| 2026-02-22 | Added SSL verification warning | Alerts users to MITM risk |
| 2026-02-22 | Added HTTP request timeouts | Prevents resource exhaustion |
| 2026-02-22 | Sanitized debug logs | Prevents credential leaks |

## Compliance

This agent handles sensitive data including:
- Network device credentials
- API authentication tokens
- Device configurations
- User account information (PaperCut integration)

Ensure your deployment complies with:
- Organizational security policies
- Data protection regulations (GDPR, etc.)
- Network security standards
- Access control requirements
