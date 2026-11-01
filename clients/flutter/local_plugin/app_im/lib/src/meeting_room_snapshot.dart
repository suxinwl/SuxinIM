import 'meeting_info.dart';

/// Authoritative authenticated room envelope, separate from cached invitations.
class MeetingRoomSnapshot {
  MeetingRoomSnapshot(this.info, this.state, this.ended);

  final MeetingInfo info;
  final String state;
  final bool ended;

  factory MeetingRoomSnapshot.fromEnvelope(
    String expectedRoomID,
    Map<String, dynamic> data,
  ) {
    final raw = data['meetingInfo'];
    final state = data['state'];
    final ended = data['ended'];
    const terminal = {'ended', 'cancelled', 'timed_out', 'rejected'};
    const known = {'scheduled', 'active', ...terminal};
    if (raw is! Map ||
        state is! String ||
        ended is! bool ||
        !known.contains(state) ||
        ended != terminal.contains(state)) {
      throw const FormatException('Invalid authoritative meeting state');
    }
    final info = MeetingInfo.fromJson(Map<String, dynamic>.from(raw));
    if (info.roomID != expectedRoomID ||
        info.startTime == null ||
        info.endTime == null ||
        info.startTime! < 0 ||
        info.endTime! < 0) {
      throw const FormatException('Meeting identity or time is unavailable');
    }
    return MeetingRoomSnapshot(info, state, ended);
  }
}
