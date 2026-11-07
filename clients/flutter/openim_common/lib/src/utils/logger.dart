
/// print full log
class Logger {
  // Sample of abstract logging function
  static void print(dynamic text, {bool isError = false}) {
    // Legacy callers pass whole messages, tokens and signed URLs. They cannot
    // enter logcat/OS crash logs. Native vault diagnostics use fixed codes only.
  }
}
