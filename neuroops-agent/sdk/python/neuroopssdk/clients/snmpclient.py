#  NeurOps Agent — Python SDK SNMP Client
#  Copyright (c) NeurOps 2025. All rights reserved.
#
#  Wraps pysnmp for SNMP v1/v2c/v3 GET, GETNEXT, WALK, BULK, and SET.
#  Used by network device metric plugins (routers, switches, firewalls,
#  UPS, storage arrays, printers, and any SNMP-managed device).
#
#  Usage:
#    client = SNMPClient(context, logger)
#    result = client.get(["1.3.6.1.2.1.1.1.0", "1.3.6.1.2.1.1.5.0"])
#    result = client.walk("1.3.6.1.2.1.2.2")     # IF-MIB interfaces

from pysnmp.hlapi import (
    CommunityData,
    ContextData,
    Integer32,
    ObjectIdentity,
    ObjectType,
    OctetString,
    SnmpEngine,
    UdpTransportTarget,
    UsmUserData,
    authHMACMD5Protocol,
    authHMACSHAProtocol,
    getCmd,
    nextCmd,
    setCmd,
    bulkCmd,
    usmDESPrivProtocol,
    usmAesCfb128Protocol,
    usmNoAuthProtocol,
    usmNoPrivProtocol,
)

from neuroopssdk.constants import Constant


