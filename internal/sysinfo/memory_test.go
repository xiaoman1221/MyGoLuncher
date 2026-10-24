package sysinfo

import "testing"

func TestRecommendedMemoryMB(t *testing.T) {
	cases := map[int]int{
		0:     4096,
		4096:  2048,
		8192:  2048,
		16384: 6144,
		32768: 8192,
		65536: 8192,
	}
	for total, want := range cases {
		if got := RecommendedMemoryMB(total); got != want {
			t.Errorf("RecommendedMemoryMB(%d) = %d, want %d", total, got, want)
		}
	}
	if total := TotalMemoryMB(); total < 0 {
		t.Fatalf("invalid total memory: %d", total)
	}
}
