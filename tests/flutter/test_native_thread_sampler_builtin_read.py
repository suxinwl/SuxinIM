"""Offline parser/static-contract tests; no shell, ADB or device is executed."""
import ast
from pathlib import Path
import subprocess
import unittest

from test_native_thread_sampler import batch, sampler, stat


class BuiltinReadCandidateTests(unittest.TestCase):
    def test_per_thread_body_has_only_builtin_reads_and_protocol_printf(self):
        shell = sampler.batch_script(7093)
        loop = shell.split("for task in /proc/7093/task/*; do\n", 1)[1].split("\ndone\n", 1)[0]
        self.assertNotIn("$(", loop)
        self.assertNotIn("`", loop)
        for forbidden in ["cat ", "tr ", "sed ", "awk ", "xargs ", "eval ", " | ", "kill", "sleep"]:
            self.assertNotIn(forbidden, loop)
        for field in ["comm", "stat", "wchan"]:
            self.assertEqual(loop.count('read_proc "$task/' + field + '"'), 1)
        for prefix in ["COMM", "STAT", "WCHAN"]:
            self.assertIn("printf '" + prefix + "\\t%s\\t%s\\n'", loop)

    def test_fixed_per_batch_subshells_and_full_cmdline_guards_retained(self):
        shell = sampler.batch_script(7093)
        active = "\n".join(line for line in shell.splitlines() if not line.lstrip().startswith("#"))
        self.assertEqual(active.count("$("), 4)  # TAB, CR, and two existing pidof.
        self.assertEqual(shell.count("$(printf"), 2)
        self.assertEqual(shell.count("$(pidof " + sampler.PACKAGE), 2)
        self.assertEqual(shell.count("tr '\\000\\n\\t' '   ' < /proc/7093/cmdline"), 2)
        self.assertEqual(shell.count('read_proc /proc/7093/stat'), 2)
        self.assertNotIn("cat ", shell)

    def test_global_read_value_no_command_substitution_and_partial_final_line(self):
        shell = sampler.batch_script(7093)
        function = shell.split("read_proc() {\n", 1)[1].split('\n}\ncurrent=', 1)[0]
        self.assertIn('while IFS= read -r proc_line || [ -n "$proc_line" ]; do', function)
        self.assertIn('done 2>/dev/null < "$1"', function)
        self.assertNotIn("$(", function)
        self.assertNotIn("`", function)
        self.assertIn('proc_value="${proc_value%%"$TAB"*} ${proc_value#*"$TAB"}"', function)
        self.assertIn('proc_value="${proc_value%%"$CR"*} ${proc_value#*"$CR"}"', function)
        self.assertIn('proc_value="$proc_value$proc_line"', function)

    def test_non_batch_identity_capture_and_parser_code_is_unchanged(self):
        root = Path(__file__).parent
        old = ast.parse((root / 'diagnostics/112/native-thread-sampler-builtin-adoption/before/native_thread_sampler.py').read_text(encoding='utf-8'))
        new = ast.parse((root / 'native_thread_sampler.py').read_text(encoding='utf-8'))
        def selected(tree):
            return {node.name: ast.dump(node, include_attributes=False)
                    for node in tree.body if isinstance(node, (ast.FunctionDef, ast.ClassDef))
                    and node.name != 'batch_script'}
        self.assertEqual(selected(old), selected(new))
        self.assertEqual(sampler.ROOT, root.resolve().parents[1])
        for value in ['1;id', '../2', 0, -1, '7093']:
            with self.assertRaises(ValueError):
                sampler.batch_script(value)

    def test_normalized_protocol_preserves_spaces_and_right_parentheses(self):
        text = batch(7093).replace('COMM\t9\tworker test', 'COMM\t9\t  left ) mid  tail ')
        text = text.replace('STAT\t9\t' + stat(9), 'STAT\t9\t' + stat(9, '  left ) mid  tail '))
        value = sampler.parse_batch(text, 7093)
        self.assertEqual(value['parseErrors'], [])
        thread = value['threads'][0]
        self.assertEqual(thread['comm'], '  left ) mid  tail')
        self.assertEqual(thread['statComm'], '  left ) mid  tail ')
        self.assertEqual((thread['utimeTicks'], thread['stimeTicks'], thread['starttimeTicks']), (13, 17, 31))

    def test_normalized_cr_tab_embedded_line_fixture_is_a_single_protocol_row(self):
        # Expected protocol AFTER the unexecuted shell normalizer, not a test of Android sh.
        normalized_name = 'one two three four ) end'
        text = batch(7093).replace('COMM\t9\tworker test', 'COMM\t9\t' + normalized_name)
        text = text.replace('STAT\t9\t' + stat(9), 'STAT\t9\t' + stat(9, normalized_name))
        result = sampler.parse_batch(text.replace('\n', '\r\n'), 7093)
        self.assertEqual(result['parseErrors'], [])
        self.assertEqual(result['threads'][0]['comm'], normalized_name)
        self.assertEqual(result['threads'][0]['statComm'], normalized_name)

    def test_exiting_thread_still_has_unavailable_fields(self):
        text = batch(7093).replace('END\n', 'COMM\t10\t\nSTAT\t10\t\nWCHAN\t10\t\nEND\n')
        result = sampler.parse_batch(text, 7093)
        row = result['threads'][1]
        self.assertEqual(row['tid'], 10)
        self.assertTrue(row['statUnavailable'])
        self.assertEqual(row['comm'], '')
        self.assertFalse(row['wchanKnown'])
        self.assertEqual(row['wchan'], '')

    def test_unterminated_wchan_expected_protocol_and_unknown_zero(self):
        known = sampler.parse_batch(batch(7093, wchan='futex_wait_queue_me'), 7093)['threads'][0]
        self.assertEqual(known['wchan'], 'futex_wait_queue_me')
        self.assertTrue(known['wchanKnown'])
        unknown = sampler.parse_batch(batch(7093), 7093)['threads'][0]
        self.assertFalse(unknown['wchanKnown'])

    def test_full_cmdline_extra_argument_and_end_starttime_refused(self):
        for bad in [batch(7093).replace(sampler.PACKAGE + ' ', sampler.PACKAGE + ' unexpected '),
                    batch(7093).replace('PSTAT_END\t' + stat(7093), 'PSTAT_END\t' + stat(7093, start=99))]:
            responses = [sampler.AVD + '\nOK\n', '7093\n', bad]
            now = [0.0]
            def runner(args, timeout):
                now[0] += .01
                return responses.pop(0)
            result = sampler.capture(sampler.Config('fake-identity', 180, 2), runner,
                                     lambda: now[0], lambda n: now.__setitem__(0, now[0] + n))
            self.assertEqual(result['sampleCount'], 0)
            self.assertIn(result['stopReason'], ['pid_or_package_identity_changed_or_unreadable',
                                                'pid_reused_during_batch'])

    def test_global180_and_per_read15_timeout_remain_bounded(self):
        calls = []
        responses = [sampler.AVD + '\nOK\n', '7093\n']
        now = [0.0]
        def runner(args, timeout):
            calls.append((args, timeout))
            now[0] += .01
            if responses:
                return responses.pop(0)
            raise subprocess.TimeoutExpired('offline-fake-only', timeout)
        result = sampler.capture(sampler.Config('fake-timeout', 180, 2), runner,
                                 lambda: now[0], lambda n: now.__setitem__(0, now[0] + n))
        self.assertEqual([v[1] for v in calls], [15, 15, 15])
        self.assertEqual(result['durationLimitSeconds'], 180)
        self.assertEqual(result['sampleCount'], 0)
        self.assertEqual(result['errorType'], 'TimeoutExpired')
        self.assertEqual(result['stopReason'], 'read_or_identity_failure')


if __name__ == '__main__':
    unittest.main(verbosity=2)
