// Offline synthetic SQLCipher integration probe, never an account login.
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	sqlite3 "github.com/mattn/go-sqlite3"
)

func main() {
	directory,err:=os.MkdirTemp("","puty-cipher-probe-")
	if err!=nil {panic("cannot create synthetic probe directory")}
	defer os.RemoveAll(directory)
	key:=bytes.Repeat([]byte{0x41},32)
	register:=func(name string,key []byte){sql.Register(name,&sqlite3.SQLiteDriver{KeyHook:func(conn *sqlite3.SQLiteConn)error{return conn.SetRawKey(key)}})}
	register("puty-probe",key);register("puty-wrong",bytes.Repeat([]byte{0x42},32))
	file:=filepath.Join(directory,"synthetic.db")
	db,err:=sql.Open("puty-probe",file);if err!=nil {panic("open failed")};db.SetMaxOpenConns(1)
	var version string
	if err=db.QueryRow("PRAGMA cipher_version").Scan(&version);err!=nil || !strings.HasPrefix(version,"4.") {panic("actual SQLCipher runtime mismatch")}
	if _,err=db.Exec("CREATE TABLE synthetic(body TEXT); INSERT INTO synthetic VALUES('SYNTHETIC-PUTY-ONLY')");err!=nil {panic("synthetic write failed")}
	if err=db.Close();err!=nil {panic("close failed")}
	sealed,err:=os.ReadFile(file);if err!=nil {panic("read failed")}
	if bytes.HasPrefix(sealed,[]byte("SQLite format 3")) || bytes.Contains(sealed,[]byte("SYNTHETIC-PUTY-ONLY")) {panic("database persisted plaintext")}
	db,err=sql.Open("puty-wrong",file);if err!=nil {panic("wrong-key handle failed")};var count int
	if err=db.QueryRow("SELECT count(*) FROM synthetic").Scan(&count);err==nil {panic("wrong key accepted")};db.Close()
	db,err=sql.Open("puty-probe",file);if err!=nil {panic("reopen failed")}
	if err=db.QueryRow("SELECT count(*) FROM synthetic").Scan(&count);err!=nil || count!=1 {panic("correct-key restore failed")};db.Close()
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"passed":true,"cipherVersion":version,"persistedPlaintext":false,"wrongKeyRejected":true,"correctKeyReopen":true,"productionRequests":0})
}
