from harness.provider import InstrumentationProvider
from providers.manual import ManualProvider
from providers.traceloop import TraceloopProvider

PROVIDERS: dict[str, InstrumentationProvider] = {
    "traceloop": TraceloopProvider(),
    "manual": ManualProvider(),
}
