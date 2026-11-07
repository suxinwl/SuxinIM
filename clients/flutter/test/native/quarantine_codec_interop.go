// Standalone test program: uses the real shared Go codec and synthetic files
// only. Its temporary module is created by test-mobile-legacy-quarantine.ps1.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	localstore "puty.local/localstore"
)

func main() {
	if len(os.Args) != 2 { panic("synthetic root required") }
	root := os.Args[1]
	if !filepath.IsAbs(root) || len(filepath.Base(root)) < len("mobile-legacy-tests-") || filepath.Base(root)[:len("mobile-legacy-tests-")] != "mobile-legacy-tests-" { panic("synthetic root required") }
	key := bytes.Repeat([]byte{42}, 32)
	body := make([]byte, 1048593)
	for i := range body { body[i] = byte(i % 251) }
	source := "/synthetic/app_flutter/unknown.bin"
	sourceHash, payloadHash := sha256.Sum256([]byte(source)), sha256.Sum256(body)
	digest := hex.EncodeToString(payloadHash[:])
	id := hex.EncodeToString(sourceHash[:])+"-"+digest
	meta := struct { Source string `json:"source"`; Kind string `json:"kind"`; Account string `json:"account"`; Size int64 `json:"size"`; SHA256 string `json:"sha256"` }{source,"file","",int64(len(body)),digest}
	metadata, err := json.Marshal(meta); must(err)
	for _, item := range []struct{name, kind string; plain []byte}{{"data","legacy-file",body},{"meta","legacy-metadata",metadata}} {
		sealed, err := os.ReadFile(filepath.Join(root,"interop-java."+item.name)); must(err)
		var output bytes.Buffer
		must(localstore.Decrypt(&output,bytes.NewReader(sealed),key,localstore.ObjectIdentity{Account:"device-legacy",ObjectID:id,Type:item.kind}))
		if !bytes.Equal(output.Bytes(),item.plain) { panic("Java to Go synthetic body mismatch") }
		file, err := os.OpenFile(filepath.Join(root,"interop-go."+item.name),os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600); must(err)
		must(localstore.Encrypt(file,bytes.NewReader(item.plain),key,localstore.ObjectIdentity{Account:"device-legacy",ObjectID:id,Type:item.kind})); must(file.Close())
	}
	fmt.Println("PASS real_Java_to_Go_PUTYLS1_data_and_metadata")
}

func must(err error) { if err != nil { panic("synthetic codec interoperability failed") } }
