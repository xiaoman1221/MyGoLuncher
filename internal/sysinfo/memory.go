package sysinfo

// RecommendedMemoryMB calculates a conservative automatic heap size from the
// machine's physical memory. It reserves memory for the OS and other apps,
// rounds to 512 MB and caps the result at 8 GB.
func RecommendedMemoryMB(totalMB int) int {
	if totalMB <= 0 {
		return 4096
	}
	reserve := 4096
	available := totalMB - reserve
	if available < 2048 {
		available = totalMB / 2
	}
	recommended := available / 2
	recommended = (recommended + 511) / 512 * 512
	if recommended < 2048 {
		recommended = 2048
	}
	if recommended > 8192 {
		recommended = 8192
	}
	// Never allocate more than half of total RAM on very small machines.
	if recommended > totalMB/2 && totalMB/2 >= 1024 {
		recommended = (totalMB / 2) / 512 * 512
		if recommended < 1024 {
			recommended = 1024
		}
	}
	return recommended
}
