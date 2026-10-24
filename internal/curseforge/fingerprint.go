package curseforge

// Fingerprint computes the CurseForge file fingerprint. CurseForge uses
// MurmurHash2 with seed 1 after removing whitespace bytes (tab, LF, CR, space).
func Fingerprint(data []byte) uint32 {
	filtered := make([]byte, 0, len(data))
	for _, value := range data {
		switch value {
		case 9, 10, 13, 32:
			continue
		default:
			filtered = append(filtered, value)
		}
	}
	return murmurHash2(filtered, 1)
}

func murmurHash2(data []byte, seed uint32) uint32 {
	const m uint32 = 0x5bd1e995
	const r = 24
	length := len(data)
	hash := seed ^ uint32(length)
	for len(data) >= 4 {
		k := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
		k *= m
		k ^= k >> r
		k *= m
		hash *= m
		hash ^= k
		data = data[4:]
	}
	switch len(data) {
	case 3:
		hash ^= uint32(data[2]) << 16
		fallthrough
	case 2:
		hash ^= uint32(data[1]) << 8
		fallthrough
	case 1:
		hash ^= uint32(data[0])
		hash *= m
	}
	hash ^= hash >> 13
	hash *= m
	hash ^= hash >> 15
	return hash
}
