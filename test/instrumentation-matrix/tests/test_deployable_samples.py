"""The deployable-sample registry maps frameworks that have a deployable agent
app to that app's build path, run command, and the span kinds the heavy driver
asserts. Three heavy-tier consumers read it, so these tests lock the contract."""
from harness.deployable_samples import DEPLOYABLE_SAMPLES, DeployableSample


def test_registry_covers_exactly_langchain_and_crewai():
    assert set(DEPLOYABLE_SAMPLES) == {"langchain", "crewai"}


def test_langchain_maps_to_customer_support_asserting_llm_only():
    s = DEPLOYABLE_SAMPLES["langchain"]
    assert isinstance(s, DeployableSample)
    assert s.app_path == "/customer-support-agent"
    assert s.run_command == "python main.py"
    assert s.expected_kinds == ("llm",)


def test_crewai_maps_to_crewai_agent_asserting_framework_kinds():
    s = DEPLOYABLE_SAMPLES["crewai"]
    assert s.app_path == "/crewai-agent"
    assert s.run_command == "python main.py"
    assert s.expected_kinds == ("llm", "agent", "crewaitask")


def test_crewai_sets_writable_home_and_storage_env():
    # CrewAI writes under $HOME at import; the deployed container's
    # HOME=/nonexistent is read-only, so the workload must repoint both the
    # home and the storage dir (before the interpreter/instrumentor starts).
    env = dict(DEPLOYABLE_SAMPLES["crewai"].env)
    assert env["HOME"] == "/tmp"
    assert env["CREWAI_STORAGE_DIR"] == "/tmp/crewai"


def test_langchain_declares_no_extra_env():
    assert DEPLOYABLE_SAMPLES["langchain"].env == ()
