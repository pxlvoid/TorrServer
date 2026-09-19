//go:build linux || darwin || freebsd

package torrstor

import "syscall"

// homelab: free and total bytes of the filesystem holding path (reached through hlDiskStat).
func hlDiskStatSys(path string) (free, total int64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, false
	}
	bsize := int64(st.Bsize)
	return int64(st.Bavail) * bsize, int64(st.Blocks) * bsize, true
}
