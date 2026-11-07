package enterprise

import (
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"openim38.local/identity"
)

// The pinned MinIO adapter uses query presigning for a single small PUT, but
// signs multipart PUTs with these exact headers. Never forward business bearer
// credentials, nor accept a new authentication mode on a read/avatar grant.
// This checks the narrow transport contract; MinIO still verifies the HMAC.
func attachmentStorageSignature(r *http.Request, grant identity.Attachment, now time.Time) (http.Header, error) {
	if !grant.Write || grant.Kind != "" || r.Method != http.MethodPut {
		return nil, nil
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, forbidden("invalid storage signature request")
	}
	_, hasUploadID := query["uploadId"]
	_, hasPartNumber := query["partNumber"]
	if !hasUploadID && !hasPartNumber {
		if strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-") {
			return nil, forbidden("invalid storage signature request")
		}
		return nil, nil
	}
	invalid := func() (http.Header, error) { return nil, forbidden("invalid storage signature request") }
	if !strings.HasPrefix(grant.Path, "/openim38/") || len(query) != 2 || len(query["uploadId"]) != 1 || len(query["partNumber"]) != 1 {
		return invalid()
	}
	uploadID, part := query.Get("uploadId"), query.Get("partNumber")
	partNumber, err := strconv.Atoi(part)
	if err != nil || partNumber < 1 || partNumber > 10000 || strconv.Itoa(partNumber) != part || len(uploadID) == 0 || len(uploadID) > 2048 {
		return invalid()
	}
	for _, ch := range uploadID {
		if ch < 0x21 || ch > 0x7e {
			return invalid()
		}
	}
	if len(r.Header.Values("Authorization")) != 1 || len(r.Header.Values("X-Amz-Date")) != 1 || len(r.Header.Values("X-Amz-Content-Sha256")) != 1 || len(r.Header.Values("X-Amz-Security-Token")) != 0 {
		return invalid()
	}
	stamp := r.Header.Get("X-Amz-Date")
	when, err := time.Parse("20060102T150405Z", stamp)
	if err != nil || when.Format("20060102T150405Z") != stamp || when.Before(now.Add(-15*time.Minute)) || when.After(now.Add(15*time.Minute)) || r.Header.Get("X-Amz-Content-Sha256") != "UNSIGNED-PAYLOAD" {
		return invalid()
	}
	accessKey := os.Getenv("OBJECT_STORAGE_ACCESS_KEY_ID")
	region := os.Getenv("OBJECT_STORAGE_REGION")
	if region == "" {
		region = "us-east-1"
	}
	if accessKey == "" {
		return nil, unavailable("object signing is unavailable")
	}
	auth := r.Header.Get("Authorization")
	fields := strings.Split(auth, ", ")
	credential := "AWS4-HMAC-SHA256 Credential=" + accessKey + "/" + stamp[:8] + "/" + region + "/s3/aws4_request"
	if len(fields) != 3 || fields[0] != credential || fields[1] != "SignedHeaders=host;x-amz-content-sha256;x-amz-date" || !strings.HasPrefix(fields[2], "Signature=") {
		return invalid()
	}
	signature := strings.TrimPrefix(fields[2], "Signature=")
	digest, err := hex.DecodeString(signature)
	if err != nil || len(digest) != 32 || hex.EncodeToString(digest) != signature {
		return invalid()
	}
	headers := http.Header{}
	headers.Set("Authorization", auth)
	headers.Set("X-Amz-Date", stamp)
	headers.Set("X-Amz-Content-Sha256", "UNSIGNED-PAYLOAD")
	return headers, nil
}
