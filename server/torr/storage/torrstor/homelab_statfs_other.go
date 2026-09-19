//go:build !(linux || darwin || freebsd)

package torrstor

// homelab: disk stats are not implemented on this platform (reached through hlDiskStat) — only LimitGB applies.
func hlDiskStatSys(path string) (free, total int64, ok bool) {
	return 0, 0, false
}