class SNMPClient:

    # SNMP versions
    V1  = "v1"
    V2C = "v2c"
    V3  = "v3"

    # SNMPv3 auth/priv protocols
    AUTH_MD5    = "MD5"
    AUTH_SHA    = "SHA"
    PRIV_DES    = "DES"
    PRIV_AES128 = "AES128"

    DEFAULT_PORT    = 161
    DEFAULT_TIMEOUT = 5
    DEFAULT_RETRIES = 1

    def __init__(self, context: dict, logger):
        self._log      = logger
        self._host     = context.get(Constant.OBJECT_IP, "")
        self._port     = int(context.get("snmp.port", self.DEFAULT_PORT))
        self._version  = context.get("snmp.version", self.V2C).lower()
        self._timeout  = int(context.get("snmp.timeout", self.DEFAULT_TIMEOUT))
        self._retries  = int(context.get("snmp.retries", self.DEFAULT_RETRIES))

        # v1/v2c
        self._community = context.get("snmp.community", "public")

        # v3
        self._username     = context.get(Constant.USERNAME, "")
        self._auth_proto   = context.get("snmp.auth.protocol", self.AUTH_SHA)
        self._auth_pass    = context.get("snmp.auth.password", "")
        self._priv_proto   = context.get("snmp.priv.protocol", self.PRIV_AES128)
        self._priv_pass    = context.get("snmp.priv.password", "")
        self._context_name = context.get("snmp.context", "")

        self._engine    = SnmpEngine()

    # ── Public API ─────────────────────────────────────────────────────────

    def get(self, oids: list) -> dict:
        """
        Perform SNMP GET for a list of OIDs.
        Returns: {"status": "succeed", "result": {"1.3.6.1...": "value", ...}}
        """
        try:
            result = {}
            objects = [ObjectType(ObjectIdentity(oid)) for oid in oids]
            error_indication, error_status, error_index, var_binds = next(
                getCmd(
                    self._engine,
                    self._auth_data(),
                    self._transport(),
                    ContextData(),
                    *objects,
                )
            )
            if error_indication:
                return self._fail(str(error_indication), Constant.ERROR_NO_RESPONSE)
            if error_status:
                return self._fail(
                    f"SNMP error at {error_index}: {error_status.prettyPrint()}",
                    Constant.ERROR_BAD_RESPONSE,
                )
            for var_bind in var_binds:
                oid, value = var_bind
                result[str(oid)] = self._decode_value(value)
            return {"status": Constant.STATUS_SUCCEED, "result": result}
        except Exception as e:
            return self._fail(str(e), Constant.ERROR_INTERNAL)

    def walk(self, base_oid: str) -> dict:
        """
        Perform SNMP WALK starting at base_oid.
        Returns: {"status": "succeed", "result": {<oid>: <value>, ...}}
        """
        try:
            result = {}
            for error_indication, error_status, _, var_binds in nextCmd(
                self._engine,
                self._auth_data(),
                self._transport(),
                ContextData(),
                ObjectType(ObjectIdentity(base_oid)),
                lexicographicMode=False,
            ):
                if error_indication:
                    return self._fail(str(error_indication), Constant.ERROR_NO_RESPONSE)
                if error_status:
                    return self._fail(str(error_status.prettyPrint()), Constant.ERROR_BAD_RESPONSE)
                for var_bind in var_binds:
                    oid, value = var_bind
                    result[str(oid)] = self._decode_value(value)
            return {"status": Constant.STATUS_SUCCEED, "result": result}
        except Exception as e:
            return self._fail(str(e), Constant.ERROR_INTERNAL)

    def bulk_walk(self, base_oid: str, max_repetitions: int = 25) -> dict:
        """
        Perform SNMP BULK WALK (v2c/v3 only).
        More efficient than walk() for large tables.
        """
        try:
            result = {}
            for error_indication, error_status, _, var_binds in bulkCmd(
                self._engine,
                self._auth_data(),
                self._transport(),
                ContextData(),
                0, max_repetitions,
                ObjectType(ObjectIdentity(base_oid)),
                lexicographicMode=False,
            ):
                if error_indication:
                    return self._fail(str(error_indication), Constant.ERROR_NO_RESPONSE)
                if error_status:
                    break
                for var_bind in var_binds:
                    oid, value = var_bind
                    result[str(oid)] = self._decode_value(value)
            return {"status": Constant.STATUS_SUCCEED, "result": result}
        except Exception as e:
            return self._fail(str(e), Constant.ERROR_INTERNAL)

    def set(self, oid: str, value, value_type: str = "string") -> dict:
        """
        Perform SNMP SET.
        value_type: "string" | "integer" | "oid"
        """
        try:
            if value_type == "integer":
                snmp_value = Integer32(int(value))
            else:
                snmp_value = OctetString(str(value))

            error_indication, error_status, error_index, _ = next(
                setCmd(
                    self._engine,
                    self._auth_data(),
                    self._transport(),
                    ContextData(),
                    ObjectType(ObjectIdentity(oid), snmp_value),
                )
            )
            if error_indication:
                return self._fail(str(error_indication), Constant.ERROR_NO_RESPONSE)
            if error_status:
                return self._fail(str(error_status.prettyPrint()), Constant.ERROR_BAD_RESPONSE)
            return {"status": Constant.STATUS_SUCCEED}
        except Exception as e:
            return self._fail(str(e), Constant.ERROR_INTERNAL)

    def get_table(self, column_oids: list) -> dict:
        """
        Walk multiple OID columns in parallel and assemble as a table.
        Returns: {"status": "succeed", "result": {<index>: {<oid>: <val>}}}
        """
        table = {}
        for oid in column_oids:
            r = self.walk(oid)
            if r["status"] != Constant.STATUS_SUCCEED:
                return r
            for full_oid, value in r["result"].items():
                # Extract table index (last component)
                idx = full_oid.rsplit(".", 1)[-1]
                table.setdefault(idx, {})[oid] = value
        return {"status": Constant.STATUS_SUCCEED, "result": table}

    # ── Helpers ────────────────────────────────────────────────────────────

    def _auth_data(self):
        if self._version in (self.V1, self.V2C):
            mp_model = 0 if self._version == self.V1 else 1
            return CommunityData(self._community, mpModel=mp_model)

        # SNMPv3
        auth_protocol = {
            self.AUTH_MD5: authHMACMD5Protocol,
            self.AUTH_SHA: authHMACSHAProtocol,
        }.get(self._auth_proto.upper(), usmNoAuthProtocol)

        priv_protocol = {
            self.PRIV_DES:    usmDESPrivProtocol,
            self.PRIV_AES128: usmAesCfb128Protocol,
        }.get(self._priv_proto.upper(), usmNoPrivProtocol)

        return UsmUserData(
            self._username,
            authKey=self._auth_pass or None,
            privKey=self._priv_pass or None,
            authProtocol=auth_protocol,
            privProtocol=priv_protocol,
        )

    def _transport(self):
        return UdpTransportTarget(
            (self._host, self._port),
            timeout=self._timeout,
            retries=self._retries,
        )

    @staticmethod
    def _decode_value(value) -> any:
        """Convert pysnmp value objects to native Python types."""
        try:
            # Try integer first
            return int(value)
        except Exception:
            pass
        try:
            # Try string
            s = value.prettyPrint()
            # Strip hex prefix if pysnmp added it
            if s.startswith("0x"):
                try:
                    return bytes.fromhex(s[2:]).decode(errors="replace")
                except Exception:
                    pass
            return s
        except Exception:
            return str(value)

    def _fail(self, message: str, code: str = Constant.ERROR_INTERNAL) -> dict:
        self._log.errorf("SNMPClient [%s]: %s", self._host, message)
        return {
            "status":     Constant.STATUS_FAIL,
            "error":      message,
            "error.code": code,
        }
