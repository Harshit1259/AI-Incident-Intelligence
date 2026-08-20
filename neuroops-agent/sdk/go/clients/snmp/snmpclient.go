/*
 * NeurOps Agent — Go SDK SNMP Client
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * Wraps gosnmp for SNMP v1/v2c/v3 GET, WALK, BULK, and SET.
 * Used by network device metric plugins.
 *
 * Usage:
 *   client := snmp.New(context, logger)
 *   if err := client.Connect(); err != nil { ... }
 *   defer client.Disconnect()
 *   result, err := client.Get([]string{"1.3.6.1.2.1.1.1.0"})
 */

package snmp

import (
	"fmt"
	"strings"
	"time"

	g "github.com/gosnmp/gosnmp"

	"github.com/neuroops/agent/internal/logger"
	"github.com/neuroops/agent/sdk/go/types"
)

// Client wraps a gosnmp session.
type Client struct {
	session *g.GoSNMP
	log     *logger.Logger
}

// New creates an SNMP Client from a plugin context map.
func New(ctx types.NeurOpsMap, log *logger.Logger) *Client {
	port := uint16(ctx.GetInt("snmp.port"))
	if port == 0 {
		port = 161
	}

	timeout := time.Duration(ctx.GetInt("snmp.timeout")) * time.Second
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	retries := int(ctx.GetInt("snmp.retries"))
	if retries == 0 {
		retries = 1
	}

	version := resolveVersion(ctx.GetString("snmp.version"))
	community := ctx.GetString("snmp.community")
	if community == "" {
		community = "public"
	}

	session := &g.GoSNMP{
		Target:    ctx.GetString("object.ip"),
		Port:      port,
		Version:   version,
		Community: community,
		Timeout:   timeout,
		Retries:   retries,
		MaxOids:   g.MaxOids,
	}

	// SNMPv3 parameters
	if version == g.Version3 {
		session.SecurityModel = g.UserSecurityModel
		session.MsgFlags      = resolveSecLevel(ctx)
		session.SecurityParameters = &g.UsmSecurityParameters{
			UserName:                 ctx.GetString("username"),
			AuthenticationProtocol:  resolveAuthProto(ctx.GetString("snmp.auth.protocol")),
			AuthenticationPassphrase: ctx.GetString("snmp.auth.password"),
			PrivacyProtocol:          resolvePrivProto(ctx.GetString("snmp.priv.protocol")),
			PrivacyPassphrase:        ctx.GetString("snmp.priv.password"),
		}
	}

	return &Client{session: session, log: log}
}

// Connect opens the SNMP session.
func (c *Client) Connect() error {
	return c.session.Connect()
}

// Disconnect closes the session.
func (c *Client) Disconnect() {
	if c.session.Conn != nil {
		_ = c.session.Conn.Close()
	}
}

// Get performs SNMP GET for the given OIDs.
// Returns a map of OID → value.
func (c *Client) Get(oids []string) (map[string]any, error) {
	result, err := c.session.Get(oids)
	if err != nil {
		return nil, fmt.Errorf("SNMP GET: %w", err)
	}
	return c.decodeVarBinds(result.Variables), nil
}

// Walk performs SNMP WALK starting at baseOID.
func (c *Client) Walk(baseOID string) (map[string]any, error) {
	result := make(map[string]any)
	err := c.session.Walk(baseOID, func(pdu g.SnmpPDU) error {
		result[pdu.Name] = decodeValue(pdu)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("SNMP WALK %s: %w", baseOID, err)
	}
	return result, nil
}

// BulkWalk performs SNMP BULK WALK (v2c/v3 only).
func (c *Client) BulkWalk(baseOID string) (map[string]any, error) {
	result := make(map[string]any)
	err := c.session.BulkWalk(baseOID, func(pdu g.SnmpPDU) error {
		result[pdu.Name] = decodeValue(pdu)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("SNMP BULKWALK %s: %w", baseOID, err)
	}
	return result, nil
}

// GetTable walks multiple OID columns and assembles them into a table
// keyed by row index (last component of each OID).
func (c *Client) GetTable(columnOIDs []string) (map[string]map[string]any, error) {
	table := make(map[string]map[string]any)
	for _, oid := range columnOIDs {
		rows, err := c.Walk(oid)
		if err != nil {
			return nil, err
		}
		for fullOID, value := range rows {
			idx := lastComponent(fullOID)
			if table[idx] == nil {
				table[idx] = make(map[string]any)
			}
			table[idx][oid] = value
		}
	}
	return table, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (c *Client) decodeVarBinds(pdus []g.SnmpPDU) map[string]any {
	result := make(map[string]any, len(pdus))
	for _, pdu := range pdus {
		result[pdu.Name] = decodeValue(pdu)
	}
	return result
}

func decodeValue(pdu g.SnmpPDU) any {
	switch pdu.Type {
	case g.Integer, g.Counter32, g.Gauge32, g.TimeTicks,
		g.Counter64, g.Uinteger32:
		switch v := pdu.Value.(type) {
		case uint:
			return int64(v)
		case uint32:
			return int64(v)
		case uint64:
			return int64(v)
		case int:
			return int64(v)
		default:
			return pdu.Value
		}
	case g.OctetString:
		if b, ok := pdu.Value.([]byte); ok {
			return string(b)
		}
		return fmt.Sprintf("%v", pdu.Value)
	case g.ObjectIdentifier:
		return fmt.Sprintf("%v", pdu.Value)
	default:
		return pdu.Value
	}
}

func lastComponent(oid string) string {
	parts := strings.Split(strings.TrimPrefix(oid, "."), ".")
	if len(parts) == 0 {
		return oid
	}
	return parts[len(parts)-1]
}

func resolveVersion(v string) g.SnmpVersion {
	switch strings.ToLower(strings.TrimPrefix(v, "v")) {
	case "1":
		return g.Version1
	case "3":
		return g.Version3
	default:
		return g.Version2c
	}
}

func resolveSecLevel(ctx types.NeurOpsMap) g.SnmpV3MsgFlags {
	authPass := ctx.GetString("snmp.auth.password")
	privPass := ctx.GetString("snmp.priv.password")
	if authPass != "" && privPass != "" {
		return g.AuthPriv
	}
	if authPass != "" {
		return g.AuthNoPriv
	}
	return g.NoAuthNoPriv
}

func resolveAuthProto(proto string) g.SnmpV3AuthProtocol {
	switch strings.ToUpper(proto) {
	case "MD5":
		return g.MD5
	default:
		return g.SHA
	}
}

func resolvePrivProto(proto string) g.SnmpV3PrivProtocol {
	switch strings.ToUpper(proto) {
	case "DES":
		return g.DES
	default:
		return g.AES
	}
}
