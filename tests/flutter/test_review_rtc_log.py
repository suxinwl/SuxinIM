"""Regression for the caught failures missed by the original107 scan."""
import unittest

from review_rtc_log import classify, review


class ReviewRtcLogTests(unittest.TestCase):
    def test_caught_cleanup_is_failure_without_unhandled(self):
        kinds, info = classify("[OpenIMRtcCleanup] Track.stop/nativeTrack failed 12313ms PlatformException")
        self.assertEqual(kinds, ["caughtCleanupFailure"])
        self.assertEqual(info["elapsedMs"], 12313)

    def test_caught_renderer_retains_code_only(self):
        kinds, info = classify("RTCVideoRenderer source change failed: PlatformException(RTC_MEDIA_TIMEOUT, https://secret.invalid/?token=private)")
        self.assertEqual(kinds, ["caughtRendererFailure"])
        self.assertEqual(info["codes"], ["RTC_MEDIA_TIMEOUT"])
        self.assertNotIn("secret", str(info))

    def test_stage_name_is_not_failure(self):
        self.assertEqual(classify("[OpenIMRtcCleanup] Room.shutdown/retiredSocketFailure complete 0ms"), ([], {}))

    def test_success_is_not_failure(self):
        self.assertEqual(classify("job=2 operation=media phase=actionEnd elapsedMs=3 thread=worker"), ([], {}))

    def test_native_fields(self):
        self.assertEqual(classify("job=2 phase=actionEnd elapsedMs=3 failure=IllegalStateException")[0], ["nativePhysicalFailure"])
        self.assertEqual(classify("job=2 phase=callerDeadline elapsedMs=12000")[0], ["nativeCallerDeadline"])
        self.assertEqual(classify("deadline=5 operation=local.track phase=callerTimeout physicalPending=true")[0], ["nativeCallerDeadline"])
        self.assertEqual(classify("deadline=5 phase=physicalOutcome outcome=failure failure=Failure")[0], ["nativePhysicalFailure"])
        self.assertEqual(classify("operation=local.admission phase=blocked sequencePending=1")[0], ["nativeCaptureRejected"])
        self.assertEqual(classify("phase=quarantine reason=native_action_failure")[0], ["nativePermanentQuarantine"])
        self.assertEqual(classify("phase=callerError code=RTC_RENDERER_BUSY")[0], ["nativeCallerError"])
        self.assertEqual(classify("phase=rejected reason=owner_unavailable")[0], ["nativeAdmissionRejected"])
        self.assertEqual(classify("phase=attachRejected reason=owner_hard_failure")[0], ["nativeAdmissionRejected"])
        self.assertEqual(classify("phase=attachRetired reason=owner_closing"), ([], {}))

    def test_pid_separates_system_network_error(self):
        data = ("11-01 05:50:06.487 4810 4810 I flutter : [OpenIMRtcCleanup] Track.stop/nativeTrack failed 12313ms PlatformException\n"
                "11-01 05:50:06.488 1199 1199 E system : SocketException: unrelated\n").encode()
        report = review(data, 4810)
        self.assertEqual(report["eventCount"], 1)
        self.assertEqual(report["ignoredOtherPIDEvents"], 1)
        self.assertNotIn("socketExceptionForReview", report["categoryCounts"])

    def test_capture_guard_and_unhandled_both_recorded(self):
        kinds, _ = classify("Unhandled Exception: Unable to getUserMedia: Local media cleanup is still pending; retry after completion")
        self.assertEqual(kinds, ["unhandledException", "captureCleanupGuard"])

    def test_observed_capture_error_records_current_and_retired(self):
        for current in (True, False):
            kinds, info = classify("[OpenIMCallCapture] stage=control.cameraOn current="
                                   + str(current).lower() + " error=RTC_MEDIA_BUSY")
            self.assertEqual(kinds, ["caughtCaptureFailure"])
            self.assertEqual(info["currentCall"], current)
            self.assertEqual(info["safeError"], "RTC_MEDIA_BUSY")

    def test_observed_renderer_error_includes_retired_metadata(self):
        kinds, info = classify("[OpenIMRendererAttachment] stage=source current=false error=RTC_RENDERER_BUSY")
        self.assertEqual(kinds, ["caughtRendererAttachmentFailure"])
        self.assertFalse(info["currentSource"])
        self.assertEqual(info["safeError"], "RTC_RENDERER_BUSY")


if __name__ == "__main__":
    unittest.main()
