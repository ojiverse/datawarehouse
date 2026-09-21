//go:build linux

package spike

// maxRSSBytes converts ru_maxrss, which Linux reports in kilobytes.
func maxRSSBytes(maxrss int64) int64 { return maxrss * 1024 }
