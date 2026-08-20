#  NeurOps Agent — Python SDK
#  Copyright (c) NeurOps 2025. All rights reserved.
#
#  neuroopssdk — the official Python SDK for writing NeurOps agent plugins.
#
#  Quick-start for a metric plugin:
#
#    from neuroopssdk.plugin import MetricPlugin
#    from neuroopssdk.logger import Logger
#
#    class MyPlugin(MetricPlugin):
#        def discover(self, context):
#            return self.succeed({"object.name": context.get("object.ip")})
#
#        def collect(self, context):
#            return self.succeed({"my.metric": 42})

from neuroopssdk.plugin import MetricPlugin, RunbookPlugin
from neuroopssdk.logger import Logger
from neuroopssdk.constants import Constant
from neuroopssdk.eventpublisher import EventPublisher
from neuroopssdk.eventsubscriber import EventSubscriber

__version__ = "1.0.0"
__all__ = [
    "MetricPlugin",
    "RunbookPlugin",
    "Logger",
    "Constant",
    "EventPublisher",
    "EventSubscriber",
]
