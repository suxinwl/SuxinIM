// Test-page observation only. Native constructor/methods and product media
// behavior are preserved; weak references do not own transport lifetimes.
export function installRtcTransportObserver() {
  if (window.collabRtcObserver || typeof window.RTCPeerConnection !== 'function') return;
  const NativePeer = window.RTCPeerConnection;
  const registry = window.collabRtcObserver = { nextID: 0, records: [], droppedRecords: 0 };
  if (typeof WeakRef !== 'function') { registry.unsupported = 'WeakRefUnavailable'; return; }
  window.RTCPeerConnection = new Proxy(NativePeer, {
    construct(target, args, newTarget) {
      const peer = Reflect.construct(target, args, newTarget);
      registry.records.push({ peerID: `pc_${++registry.nextID}`, weak: new WeakRef(peer), pending: null });
      if (registry.records.length > 32) { registry.records.shift(); registry.droppedRecords++; }
      return peer;
    },
  });
}

export function captureAudioState() {
  const registry = window.collabAudioElements ??= { nextID: 0, byElement: new WeakMap() };
  return [...document.querySelectorAll('audio')].map(audio => {
    let audioID = registry.byElement.get(audio);
    if (!audioID) { audioID = `audio_${++registry.nextID}`; registry.byElement.set(audio, audioID); }
    const media = audio.srcObject;
    const tracks = media && typeof media.getAudioTracks === 'function' ? media.getAudioTracks() : [];
    return {
      audioID, sourceKind: media ? 'mediaStream' : audio.currentSrc ? 'url' : 'empty',
      source: audio.dataset.lkSource || null, localParticipant: audio.dataset.lkLocalParticipant || null,
      currentTime: Number.isFinite(audio.currentTime) ? audio.currentTime : null,
      duration: Number.isFinite(audio.duration) ? audio.duration : null,
      readyState: audio.readyState, paused: audio.paused, ended: audio.ended,
      muted: audio.muted, volume: audio.volume, autoplay: audio.autoplay,
      networkState: audio.networkState, errorCode: audio.error?.code ?? null,
      decodedAudioBytes: Number.isFinite(audio.webkitAudioDecodedByteCount) ? audio.webkitAudioDecodedByteCount : null,
      audioTracks: tracks.map(track => ({ trackID: track.id, enabled: track.enabled, muted: track.muted, readyState: track.readyState })),
    };
  });
}

export async function captureRtcTransportState() {
  const registry = window.collabRtcObserver;
  if (!registry) return { available: false, reason: 'ObserverNotInstalled', peers: [] };
  if (registry.unsupported) return { available: false, reason: registry.unsupported, peers: [] };
  const fields = [
    'timestamp', 'bytesReceived', 'bytesSent', 'packetsReceived', 'packetsSent', 'packetsLost',
    'jitter', 'audioLevel', 'totalAudioEnergy', 'totalSamplesDuration', 'totalSamplesReceived',
    'concealedSamples', 'silentConcealedSamples', 'jitterBufferEmittedCount', 'lastPacketReceivedTimestamp',
    'framesReceived', 'framesDecoded', 'framesDropped', 'framesEncoded', 'framesSent',
    'keyFramesDecoded', 'frameWidth', 'frameHeight', 'framesPerSecond',
  ];
  const peers = [];
  for (const record of registry.records) {
    const peer = record.weak.deref();
    if (!peer) { peers.push({ peerID: record.peerID, collected: true }); continue; }
    const state = { peerID: record.peerID, connectionState: peer.connectionState, iceConnectionState: peer.iceConnectionState, signalingState: peer.signalingState };
    if (peer.signalingState === 'closed') { peers.push({ ...state, rtp: [] }); continue; }
    if (!record.pending) {
      // Share one actual getStats invocation until it physically settles;
      // overlapping /state requests and a caller deadline never spawn retries.
      record.pending = Promise.resolve().then(() => peer.getStats()).then(report => {
        const rows = [];
        report.forEach(stat => {
          const kind = stat.kind || stat.mediaType;
          if (!['inbound-rtp', 'outbound-rtp'].includes(stat.type) || !['audio', 'video'].includes(kind) || stat.isRemote) return;
          const source = stat.mediaSourceId ? report.get(stat.mediaSourceId) : null;
          const row = { id: stat.id, type: stat.type, kind, trackIdentifier: stat.trackIdentifier || source?.trackIdentifier || null };
          for (const field of fields) if (Number.isFinite(stat[field])) row[field] = stat[field];
          rows.push(row);
        });
        return { rtp: rows };
      }).catch(error => ({ statsErrorType: error?.name || 'Error', rtp: [] }));
      const current = record.pending;
      current.then(() => { if (record.pending === current) record.pending = null; });
    }
    let timer;
    try {
      const result = await Promise.race([record.pending, new Promise(resolve => { timer = setTimeout(() => resolve({ statsPending: true, rtp: [] }), 1500); })]);
      peers.push({ ...state, ...result });
    } finally { clearTimeout(timer); }
  }
  return { available: true, droppedRecords: registry.droppedRecords, peers,
    boundary: 'Actual native browser getStats counters, whitelisted RTP fields only. No ICE addresses, SDP, URLs, credentials or physical-speaker claim.' };
}
