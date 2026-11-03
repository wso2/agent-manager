import unittest

from model_stub import ModelServer


class Guards(unittest.TestCase):
    def test_missing_or_wrong_tool_output_cannot_pass(self):
        with ModelServer("dotnet-agent") as model:
            with self.assertRaises(AssertionError):
                model.completion(
                    {
                        "model": "test",
                        "messages": [{"role": "tool", "content": "wrong output"}],
                    }
                )
            self.assertFalse(model.verified)
            response = model.completion(
                {
                    "model": "test",
                    "messages": [{"role": "user", "content": "weather"}],
                    "tools": [{"function": {"name": "GetWeather"}}],
                }
            )
            self.assertEqual(response["choices"][0]["finish_reason"], "tool_calls")
            self.assertFalse(model.verified)


if __name__ == "__main__":
    unittest.main()
