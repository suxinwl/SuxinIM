import type { Participant } from "livekit-client";

import {
  sortParticipantsByAudioLevel,
  sortParticipantsByIsSpeaking,
  sortParticipantsByJoinedAt,
  sortParticipantsByLastSpokenAT,
} from "./base-sort-functions";

/**
 * Default sort for participants, it'll order participants by:
 * 1. dominant speaker (speaker with the loudest audio level)
 * 2. other speakers that are recently active
 * 3. participants with video on
 * 4. by joinedAt
 */
export function sortParticipants(participants: Participant[]): Participant[] {
  const sortedParticipants = [...participants];
  sortedParticipants.sort((a, b) => {
    // loudest speaker first
    if (a.isSpeaking && b.isSpeaking) {
      return sortParticipantsByAudioLevel(a, b);
    }

    // speaker goes first
    if (a.isSpeaking !== b.isSpeaking) {
      return sortParticipantsByIsSpeaking(a, b);
    }

    // last active speaker first
    if (a.lastSpokeAt !== b.lastSpokeAt) {
      return sortParticipantsByLastSpokenAT(a, b);
    }

    // video on
    const aVideo = a.isCameraEnabled;
    const bVideo = b.isCameraEnabled;
    if (aVideo !== bVideo) {
      if (aVideo) {
        return -1;
      }
      return 1;
    }

    // joinedAt
    return sortParticipantsByJoinedAt(a, b);
  });
  return sortedParticipants;
}
