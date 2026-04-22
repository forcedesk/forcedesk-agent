// Copyright © 2026 ForcePoint Software. All rights reserved.

package sshconn

import (
	"fmt"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// Config holds the parameters for a single SSH connection.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	Legacy   bool // Enables older key exchange algorithms for legacy network devices.
}

// legacyKex contains key exchange algorithms required by older network devices (e.g., Cisco, MikroTik).
// Includes weaker algorithms like diffie-hellman-group1-sha1 for compatibility.
var legacyKex = []string{
	"ecdh-sha2-nistp256",
	"ecdh-sha2-nistp384",
	"ecdh-sha2-nistp521",
	"diffie-hellman-group14-sha256",
	"diffie-hellman-group14-sha1",
	"diffie-hellman-group1-sha1",
}

// modernKex contains secure key exchange algorithms for modern SSH servers.
var modernKex = []string{
	"curve25519-sha256",
	"ecdh-sha2-nistp256",
	"ecdh-sha2-nistp384",
	"ecdh-sha2-nistp521",
	"diffie-hellman-group14-sha256",
}

// commandTimeout is the maximum time allowed for a command to produce output after
// the SSH session is established. This is separate from the dial timeout.
const commandTimeout = 60 * time.Second

// RunCommand opens an SSH session to the target host, executes the command, and returns the output.
// Supports both modern and legacy SSH configurations based on the Legacy flag.
func RunCommand(cfg Config, command string) (string, error) {
	// Default to the standard SSH port when the caller leaves Port at zero.
	port := cfg.Port
	if port == 0 {
		port = 22
	}

	// Select the appropriate key-exchange list. Legacy mode adds weaker
	// algorithms (e.g. diffie-hellman-group1-sha1) required by older Cisco
	// and MikroTik gear that does not support modern curves.
	kex := modernKex
	if cfg.Legacy {
		kex = legacyKex
	}

	// Build the SSH client config. HostKeyCallback is intentionally permissive
	// because network devices are identified by IP/hostname from the tenant
	// payload, not by a pre-shared host key.
	// The CBC ciphers and legacy MACs (hmac-sha1*) are included for the same
	// compatibility reason as legacyKex — many managed switches only offer them.
	sshCfg := &gossh.ClientConfig{
		User: cfg.Username,
		Auth: []gossh.AuthMethod{
			gossh.Password(cfg.Password),
		},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec
		Timeout:         15 * time.Second,
		Config: gossh.Config{
			KeyExchanges: kex,
			Ciphers: []string{
				"aes128-ctr", "aes192-ctr", "aes256-ctr",
				"aes128-cbc", "3des-cbc",
			},
			MACs: []string{
				"hmac-sha2-256-etm@openssh.com",
				"hmac-sha2-512-etm@openssh.com",
				"hmac-sha2-256", "hmac-sha1", "hmac-sha1-96",
			},
		},
	}

	// Open the TCP connection and perform the SSH handshake.
	addr := fmt.Sprintf("%s:%d", cfg.Host, port)
	client, err := gossh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return "", fmt.Errorf("dial %s: %w", addr, err)
	}
	defer client.Close()

	// Each command needs its own session; sessions are single-use in the SSH protocol.
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	// Run the command in a goroutine so we can impose a hard timeout via
	// a select. session.Output blocks until the remote side closes the channel,
	// which some devices never do — without this the call could hang forever.
	type result struct {
		out []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := session.Output(command)
		ch <- result{out, err}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			// Some devices close the connection immediately after sending output,
			// causing session.Output to return io.EOF. If there is output, the
			// command succeeded — treat any non-empty result as success.
			if len(r.out) > 0 {
				return string(r.out), nil
			}
			return "", fmt.Errorf("run command on %s: %w", cfg.Host, r.err)
		}
		return string(r.out), nil
	case <-time.After(commandTimeout):
		// Closing the session unblocks session.Output in the goroutine above,
		// allowing it to exit cleanly rather than leaking the goroutine.
		session.Close()
		return "", fmt.Errorf("command timed out on %s after %s", addr, commandTimeout)
	}
}
