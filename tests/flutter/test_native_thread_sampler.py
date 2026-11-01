"""Local fake-input tests only: no adb executable is invoked."""
import importlib.util
from pathlib import Path
import sys
import unittest
import contextlib
import io
import tempfile
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("owned_native_thread_sampler", Path(__file__).with_name("native_thread_sampler.py"))
sampler = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = sampler
spec.loader.exec_module(sampler)


def stat(pid, name="worker ) unusual", state="S", utime=13, stime=17, start=31):
    fields = [state]+["0"]*49
    fields[11], fields[12], fields[19] = str(utime), str(stime), str(start)
    return f"{pid} ({name}) " + " ".join(fields)


def batch(pid, start=31, wchan="0"):
    return (f"PIDOF\t{pid}\nCMDLINE\t{sampler.PACKAGE} \nUPTIME\t123.45 67.89\n"
            f"PSTAT\t{stat(pid, start=start)}\nCOMM\t9\tworker test\nSTAT\t9\t{stat(9)}\nWCHAN\t9\t{wchan}\n"
            f"PIDOF_END\t{pid}\nCMDLINE_END\t{sampler.PACKAGE} \nPSTAT_END\t{stat(pid, start=start)}\nEND\n")


class LocalOnlyTests(unittest.TestCase):
    def test_stat_comm_parentheses_and_exact_tick_offsets(self):
        value = sampler.parse_stat(stat(9), 9)
        self.assertEqual((value["statComm"], value["state"], value["utimeTicks"], value["stimeTicks"], value["starttimeTicks"]),
                         ("worker ) unusual", "S", 13, 17, 31))
        with self.assertRaises(ValueError):
            sampler.parse_stat(stat(9), 8)

    def test_wchan_zero_is_unknown_and_partial_thread_retained(self):
        value = sampler.parse_batch(batch(5)+"COMM\t10\texiting\nSTAT\t10\t\nWCHAN\t10\t\n", 5)
        self.assertFalse(value["threads"][0]["wchanKnown"])
        self.assertIn("unknown", value["threads"][0]["wchanMeaning"])
        self.assertTrue(value["threads"][1]["statUnavailable"])
        self.assertEqual(value["deviceUptimeSecondsRaw"], "123.45")

    def test_numeric_pid_label_and_bound_guards(self):
        for value in ["5 6", "0", "5; id", "", "-2"]:
            with self.assertRaises(ValueError):
                sampler.parse_pid(value)
        for config in [sampler.Config("../bad"), sampler.Config("x", 181), sampler.Config("x", interval=.5)]:
            with self.assertRaises(ValueError):
                config.validate()
        shell = sampler.batch_script(5)
        for forbidden in ["kill", "SIGQUIT", "debuggerd", "/stack", "logcat", "run-as", "cat /data/"]:
            self.assertNotIn(forbidden, shell)
        self.assertIn("/proc/5/task/*", shell)

    def fake_capture(self, responses):
        now = [0.0]
        calls = []
        def runner(args, timeout):
            calls.append(args)
            self.assertGreater(timeout, 0)
            now[0] += .01
            return responses.pop(0)
        report = sampler.capture(sampler.Config("local-test", 5, 1), runner, lambda: now[0], lambda n: now.__setitem__(0, now[0]+n))
        return report, calls

    def test_wrong_avd_refused_before_pid_or_task_reads(self):
        report, calls = self.fake_capture(["some-other-avd\nOK\n"])
        self.assertEqual(report["stopReason"], "refused_wrong_avd")
        self.assertEqual(calls, [["emu", "avd", "name"]])

    def test_single_batch_per_sample_and_pid_change_stops(self):
        report, calls = self.fake_capture([sampler.AVD+"\nOK\n", "5\n", batch(5), "PIDOF\t6\nSTOP\tpid_changed\n"])
        self.assertEqual(report["sampleCount"], 1)
        self.assertEqual(report["fixedPID"], 5)
        self.assertEqual(report["stopReason"], "pid_or_package_identity_changed_or_unreadable")
        self.assertEqual(len(calls), 4)
        self.assertEqual(calls[2][0], "shell")
        self.assertEqual(calls[2], calls[3])

    def test_same_pid_reused_or_other_package_stops(self):
        report, _ = self.fake_capture([sampler.AVD+"\nOK\n", "5\n", batch(5), batch(5, start=32)])
        self.assertEqual(report["sampleCount"], 1)
        self.assertEqual(report["stopReason"], "pid_reused_with_new_process_starttime")
        report, _ = self.fake_capture([sampler.AVD+"\nOK\n", "5\n", batch(5).replace(sampler.PACKAGE, "other.package")])
        self.assertEqual(report["sampleCount"], 0)
        self.assertEqual(report["stopReason"], "pid_or_package_identity_changed_or_unreadable")
        reused = batch(5).replace("PSTAT_END\t"+stat(5), "PSTAT_END\t"+stat(5, start=999))
        report, _ = self.fake_capture([sampler.AVD+"\nOK\n", "5\n", reused])
        self.assertEqual(report["sampleCount"], 0)
        self.assertEqual(report["stopReason"], "pid_reused_during_batch")

    def test_fake_clock_enforces_total_window_without_extra_adb_call(self):
        report, calls = self.fake_capture([sampler.AVD+"\nOK\n", "5\n"]+[batch(5)]*5)
        self.assertEqual(report["sampleCount"], 5)
        self.assertEqual(len(calls), 7)
        self.assertEqual(report["stopReason"], "bounded_window_completed")
        self.assertEqual(report["hostElapsedSeconds"], 5)
        self.assertFalse(report["CPUPercentCalculated"])

    def test_cli_requires_optin_and_rejects_existing_label_before_capture(self):
        with patch.object(sampler, "capture", side_effect=AssertionError("Must not touch adb")):
            with patch.object(sys, "argv", ["sampler", "--label", "newlabel"]), contextlib.redirect_stderr(io.StringIO()):
                with self.assertRaises(SystemExit):
                    sampler.main()
            with tempfile.TemporaryDirectory() as folder, patch.object(sampler, "ROOT", Path(folder)):
                output = Path(folder)/"tests/flutter/diagnostics/native-thread-samples/existing.json"
                output.parent.mkdir(parents=True)
                output.write_text("preserved", encoding="utf-8")
                with patch.object(sys, "argv", ["sampler", "--label", "existing", "--start-owned-avd"]):
                    with self.assertRaises(FileExistsError):
                        sampler.main()
                self.assertEqual(output.read_text(encoding="utf-8"), "preserved")


if __name__ == "__main__":
    unittest.main(verbosity=2)
