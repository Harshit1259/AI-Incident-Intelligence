#  NeurOps Agent — Python SDK EventSubscriber
#  Copyright (c) NeurOps 2025. All rights reserved.
#
#  Wraps a ZeroMQ SUB socket.  The NeurOps product pushes commands
#  (poll triggers, runbook invocations, config updates) to agents
#  via this channel.
#
#  Usage:
#    def on_command(event: dict):
#        print("Received:", event)
#
#    sub = EventSubscriber("192.168.1.10", 9440, "NEUROOPS.CMD.", on_command, logger)
#    sub.start()
#    # ... later ...
#    sub.shutdown()

import base64
import json
import threading

import zmq

from neuroopssdk.constants import Constant


class EventSubscriber:

    def __init__(self,
                 host: str,
                 port: int,
                 event_topic: str,
                 callback,          # callable(event: dict)
                 logger):

        self._logger      = logger
        self._endpoint    = f"{host}:{port}"
        self._topic       = event_topic
        self._callback    = callback
        self._context     = zmq.Context()
        self._socket      = None
        self._shutdown    = False
        self._thread      = threading.Thread(
            target=self._receive,
            name="neuroops-subscriber",
            daemon=True,
        )

    # ── Public API ─────────────────────────────────────────────────────────

    def start(self) -> bool:
        """Connect and begin receiving commands in a background thread."""
        if self._callback is None:
            self._logger.fatal("EventSubscriber: callback must not be None")
            return False
        try:
            self._logger.info(f"EventSubscriber connecting to {self._endpoint}")
            self._socket = self._context.socket(zmq.SUB)
            self._socket.setsockopt(zmq.LINGER, 0)
            self._socket.set_hwm(5_000)
            self._socket.setsockopt(zmq.RCVTIMEO, -1)       # blocking
            self._socket.setsockopt_string(zmq.SUBSCRIBE, self._topic)
            self._socket.connect(Constant.ZMQ_URL.format(self._endpoint))
            self._thread.start()
            self._logger.info(
                f"EventSubscriber connected to {self._endpoint} "
                f"(topic={self._topic!r})"
            )
            return True
        except Exception:
            self._logger.error()
            return False

    def shutdown(self):
        """Stop the subscriber thread."""
        self._logger.info("EventSubscriber shutting down")
        self._shutdown = True
        try:
            self._context.term()        # unblocks the blocking recv
        except Exception:
            pass
        self._thread.join(timeout=5)
        self._logger.info("EventSubscriber stopped")

    # ── Internal ───────────────────────────────────────────────────────────

    def _receive(self):
        while not self._shutdown:
            try:
                raw = self._socket.recv_string()
                if not raw:
                    continue

                # Strip topic prefix, base64-decode, JSON-parse
                payload = raw[len(self._topic):] if raw.startswith(self._topic) else raw
                decoded = base64.b64decode(payload.encode())
                event   = json.loads(decoded)

                self._logger.trace(f"EventSubscriber received: {event}")
                self._callback(event)

            except zmq.ZMQError as e:
                if "Context was terminated" in str(e) or self._shutdown:
                    return
                self._logger.errorf("EventSubscriber ZMQ error: %s", str(e))
            except Exception:
                self._logger.error()
