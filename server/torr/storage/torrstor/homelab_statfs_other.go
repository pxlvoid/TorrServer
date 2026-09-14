//go:build !(linux || darwin || freebsd)

package torrstor

// homelab: disk stats are not implemented on this platform — only LimitGB applies.
func hlDiskStat(path string) (free, total int64, ok bool) {
	return 0, 0, false
}
