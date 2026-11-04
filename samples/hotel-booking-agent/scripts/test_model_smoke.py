"""Guard against reporting success without the real tool result."""
import json
import unittest
from model_smoke import completion


class ModelSmokeGuards(unittest.TestCase):
    def test_requires_tool_binding_and_correct_result(self):
        marker = 'hotel-smoke-' + 'a' * 32
        body = {'model': 'fixture', 'messages': [{'role': 'user', 'content': marker}],
                'tools': [{'type': 'function', 'function': {'name': 'search_hotels_tool'}}]}
        response, _, finish = completion(body)
        self.assertEqual(finish, 'tool_calls')
        call = response['choices'][0]['message']['tool_calls'][0]
        self.assertEqual(json.loads(call['function']['arguments']), {'destination': 'Tokyo'})
        tool_result = {'role': 'tool', 'tool_call_id': call['id'], 'content': '{"hotels": []}'}
        body['messages'].append(tool_result)
        with self.assertRaises(ValueError):
            completion(body)
        tool_result['content'] = '{"hotels": [{"hotel_id": "mock-002-tokyo"}]}'
        self.assertEqual(completion(body)[2], 'stop')
        tool_result['tool_call_id'] = 'unrelated'
        with self.assertRaises(ValueError):
            completion(body)

    def test_requires_model_tool_binding(self):
        with self.assertRaises(ValueError):
            completion({'model': 'fixture', 'messages': [{'role': 'user', 'content': 'hotel-smoke-' + 'a' * 32}]})


if __name__ == '__main__':
    unittest.main()
