package localstore

import (
	"bytes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
)

// ChunkWriter leaves only complete authenticated frames on Abort. Its partial
// chunk exists in RAM. Only Finish writes authenticated EOF, so pending files
// cannot be mistaken for a completed object by the normal Decrypt function.
type ChunkWriter struct {
	writer                 io.Writer
	aead                   cipher.AEAD
	header, prefix, buffer []byte
	index                  uint32
	plain                  int64
	closed                 bool
}

func NewChunkWriter(writer io.Writer, master []byte, id ObjectIdentity) (*ChunkWriter, error) {
	aead, err := objectCipher(master, id)
	if err != nil {
		return nil, err
	}
	prefix := make([]byte, 8)
	if _, err = io.ReadFull(rand.Reader, prefix); err != nil {
		return nil, err
	}
	header, err := json.Marshal(Header{id.Account, ChunkSize, Format, base64.StdEncoding.EncodeToString(prefix), id.ObjectID, id.Type})
	if err != nil {
		return nil, err
	}
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(header)))
	for _, data := range [][]byte{magic, length[:], header} {
		if err = writeAll(writer, data); err != nil {
			return nil, err
		}
	}
	return &ChunkWriter{writer: writer, aead: aead, header: header, prefix: prefix, buffer: make([]byte, 0, ChunkSize)}, nil
}
func (w *ChunkWriter) frame(plain []byte) error {
	if w.index == math.MaxUint32 {
		return ErrInvalidObject
	}
	nonce, aad := frameData(w.header, w.prefix, w.index, uint32(len(plain)))
	sealed := w.aead.Seal(nil, nonce, plain, aad)
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(plain)))
	if err := writeAll(w.writer, length[:]); err != nil {
		return err
	}
	if err := writeAll(w.writer, sealed); err != nil {
		return err
	}
	w.index++
	w.plain += int64(len(plain))
	return nil
}
func (w *ChunkWriter) Write(data []byte) (int, error) {
	if w.closed {
		return 0, ErrInvalidObject
	}
	written := 0
	for len(data) > 0 {
		count := ChunkSize - len(w.buffer)
		if count > len(data) {
			count = len(data)
		}
		w.buffer = append(w.buffer, data[:count]...)
		written += count
		data = data[count:]
		if len(w.buffer) == ChunkSize {
			if err := w.frame(w.buffer); err != nil {
				return written, err
			}
			clear(w.buffer)
			w.buffer = w.buffer[:0]
		}
	}
	return written, nil
}
func (w *ChunkWriter) DurablePlaintextBytes() int64 { return w.plain }
func (w *ChunkWriter) TotalPlaintextBytes() int64   { return w.plain + int64(len(w.buffer)) }
func (w *ChunkWriter) Finish() error {
	if w.closed {
		return ErrInvalidObject
	}
	defer w.Abort()
	if len(w.buffer) > 0 {
		if err := w.frame(w.buffer); err != nil {
			return err
		}
	}
	return w.frame(nil)
}
func (w *ChunkWriter) Abort() {
	clear(w.buffer)
	w.buffer = nil
	w.aead = nil
	w.header = nil
	w.prefix = nil
	w.closed = true
}

// RewrapPending authenticates complete frames from an interrupted encrypted
// object and writes them under a FRESH random prefix. It never reuses the old
// key/nonce for a truncated frame. Caller fsync+renames the ciphertext temp file
// before issuing HTTP Range from returned offset. This function is never used
// for ordinary reads, where authenticated EOF is mandatory.
func RewrapPending(source io.Reader, destination *ChunkWriter, master []byte, id ObjectIdentity) (offset int64, complete bool, err error) {
	var start [12]byte
	n, readErr := io.ReadFull(source, start[:])
	if readErr == io.EOF && n == 0 {
		return 0, false, nil
	}
	if readErr != nil {
		return 0, false, ErrInvalidObject
	}
	if !bytes.Equal(start[:8], magic) {
		return 0, false, ErrInvalidObject
	}
	length := binary.BigEndian.Uint32(start[8:])
	if length == 0 || length > maxHeader {
		return 0, false, ErrInvalidObject
	}
	header := make([]byte, length)
	if _, err = io.ReadFull(source, header); err != nil {
		return 0, false, ErrInvalidObject
	}
	var meta Header
	decoder := json.NewDecoder(bytes.NewReader(header))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&meta); err != nil {
		return 0, false, ErrInvalidObject
	}
	canonical, _ := json.Marshal(meta)
	if !bytes.Equal(header, canonical) || meta.Account != id.Account || meta.Type != id.Type || meta.ObjectID != id.ObjectID || meta.Format != Format || meta.ChunkSize != ChunkSize {
		return 0, false, ErrInvalidObject
	}
	prefix, err := base64.StdEncoding.Strict().DecodeString(meta.NoncePrefix)
	if err != nil || len(prefix) != 8 {
		return 0, false, ErrInvalidObject
	}
	aead, err := objectCipher(master, id)
	if err != nil {
		return 0, false, err
	}
	for index := uint32(0); ; index++ {
		var lengthBytes [4]byte
		if _, err = io.ReadFull(source, lengthBytes[:]); err == io.EOF || err == io.ErrUnexpectedEOF {
			return offset, false, nil
		} else if err != nil {
			return 0, false, err
		}
		size := binary.BigEndian.Uint32(lengthBytes[:])
		if size > ChunkSize {
			return 0, false, ErrInvalidObject
		}
		sealed := make([]byte, int(size)+aead.Overhead())
		if _, err = io.ReadFull(source, sealed); err == io.EOF || err == io.ErrUnexpectedEOF {
			return offset, false, nil
		} else if err != nil {
			return 0, false, err
		}
		nonce, aad := frameData(header, prefix, index, size)
		plain, openErr := aead.Open(nil, nonce, sealed, aad)
		if openErr != nil {
			return 0, false, ErrInvalidObject
		}
		if size == 0 {
			var trailing [1]byte
			n, tailErr := source.Read(trailing[:])
			if n != 0 || tailErr != io.EOF {
				return 0, false, ErrInvalidObject
			}
			return offset, true, nil
		}
		err = destination.frame(plain)
		offset += int64(len(plain))
		clear(plain)
		if err != nil {
			return 0, false, err
		}
		if index == math.MaxUint32 {
			return 0, false, ErrInvalidObject
		}
	}
}
