#  NeurOps Agent — Python SDK Logger
#  Copyright (c) NeurOps 2025. All rights reserved.
#
#  Structured, levelled, daily-rotating logger.
#  Writes JSON lines to logs/<component>-<date>.log AND stdout.
#
#  Usage:
#    from neuroopssdk.logger import Logger
#    log = Logger("my_plugin", level=2)
#    log.info("Plugin started")
#    log.errorf("Connection failed: %s", str(err))

import json
import os
import sys
import threading
import traceback
from datetime import datetime


class Logger:

    TRACE = 0
    DEBUG = 1
    INFO  = 2
    WARN  = 3
    ERROR = 4
    FATAL = 5

    _LEVEL_NAMES = {0: "TRACE", 1: "DEBUG", 2: "INFO", 3: "WARN", 4: "ERROR", 5: "FATAL"}

    def __init__(self, component: str, level: int = 2, log_dir: str = "logs"):
        self._component = component
        self._level     = level
        self._log_dir   = log_dir
        self._lock      = threading.Lock()
        self._file      = None
        self._date_key  = None
        os.makedirs(log_dir, exist_ok=True)
        self._rotate()

    # ── Public API ─────────────────────────────────────────────────────────

    def set_level(self, level: int):
        self._level = level

    def trace(self, msg: str):  self._log(self.TRACE, msg)
    def debug(self, msg: str):  self._log(self.DEBUG, msg)
    def info(self,  msg: str):  self._log(self.INFO,  msg)
    def warn(self,  msg: str):  self._log(self.WARN,  msg)
    def error(self, msg: str = None):
        if msg is None:
            msg = traceback.format_exc()
        self._log(self.ERROR, msg)
    def fatal(self, msg: str):  self._log(self.FATAL, msg)

    def tracef(self, fmt: str, *args): self.trace(fmt % args)
    def debugf(self, fmt: str, *args): self.debug(fmt % args)
    def infof(self,  fmt: str, *args): self.info(fmt  % args)
    def warnf(self,  fmt: str, *args): self.warn(fmt  % args)
    def errorf(self, fmt: str, *args): self.error(fmt % args)
    def fatalf(self, fmt: str, *args): self.fatal(fmt % args)

    def with_component(self, name: str) -> "Logger":
        """Return a child logger with a different component label."""
        child = Logger.__new__(Logger)
        child._component = name
        child._level     = self._level
        child._log_dir   = self._log_dir
        child._lock      = self._lock
        child._file      = self._file
        child._date_key  = self._date_key
        return child

    # ── Internal ───────────────────────────────────────────────────────────

    def _log(self, level: int, msg: str):
        if level < self._level:
            return

        now    = datetime.now()
        today  = now.strftime("%Y-%m-%d")

        with self._lock:
            if today != self._date_key:
                self._rotate(today)

            record = json.dumps({
                "timestamp": now.strftime("%Y-%m-%d %H:%M:%S.%f")[:-3],
                "level":     self._LEVEL_NAMES.get(level, "INFO"),
                "component": self._component,
                "message":   str(msg),
            })
            line = record + "\n"
            try:
                if self._file:
                    self._file.write(line)
                    self._file.flush()
            except Exception:
                pass
            print(line, end="", file=sys.stdout, flush=True)

    def _rotate(self, today: str = None):
        if today is None:
            today = datetime.now().strftime("%Y-%m-%d")
        if self._file:
            try:
                self._file.close()
            except Exception:
                pass
        safe_name = self._component.replace("/", "-").replace("\\", "-")
        path = os.path.join(self._log_dir, f"{safe_name}-{today}.log")
        try:
            self._file = open(path, "a", encoding="utf-8")
        except Exception:
            self._file = None
        self._date_key = today
