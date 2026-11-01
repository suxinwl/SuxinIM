"""Local fake-ADB guard tests; never run a device command or HTTP request."""
import importlib.util
from pathlib import Path
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('stopped_revoke', ROOT / 'tests/flutter/revoke_stopped_ui_session.py')
tool = importlib.util.module_from_spec(spec)
spec.loader.exec_module(tool)


class StoppedSessionAvdGuards(unittest.TestCase):
    def exercise(self, *, boot=tool.AVD, kernel=tool.AVD, console=tool.AVD + '\nOK\n',
                 pid='', package='User 0: installed=true stopped=true notLaunched=false', refused=False):
        calls = []
        def fake_adb(*args, **kwargs):
            calls.append(args)
            responses = {('get-state',): 'device\n',
                         ('shell', 'getprop', 'ro.kernel.qemu'): '1\n',
                         ('shell', 'getprop', 'ro.build.version.sdk'): '30\n',
                         ('shell', 'getprop', 'ro.boot.qemu.avd_name'): boot,
                         ('shell', 'getprop', 'ro.kernel.qemu.avd_name'): kernel,
                         ('emu', 'avd', 'name'): console,
                         ('shell', 'pidof', tool.PACKAGE): pid,
                         ('shell', 'dumpsys', 'package', tool.PACKAGE): package}
            return responses[args]
        with patch.object(tool, 'adb', side_effect=fake_adb), \
                patch.object(tool.urllib.request, 'build_opener') as network:
            if refused:
                with self.assertRaises(tool.Refusal):
                    tool.verify_stopped()
            else:
                tool.verify_stopped()
            network.assert_not_called()
        return calls

    def test_nonempty_exact_boot_also_checks_kernel_consistency(self):
        calls = self.exercise()
        self.assertIn(('shell', 'getprop', 'ro.kernel.qemu.avd_name'), calls)
        self.assertIn(('emu', 'avd', 'name'), calls)

    def test_empty_boot_requires_exact_kernel_and_console(self):
        calls = self.exercise(boot='\n')
        self.assertIn(('shell', 'getprop', 'ro.kernel.qemu.avd_name'), calls)
        self.assertIn(('emu', 'avd', 'name'), calls)

    def test_nonempty_wrong_boot_cannot_be_rescued_by_kernel(self):
        calls = self.exercise(boot='different-avd', refused=True)
        self.assertIn(('shell', 'getprop', 'ro.kernel.qemu.avd_name'), calls)

    def test_wrong_nonempty_kernel_cannot_be_rescued_by_boot(self):
        self.exercise(kernel='different-avd', refused=True)

    def test_empty_boot_wrong_or_empty_kernel_refused(self):
        for kernel in ('', 'different-avd'):
            self.exercise(boot='', kernel=kernel, refused=True)

    def test_console_mismatch_refused_with_valid_boot_or_fallback(self):
        for boot in (tool.AVD, ''):
            self.exercise(boot=boot, console='different-avd\nOK\n', refused=True)

    def test_empty_console_refused(self):
        self.exercise(console='', refused=True)

    def test_console_requires_exact_final_ok(self):
        for console in (tool.AVD, tool.AVD + '\nERROR\n', tool.AVD + '\nOK\nextra\n',
                        '\n' + tool.AVD + '\nOK\n', tool.AVD + '\n\nOK\n'):
            self.exercise(console=console, refused=True)

    def test_console_lf_crlf_and_actual_crcrlf_pass(self):
        for ending in ('\n', '\r\n', '\r\r\n'):
            self.exercise(console=tool.AVD + ending + 'OK' + ending)

    def test_console_real_empty_lines_still_refused(self):
        for console in ('\n' + tool.AVD + '\nOK\n', tool.AVD + '\n\nOK\n',
                        tool.AVD + '\nOK\n\n', tool.AVD + '\r\r\n\r\r\nOK\r\r\n'):
            self.exercise(console=console, refused=True)

    def test_console_spaces_and_wrong_names_still_refused(self):
        for console in (' ' + tool.AVD + '\nOK\n', tool.AVD + ' \nOK\n',
                        tool.AVD + '\n OK\n', tool.AVD + '\nOK \n', 'other-avd\nOK\n'):
            self.exercise(console=console, refused=True)

    def test_running_app_still_refused(self):
        self.exercise(boot='', pid='1234', refused=True)

    def test_package_not_stopped_still_refused(self):
        self.exercise(boot='', package='User 0: installed=true stopped=false', refused=True)


if __name__ == '__main__':
    unittest.main()
