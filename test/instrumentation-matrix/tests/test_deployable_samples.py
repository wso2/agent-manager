"""The deployable-sample registry maps frameworks that have a runnable agent
app to that app's directory, ASGI app, and the span kinds the heavy driver
asserts. Three heavy-tier consumers read it, so these tests lock the contract."""
from harness.deployable_samples import DEPLOYABLE_SAMPLES, DeployableSample


def test_registry_covers_exactly_langchain_and_crewai():
    assert set(DEPLOYABLE_SAMPLES) == {"langchain", "crewai"}


def test_langchain_maps_to_customer_support_asserting_llm_only():
    s = DEPLOYABLE_SAMPLES["langchain"]
    assert isinstance(s, DeployableSample)
    assert s.app_path == "/samples/customer-support-agent"
    assert s.asgi_app == "app:app"
    assert s.expected_kinds == ("llm",)


def test_crewai_maps_to_crewai_agent_asserting_framework_kinds():
    s = DEPLOYABLE_SAMPLES["crewai"]
    assert s.app_path == "/samples/crewai-agent"
    assert s.asgi_app == "app:app"
    assert s.expected_kinds == ("llm", "agent", "crewaitask")


def test_crewai_sets_writable_home_and_storage_env():
    # CrewAI writes under $HOME at import; point both the home and the storage
    # dir somewhere writable before the interpreter/instrumentor starts.
    env = dict(DEPLOYABLE_SAMPLES["crewai"].env)
    assert env["HOME"] == "/tmp"
    assert env["CREWAI_STORAGE_DIR"] == "/tmp/crewai"


def test_langchain_declares_no_extra_env():
    assert DEPLOYABLE_SAMPLES["langchain"].env == ()
