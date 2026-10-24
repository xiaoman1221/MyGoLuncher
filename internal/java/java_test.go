package java

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMajorFromVersion(t *testing.T) {
	cases := map[string]int{
		"1.8.0_511": 8,
		"8.0.402":   8,
		"17.0.10":   17,
		"21-ea":     21,
		"25.0.2+9":  25,
		"v26.0.1":   26,
	}
	for input, want := range cases {
		if got := MajorFromVersion(input); got != want {
			t.Errorf("MajorFromVersion(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestSimplifyVendor(t *testing.T) {
	cases := map[string]string{
		"Eclipse Adoptium":                    "Temurin",
		"Oracle Corporation":                  "Oracle",
		"Azul Systems, Inc.":                  "Azul",
		"Amazon.com Inc.":                     "Corretto",
		"BellSoft":                            "Liberica",
		"Some Very Long Java Vendor Name Inc": "Some Very Long Java Vend",
	}
	for input, want := range cases {
		if got := simplifyVendor(input); got != want {
			t.Errorf("simplifyVendor(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCompatible(t *testing.T) {
	cases := []struct {
		actual, required int
		want             bool
	}{
		{8, 8, true},
		{8, 16, false},
		{16, 17, false},
		{17, 16, true},
		{21, 17, true},
		{25, 21, true},
		{21, 25, false},
		{11, 8, false},
	}
	for _, tc := range cases {
		if got := Compatible(tc.actual, tc.required); got != tc.want {
			t.Errorf("Compatible(%d, %d) = %v, want %v", tc.actual, tc.required, got, tc.want)
		}
	}
}

func TestResolveExecutableDirectory(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(bin, javaExeName())
	if err := os.WriteFile(exe, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, home, err := resolveExecutable(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != exe || home != root {
		t.Fatalf("resolveExecutable = %q, %q", got, home)
	}
}
