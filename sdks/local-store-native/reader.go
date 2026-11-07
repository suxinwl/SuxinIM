package localstore

import (
	"bytes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"sync"
)

type frameLocation struct {
	cipherOffset, plainOffset int64
	length                    uint32
}

// AuthenticatedReader exposes bounded random access only after every frame and
// the authenticated EOF have passed. Each later read reauthenticates its frame,
// so a modified backing file cannot turn previously checked bytes into output.
type AuthenticatedReader struct {
	mu             sync.Mutex
	source         io.ReaderAt
	header, prefix []byte
	cipher         cipher.AEAD
	frames         []frameLocation
	size, position int64
	cached         []byte
	cachedIndex    int
	closed         bool
	authorized     func() error
}

func OpenAuthenticatedReader(source io.ReaderAt, cipherSize int64, master []byte, id ObjectIdentity, authorized func() error) (*AuthenticatedReader, error) {
	if authorized == nil || authorized() != nil || cipherSize < 32 {
		return nil, ErrInvalidObject
	}
	var start [12]byte
	if _, err := source.ReadAt(start[:], 0); err != nil || !bytes.Equal(start[:8], magic) {
		return nil, ErrInvalidObject
	}
	length := binary.BigEndian.Uint32(start[8:])
	if length < 1 || length > maxHeader {
		return nil, ErrInvalidObject
	}
	header := make([]byte, length)
	if _, err := source.ReadAt(header, 12); err != nil {
		return nil, ErrInvalidObject
	}
	var meta Header
	decoder := json.NewDecoder(bytes.NewReader(header))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&meta) != nil {
		return nil, ErrInvalidObject
	}
	canonical, _ := json.Marshal(meta)
	if !bytes.Equal(canonical, header) || meta.ChunkSize != ChunkSize || meta.Format != Format || meta.Account != id.Account || meta.ObjectID != id.ObjectID || meta.Type != id.Type {
		return nil, ErrInvalidObject
	}
	prefix, err := base64.StdEncoding.Strict().DecodeString(meta.NoncePrefix)
	if err != nil || len(prefix) != 8 {
		return nil, ErrInvalidObject
	}
	aead, err := objectCipher(master, id)
	if err != nil {
		return nil, err
	}
	result := &AuthenticatedReader{source: source, header: header, prefix: prefix, cipher: aead, cachedIndex: -1, authorized: authorized}
	offset := int64(12 + length)
	for index := uint32(0); ; index++ {
		if err = authorized(); err != nil {
			return nil, err
		}
		var size [4]byte
		if _, err = source.ReadAt(size[:], offset); err != nil {
			return nil, ErrInvalidObject
		}
		n := binary.BigEndian.Uint32(size[:])
		if n > ChunkSize || offset+4+int64(n)+16 > cipherSize {
			return nil, ErrInvalidObject
		}
		sealed := make([]byte, int(n)+16)
		if _, err = source.ReadAt(sealed, offset+4); err != nil {
			return nil, ErrInvalidObject
		}
		nonce, aad := frameData(header, prefix, index, n)
		plain, err := aead.Open(nil, nonce, sealed, aad)
		if err != nil {
			return nil, ErrInvalidObject
		}
		clear(plain)
		offset += 4 + int64(n) + 16
		if n == 0 {
			if offset != cipherSize || authorized() != nil {
				return nil, ErrInvalidObject
			}
			return result, nil
		}
		if index == math.MaxUint32 || len(result.frames) >= 1048576 {
			return nil, ErrInvalidObject
		}
		result.frames = append(result.frames, frameLocation{offset - int64(n) - 16, result.size, n})
		result.size += int64(n)
	}
}

func (r *AuthenticatedReader) Size() int64 { r.mu.Lock(); defer r.mu.Unlock(); return r.size }

// ReadAt permits ZIP/PDF/media consumers to seek over encrypted bytes without
// constructing a plaintext temporary file or changing the sequential cursor.
func (r *AuthenticatedReader) ReadAt(output []byte, offset int64) (int, error) {
	r.mu.Lock()
	if r.closed || offset < 0 {
		r.mu.Unlock()
		return 0, ErrInvalidObject
	}
	clone := &AuthenticatedReader{source: r.source, header: bytes.Clone(r.header), prefix: bytes.Clone(r.prefix), cipher: r.cipher, frames: r.frames, size: r.size, position: offset, cachedIndex: -1}
	r.mu.Unlock()
	clone.authorized = func() error {
		r.mu.Lock()
		closed := r.closed
		r.mu.Unlock()
		if closed {
			return ErrInvalidObject
		}
		return r.authorized()
	}
	defer clone.Close()
	if offset > clone.size {
		return 0, io.EOF
	}
	n, err := io.ReadFull(clone, output)
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	return n, err
}
func (r *AuthenticatedReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	clear(r.cached)
	r.cached = nil
	r.cipher = nil
	clear(r.header)
	clear(r.prefix)
	return nil
}
func (r *AuthenticatedReader) Seek(offset int64, whence int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.authorized() != nil {
		return 0, ErrInvalidObject
	}
	switch whence {
	case io.SeekCurrent:
		offset += r.position
	case io.SeekEnd:
		offset += r.size
	case io.SeekStart:
	default:
		return 0, ErrInvalidObject
	}
	if offset < 0 || offset > r.size {
		return 0, ErrInvalidObject
	}
	r.position = offset
	return offset, nil
}
func (r *AuthenticatedReader) Read(output []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.authorized() != nil {
		return 0, ErrInvalidObject
	}
	if r.position == r.size {
		return 0, io.EOF
	}
	if len(output) == 0 {
		return 0, nil
	}
	index := int(r.position / ChunkSize)
	// The encoder normally emits full chunks. Derive a bounded index even for
	// authenticated compatible writers which emitted a shorter earlier frame.
	if index >= len(r.frames) || r.position < r.frames[index].plainOffset || r.position >= r.frames[index].plainOffset+int64(r.frames[index].length) {
		index = 0
		for index+1 < len(r.frames) && r.frames[index+1].plainOffset <= r.position {
			index++
		}
	}
	frame := r.frames[index]
	if r.cachedIndex != index {
		clear(r.cached)
		sealed := make([]byte, int(frame.length)+16)
		if _, err := r.source.ReadAt(sealed, frame.cipherOffset); err != nil {
			return 0, ErrInvalidObject
		}
		nonce, aad := frameData(r.header, r.prefix, uint32(index), frame.length)
		plain, err := r.cipher.Open(nil, nonce, sealed, aad)
		if err != nil {
			return 0, ErrInvalidObject
		}
		r.cached = plain
		r.cachedIndex = index
	}
	if r.authorized() != nil {
		clear(r.cached)
		r.cached = nil
		r.cachedIndex = -1
		return 0, ErrInvalidObject
	}
	n := copy(output, r.cached[r.position-frame.plainOffset:])
	r.position += int64(n)
	if r.authorized() != nil {
		clear(output[:n])
		clear(r.cached)
		r.cached = nil
		r.cachedIndex = -1
		return 0, ErrInvalidObject
	}
	return n, nil
}
