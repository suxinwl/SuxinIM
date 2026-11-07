package localstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

var ErrInvalidName = errors.New("source filename cannot be represented safely")

// ValidateSourceName preserves legal source names byte-for-byte. It never
// changes Unicode compatibility forms or replaces the name with an object ID.
func ValidateSourceName(name string) error {
	if !utf8.ValidString(name) {
		return errors.New("filename must be valid UTF-8")
	}
	if name == "" || name == "." || name == ".." || strings.TrimRight(name, ". ") != name || len(name) > 255 {
		return ErrInvalidName
	}
	for _, r := range name {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return ErrInvalidName
		}
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
		return ErrInvalidName
	}
	return nil
}

func CollisionName(source string, number uint32) (string, error) {
	if err := ValidateSourceName(source); err != nil {
		return "", err
	}
	if number == 0 {
		return source, nil
	}
	ext := filepath.Ext(source)
	if strings.HasPrefix(source, ".") && strings.Count(source, ".") == 1 {
		ext = ""
	}
	name := fmt.Sprintf("%s（%d）%s", strings.TrimSuffix(source, ext), number, ext)
	if err := ValidateSourceName(name); err != nil {
		return "", err
	}
	return name, nil
}

// Reservation owns an exclusively-created ciphertext file. The storage index
// must persist taskID -> Name and PENDING before exposing or resuming it.
type Reservation struct {
	File     *os.File
	Name     string
	path     string
	identity os.FileInfo
}

func ReserveName(directory, source string) (*Reservation, error) {
	if err := ValidateSourceName(source); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(directory) {
		return nil, ErrInvalidName
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	for number := uint32(0); number < 100000; number++ {
		name, err := CollisionName(source, number)
		if err != nil {
			return nil, err
		}
		p := filepath.Join(directory, name)
		file, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		identity, err := file.Stat()
		if err != nil {
			file.Close()
			return nil, err
		}
		return &Reservation{file, name, p, identity}, nil
	}
	return nil, errors.New("attachment name allocation exhausted")
}

func (r *Reservation) Commit() error {
	if err := r.File.Sync(); err != nil {
		return err
	}
	return r.File.Close()
}

func (r *Reservation) Abort() error {
	_ = r.File.Close()
	current, err := os.Lstat(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !os.SameFile(r.identity, current) {
		return errors.New("reservation ownership changed")
	}
	return os.Remove(r.path)
}
