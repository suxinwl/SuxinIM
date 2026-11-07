//go:build android || ios

package file

import (
	"io"
	nativestore "puty.local/nativestore"
)

// Upload consumes encrypted native sources directly. No plaintext retry copy or
// temporary file is opened, and every Read/Seek rechecks the online lease.
func Open(req *UploadFileReq) (ReadFile, error) {
	reader, err := nativestore.OpenOutgoing(req.Filepath)
	if err != nil {
		return nil, err
	}
	return &encryptedUpload{reader}, nil
}

type encryptedUpload struct{ *nativestore.OutgoingReader }

func (e *encryptedUpload) StartSeek(offset int) error {
	_, err := e.Seek(int64(offset), io.SeekStart)
	return err
}
