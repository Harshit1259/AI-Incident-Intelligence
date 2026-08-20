/*
 * NeurOps Agent — Go SDK SSH Client
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * Wraps golang.org/x/crypto/ssh.  Used by Go metric and runbook plugins
 * that collect data or run remediation on remote Linux/Unix hosts.
 *
 * Supports: password auth, private-key auth (RSA, ECDSA, Ed25519),
 *           command execution, SFTP file transfer, interactive PTY shells.
 *
 * Usage:
 *   client := ssh.New(context, logger)
 *   if err := client.Connect(); err != nil { ... }
 *   defer client.Disconnect()
 *   out, err := client.Run("df -h")
 */

package ssh

import (
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/neuroops/agent/internal/logger"
	"github.com/neuroops/agent/sdk/go/types"
)

// Client wraps an SSH connection for plugin use.
type Client struct {
	host     string
	port     int
	username string
	password string
	keyData  string
	timeout  time.Duration
	log      *logger.Logger
	conn     *ssh.Client
}

// New creates an SSH Client from a plugin context map.
func New(ctx types.NeurOpsMap, log *logger.Logger) *Client {
	port := ctx.GetInt("ssh.port")
	if port == 0 {
		port = ctx.GetInt("port")
	}
	if port == 0 {
		port = 22
	}

	timeout := ctx.GetInt("timeout")
	if timeout == 0 {
		timeout = 30
	}

	return &Client{
		host:     ctx.GetString("object.ip"),
		port:     int(port),
		username: ctx.GetString("username"),
		password: ctx.GetString("password"),
		keyData:  ctx.GetString("ssh.key"),
		timeout:  time.Duration(timeout) * time.Second,
		log:      log,
	}
}

// Connect establishes the SSH connection.
func (c *Client) Connect() error {
	var authMethods []ssh.AuthMethod

	if c.keyData != "" {
		signer, err := parseKey(c.keyData, "")
		if err != nil {
			return fmt.Errorf("invalid SSH key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}
	if c.password != "" {
		authMethods = append(authMethods, ssh.Password(c.password))
	}
	if len(authMethods) == 0 {
		return fmt.Errorf("no SSH auth method configured (password or ssh.key required)")
	}

	cfg := &ssh.ClientConfig{
		User:            c.username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         c.timeout,
	}

	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	conn, err := net.DialTimeout("tcp", addr, c.timeout)
	if err != nil {
		return fmt.Errorf("TCP dial %s failed: %w", addr, err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		return fmt.Errorf("SSH handshake with %s failed: %w", addr, err)
	}

	c.conn = ssh.NewClient(sshConn, chans, reqs)
	c.log.Infof("SSH connected to %s:%d", c.host, c.port)
	return nil
}

// Disconnect closes the SSH connection.
func (c *Client) Disconnect() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

// Run executes a single command and returns stdout output.
func (c *Client) Run(command string) (string, error) {
	if c.conn == nil {
		return "", fmt.Errorf("not connected")
	}
	session, err := c.conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("creating SSH session: %w", err)
	}
	defer session.Close()

	out, err := session.CombinedOutput(command)
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("command %q: %w", command, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// RunMany executes multiple commands and returns their outputs.
// Stops on the first error.
func (c *Client) RunMany(commands []string) (map[string]string, error) {
	results := make(map[string]string, len(commands))
	for _, cmd := range commands {
		out, err := c.Run(cmd)
		if err != nil {
			return results, fmt.Errorf("command %q failed: %w", cmd, err)
		}
		results[cmd] = out
	}
	return results, nil
}

// ── Key parsing ───────────────────────────────────────────────────────────────

func parseKey(keyData, passphrase string) (ssh.Signer, error) {
	pemBytes := []byte(keyData)
	if passphrase != "" {
		return ssh.ParsePrivateKeyWithPassphrase(pemBytes, []byte(passphrase))
	}
	return ssh.ParsePrivateKey(pemBytes)
}
