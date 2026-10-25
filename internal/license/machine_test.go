package license

import "testing"

func TestMachineCodeVectors(t *testing.T) {
	cases := []struct {
		name       string
		components Components
		want       string
	}{
		{
			name: "basic",
			components: Components{
				"arch": "x86_64", "host": "desktop-abc", "mac": "AABBCCDDEEFF", "os": "windows",
			},
			want: "0528-F28F-281B-6C76",
		},
		{
			name: "full",
			components: Components{
				"arch": "arm64", "board": "board-1", "cpu": "apple m1", "disk": "1234abcd",
				"host": "macbook-pro", "mac": "001122334455", "machine_guid": "uuid-1234", "os": "darwin",
			},
			want: "BD9B-69F7-DD66-2D86",
		},
		{
			name:       "no_mac",
			components: Components{"arch": "x86_64", "host": "server-01", "os": "linux"},
			want:       "355A-B2B9-A809-D6CC",
		},
		{
			name:       "normalized",
			components: Components{"arch": "AMD64", "host": " Server-01 ", "mac": "aa:bb:cc:dd:ee:ff", "os": "Linux"},
			want:       "B939-9F09-A1A6-B789",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := MachineCode(test.components); got != test.want {
				t.Fatalf("MachineCode() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestProductConfigInjection(t *testing.T) {
	useTestProduct(t)
	key, err := productAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(key) < 20 || key[:3] != "lg_" {
		t.Fatalf("unexpected product key format")
	}
	name, slug := ProductInfo()
	if name == "" || slug == "" {
		t.Fatalf("product info did not decrypt")
	}
	config, err := loadProductConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Server == "" || config.Server[:8] != "https://" {
		t.Fatalf("production endpoint must be HTTPS")
	}
}

func TestMachineCodeOmitsEmptyComponents(t *testing.T) {
	withEmpty := Components{"arch": "x86_64", "host": "server-01", "mac": "", "os": "linux"}
	if got, want := MachineCode(withEmpty), "355A-B2B9-A809-D6CC"; got != want {
		t.Fatalf("empty component changed hash: %q", got)
	}
}
