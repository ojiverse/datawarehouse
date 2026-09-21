//go:build darwin

package spike

// maxRSSBytes converts ru_maxrss, which macOS reports in bytes.
func maxRSSBytes(maxrss int64) int64 { return maxrss }
