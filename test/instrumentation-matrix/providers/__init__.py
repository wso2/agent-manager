from harness.provider import InstrumentationProvider
from providers.traceloop import TraceloopProvider

PROVIDERS: dict[str, InstrumentationProvider] = {
    "traceloop": TraceloopProvider(),
}
