package enterprise

import (
	"os"
	"time"
)

// These are deployment-owned read-only mountpoints, never request parameters.
// No database or object content is opened to measure filesystem capacity.
var capacityPaths = []struct{ kind, path string }{{"mongo", "/capacity/mongo"}, {"minio", "/capacity/minio"}, {"service_root", "/"}}

type capacitySample struct {
	fsType           string
	total, available uint64
	mountedReadOnly  bool
}
type filesystemCapacity struct {
	Kind         string   `json:"kind"`
	Path         string   `json:"path"`
	FSType       string   `json:"fsType"`
	Measured     bool     `json:"measured"`
	DataCapacity bool     `json:"dataCapacity"`
	Usage        *uint64  `json:"usage_bytes"`
	Available    *uint64  `json:"available_bytes"`
	Total        *uint64  `json:"total_bytes"`
	Ratio        *float64 `json:"usageRatio"`
	Status       string   `json:"status"`
}
type storageCapacity struct {
	DataCapacity bool                 `json:"dataCapacity"`
	Status       string               `json:"status"`
	MeasuredAt   int64                `json:"measuredAt"`
	Filesystems  []filesystemCapacity `json:"filesystems"`
}

func capacityState(ratio float64) string {
	if ratio >= .85 {
		return "critical"
	}
	if ratio >= .70 {
		return "warning"
	}
	return "ok"
}
func collectCapacity(sample func(string) (capacitySample, error), verified bool, now time.Time) storageCapacity {
	result := storageCapacity{MeasuredAt: now.Unix(), Status: "unknown", Filesystems: []filesystemCapacity{}}
	known := 0
	worst := 0
	for _, target := range capacityPaths {
		row := filesystemCapacity{Kind: target.kind, Path: target.path, Status: "unknown"}
		v, err := sample(target.path)
		if err == nil && v.total > 0 && v.available <= v.total {
			used := v.total - v.available
			ratio := float64(used) / float64(v.total)
			row.Measured = true
			row.FSType = v.fsType
			row.Total = &v.total
			row.Available = &v.available
			row.Usage = &used
			row.Ratio = &ratio
			row.Status = capacityState(ratio)
			row.DataCapacity = verified && target.kind != "service_root" && v.mountedReadOnly && v.fsType != "overlay" && v.fsType != "tmpfs" && v.fsType != "unknown"
			if row.DataCapacity {
				known++
				level := 0
				if row.Status == "warning" {
					level = 1
				}
				if row.Status == "critical" {
					level = 2
				}
				if level > worst {
					worst = level
				}
			}
		}
		result.Filesystems = append(result.Filesystems, row)
	}
	if known == 2 {
		result.DataCapacity = true
		result.Status = []string{"ok", "warning", "critical"}[worst]
	}
	return result
}
func (s *Server) storageCapacity() storageCapacity {
	// The operator may enable this only after read-only Docker Mounts/owned
	// volume verification. It is deliberately false on newly rendered stacks.
	return collectCapacity(sampleCapacity, os.Getenv("STORAGE_CAPACITY_DATA_VERIFIED") == "true", s.now())
}
