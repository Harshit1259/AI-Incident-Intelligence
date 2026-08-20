#  NeurOps Agent — Python SDK Plugin Base Classes
#  Copyright (c) NeurOps 2025. All rights reserved.
#
#  Every Python metric/runbook plugin inherits one of these ABCs.
#  The plugin engine calls the defined abstract methods with a "context"
#  dict and expects a "result" dict back.
#
#  MetricPlugin contract
#  ─────────────────────
#    discover(context)   → dict   Called once on startup / rediscovery.
#                                 Returns object metadata.
#    collect(context)    → dict   Called on every poll interval.
#                                 Returns metric key/value pairs.
#    rediscover(context) → dict   Optional. Called when object context
#                                 changes (e.g. after a config reload).
#
#  RunbookPlugin contract
#  ──────────────────────
#    init(context)       → dict   Allocate resources, open connections.
#    run(context)        → dict   Execute the remediation/action.
#    destroy(context)    → dict   Release resources unconditionally.
#
#  Result envelope expected by the engine:
#    {
#      "status":   "succeed" | "fail",
#      "result":   { <metric_name>: <value>, ... },
#      "error":    "...",          # only on failure
#      "error.code": "NO0xx"       # only on failure
#    }

from abc import ABC, abstractmethod


class MetricPlugin(ABC):
    """Base class for all NeurOps metric collection plugins."""

    def __init__(self):
        super().__init__()

    @abstractmethod
    def discover(self, context: dict) -> dict:
        """
        Called once at startup (and after config reload).
        Returns: result dict with object attributes and discovered sub-objects.

        Example return value:
          {
            "status": "succeed",
            "result": {
              "object.name": "web-server-01",
              "object.os":   "Ubuntu 22.04",
              "object.cpu.count": 8
            }
          }
        """
        ...

    @abstractmethod
    def collect(self, context: dict) -> dict:
        """
        Called on every poll cycle.
        Returns: result dict with metric key/value pairs.

        Example return value:
          {
            "status": "succeed",
            "result": {
              "system.cpu.used.percent": 42.5,
              "system.memory.used.bytes": 3221225472
            }
          }
        """
        ...

    def rediscover(self, context: dict) -> dict:
        """
        Optional. Override to handle object context changes.
        Default: delegates to discover().
        """
        return self.discover(context)

    # ── Convenience helpers ────────────────────────────────────────────────

    @staticmethod
    def succeed(result: dict) -> dict:
        """Wrap a result dict in the success envelope."""
        return {"status": "succeed", "result": result}

    @staticmethod
    def fail(error: str, error_code: str = "NO031") -> dict:
        """Wrap an error message in the failure envelope."""
        return {"status": "fail", "error": error, "error.code": error_code}


class RunbookPlugin(ABC):
    """Base class for all NeurOps runbook (remediation) plugins."""

    def __init__(self):
        super().__init__()

    @abstractmethod
    def init(self, context: dict) -> dict:
        """
        Allocate resources required to run the runbook.
        E.g. open SSH connection, authenticate to an API.
        Returns: result dict or error.
        """
        ...

    @abstractmethod
    def run(self, context: dict) -> dict:
        """
        Execute the remediation action.
        E.g. restart a service, clear a queue, reboot a VM.
        Returns: result dict with action output.
        """
        ...

    @abstractmethod
    def destroy(self, context: dict) -> dict:
        """
        Release all resources regardless of run() outcome.
        Always called — even if run() raised an exception.
        Returns: result dict (usually empty on clean teardown).
        """
        ...

    # ── Convenience helpers ────────────────────────────────────────────────

    @staticmethod
    def succeed(result: dict = None) -> dict:
        return {"status": "succeed", "result": result or {}}

    @staticmethod
    def fail(error: str, error_code: str = "NO031") -> dict:
        return {"status": "fail", "error": error, "error.code": error_code}
