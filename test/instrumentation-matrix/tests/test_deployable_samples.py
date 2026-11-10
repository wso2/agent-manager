"""The deployable-sample registry maps frameworks that have a deployable agent
app to that app's build path, run command, and the span kinds the heavy driver
asserts. Three heavy-tier consumers read it, so these tests lock the contract."""
from harness.deployable_samples import DEPLOYABLE_SAMPLES, DeployableSample


def test_registry_covers_exactly_langchain():
    assert set(DEPLOYABLE_SAMPLES) == {"langchain"}


def test_langchain_maps_to_customer_support_asserting_llm_only():
    s = DEPLOYABLE_SAMPLES["langchain"]
    assert isinstance(s, DeployableSample)
    assert s.app_path == "/samples/customer-support-agent"
    assert s.run_command == "python main.py"
    assert s.expected_kinds == ("llm",)


def test_langchain_declares_no_extra_env():
    assert DEPLOYABLE_SAMPLES["langchain"].env == ()
