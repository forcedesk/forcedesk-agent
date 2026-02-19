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
	Legacy   bool // enables older KEx algorithms for legacy Cisco devices
}

// legacyKex includes weak algorithms required by older Cisco/MikroTik devices.
var legacyKex = []string{
	"ecdh-sha2-nistp256",
	"ecdh-sha2-nistp384",
	"ecdh-sha2-nistp521",
	"diffie-hellman-group14-sha256",
	"diffie-hellman-group14-sha1",
	"diffie-hellman-group1-sha1",
}

var modernKex = []string{
	"curve25519-sha256",
	"ecdh-sha2-nistp256",
	"ecdh-sha2-nistp384",
	"ecdh-sha2-nistp521",
	"diffie-hellman-group14-sha256",
}

// RunCommand opens an SSH session to the target host, runs command, and
// returns the combined stdout output.
func RunCommand(cfg Config, command string) (string, error) {
	port := cfg.Port
	if port == 0 {
		port = 22
	}

	kex := modernKex
	if cfg.Legacy {
		kex = legacyKex
	}

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

	addr := fmt.Sprintf("%s:%d", cfg.Host, port)
	client, err := gossh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return "", fmt.Errorf("dial %s: %w", addr, err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	out, err := session.Output(command)
	if err != nil {
		// Some devices close the connection after sending output; treat
		// non-empty output with an error as partial success.
		if len(out) > 0 {
			return string(out), nil
		}
		return "", fmt.Errorf("run command on %s: %w", cfg.Host, err)
	}
	return string(out), nil
}
