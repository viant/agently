import json
import unittest
from presentation_fixture import report_text, feed_call

class PresentationFixtureTest(unittest.TestCase):
    def test_static_progressive_report(self):
        text = report_text()
        events = [json.loads(part.split('\n', 1)[1].split('\n```')[0]) for part in text.split('```') if part.startswith(('forge-data\n', 'forge-report\n'))]
        self.assertEqual([event['sequence'] for event in events], [1, 2, 3])
        self.assertEqual(events[-1]['mode'], 'commit')
        self.assertIn('Synthetic CTV', text)
        self.assertNotIn('http', text)
    def test_new_followup_uses_new_tool_call_but_current_result_does_not_repeat(self):
        tools = [{'function': {'name': 'presentation-fixture_feed'}}]
        messages = [{'role': 'user', 'content': 'fixture-feed'}, {'role': 'tool', 'content': '{}'}, {'role': 'user', 'content': 'fixture-feed-update'}]
        call = feed_call(messages, tools)
        self.assertEqual(json.loads(call['function']['arguments']), {'version': 2})
        self.assertIsNone(feed_call(messages + [{'role': 'tool', 'content': '{}'}], tools))
        self.assertIsNone(feed_call([{'role': 'user', 'content': 'finish this fixture'}], tools))

if __name__ == '__main__':
    unittest.main()
