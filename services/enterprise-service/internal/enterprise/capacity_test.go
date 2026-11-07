package enterprise

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStorageCapacityMeasuresFixedPathsAndRequiresVerifiedDataMounts(t *testing.T) {
	seen := []string{}
	sample := func(path string) (capacitySample, error) {
		seen = append(seen, path)
		return capacitySample{fsType: "ext4", total: 1000, available: 150, mountedReadOnly: path != "/"}, nil
	}
	ref := collectCapacity(sample, false, time.Unix(1000, 0))
	if ref.DataCapacity || ref.Status != "unknown" || len(ref.Filesystems) != 3 {
		t.Fatal("unverified data disk claimed authoritative")
	}
	if len(seen) != 3 || seen[0] != "/capacity/mongo" || seen[1] != "/capacity/minio" || seen[2] != "/" {
		t.Fatal("sampler escaped fixed capacity paths")
	}
	data := collectCapacity(sample, true, time.Unix(1000, 0))
	if !data.DataCapacity || data.Status != "critical" || data.MeasuredAt != 1000 {
		t.Fatal("verified data filesystem alert missing")
	}
	if *data.Filesystems[0].Usage != 850 || *data.Filesystems[0].Available != 150 || *data.Filesystems[0].Total != 1000 || data.Filesystems[2].DataCapacity {
		t.Fatal("capacity values/root data classification wrong")
	}
	for _, available := range []uint64{301, 300, 151, 150} {
		r := collectCapacity(func(string) (capacitySample, error) {
			return capacitySample{fsType: "xfs", total: 1000, available: available, mountedReadOnly: true}, nil
		}, true, time.Now())
		want := "warning"
		if available == 301 {
			want = "ok"
		}
		if available == 150 {
			want = "critical"
		}
		if r.Status != want {
			t.Fatal("70/85 threshold changed", available, r.Status)
		}
	}
}

func TestStorageCapacityMissingOrUncertainFilesystemIsUnknownNotZero(t *testing.T) {
	for _, sample := range []func(string) (capacitySample, error){
		func(string) (capacitySample, error) {
			return capacitySample{}, errors.New("synthetic measurement failure")
		},
		func(string) (capacitySample, error) {
			return capacitySample{fsType: "ext4", total: 0, mountedReadOnly: true}, nil
		},
		func(string) (capacitySample, error) {
			return capacitySample{fsType: "ext4", total: 10, available: 11, mountedReadOnly: true}, nil
		},
	} {
		r := collectCapacity(sample, true, time.Now())
		if r.DataCapacity || r.Status != "unknown" {
			t.Fatal("missing metrics became data capacity")
		}
		for _, row := range r.Filesystems {
			if row.Measured || row.Total != nil || row.Usage != nil || row.Available != nil || row.Ratio != nil {
				t.Fatal("missing capacity reported zero")
			}
		}
	}
	for _, fstype := range []string{"overlay", "tmpfs", "unknown"} {
		r := collectCapacity(func(string) (capacitySample, error) {
			return capacitySample{fsType: fstype, total: 100, available: 10, mountedReadOnly: true}, nil
		}, true, time.Now())
		if r.DataCapacity || r.Status != "unknown" {
			t.Fatal("ephemeral filesystem became data capacity")
		}
	}
	r := collectCapacity(func(path string) (capacitySample, error) {
		return capacitySample{fsType: "ext4", total: 100, available: 10, mountedReadOnly: path == "/capacity/mongo"}, nil
	}, true, time.Now())
	if r.DataCapacity {
		t.Fatal("partially verified mount set became authoritative")
	}
}

func TestStorageCapacityIsReadOnlyManagementOnlyAndRejectsArbitraryPaths(t *testing.T) {
	s := &Server{Store: NewMemoryStore(), Auth: &testAuth{revoked: map[string]bool{}}}
	for _, row := range []struct {
		method, target, user string
		want                 int
	}{{"GET", "/enterprise/v1/storage/capacity", "alice", 403}, {"GET", "/enterprise/v1/storage/capacity", "admin", 200}, {"POST", "/enterprise/v1/storage/capacity", "admin", 400}, {"GET", "/enterprise/v1/storage/capacity?path=/etc", "admin", 400}} {
		r := httptest.NewRequest(row.method, row.target, nil)
		r.Header.Set("chatToken", row.user)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != row.want {
			t.Fatal("capacity route authorization/method/path guard changed", row.want, w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("capacity became cacheable")
		}
	}
	b := &HRMBridge{Server: s}
	if _, err := b.manage(t.Context(), "storage/capacity", asRequest(map[string]any{"path": "/etc"})); err == nil {
		t.Fatal("bridge accepted arbitrary filesystem path")
	}
	if _, err := b.manage(t.Context(), "storage/capacity", asRequest(map[string]any{})); err != nil {
		t.Fatal("capacity management bridge unavailable", err)
	}
}
