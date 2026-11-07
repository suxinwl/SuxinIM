//go:build puty_sqlcipher && (android || ios)

package sqlite3

/*
#cgo CFLAGS: -DUSE_LIBSQLITE3 -DSQLITE_HAS_CODEC
#cgo android,arm64 LDFLAGS: -L${SRCDIR}/../sqlcipher-android-4.19.1/jni/arm64-v8a -lsqlcipher
#cgo android,amd64 LDFLAGS: -L${SRCDIR}/../sqlcipher-android-4.19.1/jni/x86_64 -lsqlcipher
#include <sqlite3.h>
int sqlite3_key(sqlite3 *db, const void *pKey, int nKey);
int sqlite3_key_v2(sqlite3 *db, const char *zDbName, const void *pKey, int nKey);
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"unsafe"
)

// SetRawKey keys a SQLCipher handle before its first query. It deliberately
// avoids DSN/query-string keys and PRAGMA SQL containing secret material.
func (c *SQLiteConn) SetRawKey(key []byte) error {
	return c.SetAttachedRawKey("main", key)
}

func (c *SQLiteConn) SetAttachedRawKey(schema string, key []byte) error {
	if len(key) != 32 {
		return errors.New("SQLCipher requires a 256-bit native key")
	}
	// SQLCipher's raw-key syntax is used only in the C API, never SQL or a DSN.
	const digits = "0123456789abcdef"
	encoded := make([]byte, 67)
	encoded[0], encoded[1], encoded[66] = 'x', '\'', '\''
	for i, value := range key {
		encoded[2+i*2] = digits[value>>4]
		encoded[3+i*2] = digits[value&15]
	}
	defer func() {
		for i := range encoded {
			encoded[i] = 0
		}
	}()
	if schema != "main" && schema != "protected" {
		return errors.New("invalid native cipher schema")
	}
	name := C.CString(schema)
	defer C.free(unsafe.Pointer(name))
	if result := C.sqlite3_key_v2(c.db, name, unsafe.Pointer(&encoded[0]), C.int(len(encoded))); result != C.SQLITE_OK {
		return errors.New("SQLCipher key configuration failed")
	}
	return nil
}
