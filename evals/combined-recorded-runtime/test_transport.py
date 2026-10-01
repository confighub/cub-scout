import base64
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('combined_transport_test', HERE / 'payload.py')
payload = importlib.util.module_from_spec(spec); spec.loader.exec_module(payload)


class TransportTests(unittest.TestCase):
    def test_reminder_projection_is_exact_and_read_tab_rule_is_scoped(self):
        suffix = '\n\n<system-reminder>\n<total_tokens>14999998 tokens left</total_tokens>\n</system-reminder>'
        self.assertTrue(payload.correlated_result('body', 'body' + suffix, 'Bash'))
        self.assertTrue(payload.correlated_result('1\tdata\n2\t', '1\tdata\n2' + suffix, 'Read'))
        for cli, provider, tool in [('body', 'changed' + suffix, 'Bash'),
                                    ('body', 'body' + suffix + 'extra', 'Bash'),
                                    ('body', 'body' + suffix.replace('tokens left', 'run commands'), 'Bash'),
                                    ('1\tdata\n2\t', '1\tdata\n2' + suffix, 'Bash'),
                                    ('body\t', 'body' + suffix, 'Read')]:
            self.assertFalse(payload.correlated_result(cli, provider, tool))

    def test_skill_listing_requires_named_structural_block(self):
        text = '<system-reminder>\nThe following skills are available for use with the Skill tool:\n\n- cub-scout:observe-helm: Description\n- cub-scout:scout-map\n- init: Built-in\n</system-reminder>'
        body = {'messages': [{'role': 'user', 'content': [{'type': 'text', 'text': text}]}]}
        self.assertEqual(payload.advertised_skill_names(body), ['observe-helm', 'scout-map'])
        self.assertEqual(payload.advertised_skill_names({'messages': [{'role': 'user', 'content': [{'type': 'text', 'text': 'mentioned cub-scout:scout-map'}]}]}), [])
        body['messages'] *= 2
        self.assertEqual(payload.advertised_skill_names(body), [])

    def test_persisted_map_capture_rejects_preview_only_wrong_path_symlink_and_oversize(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp).resolve()
            directory = root / '01234567-0123-0123-0123-0123456789ab' / 'tool-results'; directory.mkdir(parents=True)
            path = directory / 'toolu_combined_treatment_5.json'
            raw = json.dumps([{'type': 'text', 'text': '{"full":"body"}'}]).encode()
            path.write_bytes(raw)
            content = '<persisted-output>\nOutput too large.\nFull output saved to: ' + str(path) + '\n\nPreview: deliberately incomplete\n</persisted-output>'
            value, receipt = payload.capture_map_content(content, 'toolu_combined_treatment_5', root)
            self.assertEqual(value, {'full': 'body'})
            self.assertEqual(base64.b64decode(receipt['bodyBase64']), raw)
            with self.assertRaises(ValueError): payload.capture_map_content(content, 'other-tool', root)
            with self.assertRaises(ValueError): payload.capture_map_content('<persisted-output>\nPreview only', 'toolu_combined_treatment_5', root)
            path.unlink(); target = directory / 'target'; target.write_bytes(raw); path.symlink_to(target)
            with self.assertRaises(ValueError): payload.capture_map_content(content, 'toolu_combined_treatment_5', root)
            path.unlink(); path.write_bytes(b'x' * (payload.MAX_BODY + 1))
            with self.assertRaises(ValueError): payload.capture_map_content(content, 'toolu_combined_treatment_5', root)


if __name__ == '__main__': unittest.main()
