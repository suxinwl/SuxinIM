//go:build linux

package enterprise

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
)

func sampleCapacity(path string) (capacitySample, error) {
	var v capacitySample
	meta, err := os.Lstat(path)
	if err != nil || !meta.IsDir() || meta.Mode()&os.ModeSymlink != 0 {
		return v, errors.New("capacity path unavailable")
	}
	var st syscall.Statfs_t
	if err = syscall.Statfs(path, &st); err != nil || st.Bsize <= 0 || st.Bavail > st.Blocks {
		return v, errors.New("capacity measurement unavailable")
	}
	block := uint64(st.Bsize)
	if st.Blocks > ^uint64(0)/block {
		return v, errors.New("capacity measurement outside supported range")
	}
	v.total = st.Blocks * block
	v.available = st.Bavail * block
	switch uint64(st.Type) {
	case 0xEF53:
		v.fsType = "ext4"
	case 0x58465342:
		v.fsType = "xfs"
	case 0x9123683E:
		v.fsType = "btrfs"
	case 0x794c7630:
		v.fsType = "overlay"
	case 0x01021994:
		v.fsType = "tmpfs"
	default:
		v.fsType = fmt.Sprintf("0x%x", uint64(st.Type))
	}
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return v, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 7 || fields[4] != path {
			continue
		}
		for _, option := range strings.Split(fields[5], ",") {
			if option == "ro" {
				v.mountedReadOnly = true
			}
		}
	}
	if scanner.Err() != nil {
		return capacitySample{}, errors.New("capacity mount verification unavailable")
	}
	return v, nil
}
