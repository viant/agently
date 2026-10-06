"""Static native renderer fixtures; no URLs, secrets or business datasource calls."""
import json

def report_text():
    blocks = [
        {'id': 'fixture-table', 'kind': 'dashboard.table', 'title': 'Synthetic channel delivery', 'dataSourceRef': 'delivery', 'columns': [{'key': 'channel', 'label': 'Channel'}, {'key': 'spend', 'label': 'Spend'}]},
    ]
    events = [
        ('forge-data', {'version': 2, 'reportRef': 'fixture-report', 'id': 'delivery', 'sequence': 1, 'data': [{'channel': 'Synthetic CTV', 'spend': 12}, {'channel': 'Synthetic Display', 'spend': 7}]}),
        ('forge-report', {'version': 1, 'id': 'fixture-report', 'sequence': 2, 'mode': 'start', 'title': 'AG-UI synthetic report', 'blocks': blocks}),
        ('forge-report', {'version': 1, 'id': 'fixture-report', 'sequence': 3, 'mode': 'commit'}),
    ]
    return 'Deterministic presentation fixture.\n' + '\n'.join('```' + kind + '\n' + json.dumps(value) + '\n```' for kind, value in events) + '\nSynthetic report complete.'

def feed_call(messages, tools):
    users = [index for index, message in enumerate(messages) if message.get('role') == 'user']
    last = users[-1] if users else -1
    prompt = str(messages[last].get('content', '')) if last >= 0 else ''
    selected = next((tool for tool in tools if 'fixture_feed' in tool['function']['name']), None)
    if 'fixture-feed' not in prompt or selected is None or any(message.get('role') == 'tool' for message in messages[last + 1:]):
        return None
    return {'index': 0, 'id': 'fixture-feed-call', 'type': 'function', 'function': {'name': selected['function']['name'], 'arguments': json.dumps({'version': 2 if 'fixture-feed-update' in prompt else 1})}}
