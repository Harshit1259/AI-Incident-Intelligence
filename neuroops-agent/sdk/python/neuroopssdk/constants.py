#  NeurOps Agent — Python SDK Constants
#  Copyright (c) NeurOps 2025. All rights reserved.

import os
import re


class Constant:

    # ── Transport ──────────────────────────────────────────────────────────────
    ZMQ_URL            = "tcp://{}"
    DEFAULT_PUB_PORT   = 9441
    DEFAULT_SUB_PORT   = 9440

    # ── Event types ───────────────────────────────────────────────────────────
    EVENT_TYPE_METRIC   = "metric"
    EVENT_TYPE_LOG      = "log"
    EVENT_TYPE_TRACE    = "trace"
    EVENT_TYPE_RUNBOOK  = "runbook"
    EVENT_TYPE_TOPOLOGY = "topology"

    # ── Plugin types ──────────────────────────────────────────────────────────
    PLUGIN_TYPE_METRIC   = "metric"
    PLUGIN_TYPE_RUNBOOK  = "runbook"
    PLUGIN_TYPE_TOPOLOGY = "topology"

    # ── Result keys ───────────────────────────────────────────────────────────
    RESULT          = "result"
    ERROR           = "error"
    ERROR_CODE      = "error.code"
    MESSAGE         = "message"
    STATUS          = "status"
    STATUS_SUCCEED  = "succeed"
    STATUS_FAIL     = "fail"
    CONTEXT         = "context"
    OBJECTS         = "objects"

    # ── Object context keys ───────────────────────────────────────────────────
    OBJECT_IP             = "object.ip"
    OBJECT_TYPE           = "object.type"
    OBJECT_NAME           = "object.name"
    OBJECT_HOST           = "object.host"
    OBJECT_CATEGORY       = "object.category"
    OBJECT_INSTANCE_ID    = "object.instance.id"
    OBJECT_HOST_UUID      = "object.host.uuid"
    OBJECT_CONTEXT        = "object.context"
    OBJECT_VENDOR         = "object.vendor"

    # ── Credential keys ───────────────────────────────────────────────────────
    USERNAME   = "username"
    PASSWORD   = "password"
    PORT       = "port"
    TIMEOUT    = "timeout"
    KEY        = "key"

    # ── Common metric names ───────────────────────────────────────────────────
    METRIC_NAME = "metric.name"
    AGENT_ID    = "agent.id"
    TIMESTAMP   = "timestamp"
    PLUGIN_ID   = "plugin.id"

    # ── Status values ─────────────────────────────────────────────────────────
    STATUS_UP          = "Up"
    STATUS_DOWN        = "Down"
    STATUS_UNREACHABLE = "Unreachable"
    STATUS_MAINTENANCE = "Maintenance"
    STATUS_UNKNOWN     = "Unknown"
    YES                = "yes"
    NO                 = "no"

    # ── Separators ────────────────────────────────────────────────────────────
    BLANK         = ""
    COLON         = ":"
    COMMA         = ","
    DOT           = "."
    PIPE          = "|"
    SPACE         = " "
    NEWLINE       = "\n"
    NL_SEP        = "\r\n"

    # ── Log levels ────────────────────────────────────────────────────────────
    LOG_LEVEL_TRACE = 0
    LOG_LEVEL_DEBUG = 1
    LOG_LEVEL_INFO  = 2
    LOG_LEVEL_WARN  = 3
    LOG_LEVEL_ERROR = 4
    LOG_LEVEL_FATAL = 5

    # ── Error codes ───────────────────────────────────────────────────────────
    ERROR_PING_FAILED               = "NO001"
    ERROR_INVALID_PORT              = "NO002"
    ERROR_INVALID_CREDENTIALS       = "NO003"
    ERROR_TIMEOUT                   = "NO004"
    ERROR_CONNECTION_FAILED         = "NO047"
    ERROR_COMMAND_EXECUTION_FAILED  = "NO059"
    ERROR_INVALID_HOST              = "NO310"
    ERROR_INTERNAL                  = "NO031"
    ERROR_SSL_FAILED                = "NO123"
    ERROR_UNAUTHORIZED_ACCESS       = "NO007"
    ERROR_BAD_RESPONSE              = "NO083"
    ERROR_NO_RESPONSE               = "NO056"
    ERROR_VENDOR_NOT_SUPPORTED      = "NO049"

    # ── Regex patterns ────────────────────────────────────────────────────────
    IP_PATTERN   = re.compile(
        r"^(?:(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}"
        r"(?:25[0-5]|2[0-4]\d|[01]?\d\d?)$"
    )
    MAC_PATTERN  = re.compile(r"^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$")

    # ── Time formats ──────────────────────────────────────────────────────────
    TIME_FORMAT       = "%Y-%m-%d %H:%M:%S"
    LOG_DATE_FORMAT   = "%d-%B-%Y"
    LOG_TIME_FORMAT   = "%I:%M:%S.%f %p"

    # ── Filesystem ────────────────────────────────────────────────────────────
    LOG_DIRECTORY = "logs"
    PATH_SEP      = os.sep
