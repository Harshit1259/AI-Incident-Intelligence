#  NeurOps Agent — Python SDK EventPublisher
#  Copyright (c) NeurOps 2025. All rights reserved.
#
#  Wraps a ZeroMQ PUSH socket.  Plugin output is JSON-serialised,
#  base64-encoded, prefixed with an event topic, and sent to the
#  NeurOps product (or agent bridge) over TCP.
#
#  Wire format:  <topic><base64(json_payload)>\n
#
#  Usage:
#    pub = EventPublisher("192.168.1.10", 9441, "NEUROOPS.AGENT.", logger)
#    pub.start()
#    pub.publish({"event.type": "metric", "cpu.used.percent": 42.5})
#    pub.shutdown()

import base64
import json
import queue
import threading
import time

import zmq

from neuroopssdk.constants import Constant


class EventPublisher:

    def __init__(self,
                 host: str,
                 port: int,
                 event_topic: str,
                 logger,
                 queue_size: int = 100_000):

        self._logger        = logger
        self._endpoint      = f"{host}:{port}"
        self._event_topic   = event_topic
        self._context       = zmq.Context()
        self._socket        = None
        self._shutdown      = False
        self._event_queue   = queue.Queue(maxsize=queue_size)
        self._thread        = threading.Thread(
            target=self._drain,
            name="neuroops-publisher",
            daemon=True,
        )

    # ── Public API ─────────────────────────────────────────────────────────

    def start(self) -> bool:
        """Connect to the product and start the background drain thread."""
        try:
            self._logger.info(f"EventPublisher connecting to {self._endpoint}")
            self._socket = self._context.socket(zmq.PUSH)
            self._socket.setsockopt(zmq.LINGER, 0)
            self._socket.set_hwm(5_000)
            self._socket.connect(Constant.ZMQ_URL.format(self._endpoint))
            time.sleep(0.5)           # brief settle after connect
            self._thread.start()
            self._logger.info(f"EventPublisher connected to {self._endpoint}")
            return True
        except Exception:
            self._logger.error()
            return False

    def publish(self, event: dict) -> bool:
        """
        Enqueue a single event dict for delivery.
        Returns False if the queue is full (backpressure signal).
        """
        if self._socket is None or self._shutdown:
            return False
        try:
            self._event_queue.put_nowait(event)
            return True
        except queue.Full:
            self._logger.warn(
                "EventPublisher queue full — event dropped "
                "(product may be unreachable)"
            )
            return False

    def publish_batch(self, events: list) -> int:
        """Enqueue multiple events. Returns number successfully enqueued."""
        return sum(1 for e in events if self.publish(e))

    def queue_depth(self) -> int:
        return self._event_queue.qsize()

    def shutdown(self):
        """Flush remaining events and disconnect."""
        self._logger.info("EventPublisher shutting down")
        self._shutdown = True
        self._event_queue.put(None)     # sentinel to unblock drain thread
        self._thread.join(timeout=5)
        try:
            if self._socket:
                self._socket.close()
            if self._context:
                self._context.term()
        except Exception:
            pass
        self._logger.info("EventPublisher stopped")

    # ── Internal ───────────────────────────────────────────────────────────

    def _drain(self):
        while not self._shutdown:
            try:
                event = self._event_queue.get(timeout=1)
                if event is None:
                    break
                encoded = (
                    self._event_topic
                    + base64.b64encode(json.dumps(event).encode()).decode()
                )
                self._socket.send_string(encoded, zmq.NOBLOCK)
            except queue.Empty:
                continue
            except Exception:
                self._logger.error()
            finally:
                try:
                    self._event_queue.task_done()
                except Exception:
                    pass
