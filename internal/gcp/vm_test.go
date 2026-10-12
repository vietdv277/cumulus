package gcp

import "testing"

func TestLocationClassification(t *testing.T) {
	cases := []struct {
		name   string
		value  string
		isZone bool
		isReg  bool
	}{
		{"zone us-central1-a", "us-central1-a", true, false},
		{"zone europe-west1-b", "europe-west1-b", true, false},
		{"zone us-east4-c", "us-east4-c", true, false},
		{"zone me-central2-a", "me-central2-a", true, false},
		{"zone africa-south1-a", "africa-south1-a", true, false},
		{"zone multi-word region northamerica-northeast1-a", "northamerica-northeast1-a", true, false},
		{"zone asia-southeast1-b", "asia-southeast1-b", true, false},
		{"region us-central1", "us-central1", false, true},
		{"region europe-west1", "europe-west1", false, true},
		{"region northamerica-northeast1", "northamerica-northeast1", false, true},
		{"region me-central2", "me-central2", false, true},
		{"empty", "", false, false},
		{"malformed zone letter us-central1-z", "us-central1-z", false, false},
		{"trailing hyphen us-central1-", "us-central1-", false, false},
		{"double digit zone suffix us-central1-ab", "us-central1-ab", false, false},
		{"region without zone suffix us-central1", "us-central1", false, true},
		{"arbitrary name", "bastion-host", false, false},
		{"uppercase", "US-CENTRAL1-A", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isZone(tc.value); got != tc.isZone {
				t.Errorf("isZone(%q) = %v, want %v", tc.value, got, tc.isZone)
			}
			if got := isRegion(tc.value); got != tc.isReg {
				t.Errorf("isRegion(%q) = %v, want %v", tc.value, got, tc.isReg)
			}
		})
	}
}

func TestValidateRegion(t *testing.T) {
	for _, ok := range []string{"", "us-central1", "northamerica-northeast1", "us-central1-a"} {
		if err := validateRegion(ok); err != nil {
			t.Errorf("validateRegion(%q) error = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"us-central1-z", "us-central1-", "bastion-host", "US-CENTRAL1-A"} {
		if err := validateRegion(bad); err == nil {
			t.Errorf("validateRegion(%q) = nil, want error", bad)
		}
	}
}

func TestResolveBastionZone(t *testing.T) {
	cases := []struct {
		name        string
		bastionZone string
		region      string
		want        string
		wantErr     bool
	}{
		{
			name:        "explicit bastion zone wins",
			bastionZone: "asia-southeast1-b",
			region:      "asia-southeast1",
			want:        "asia-southeast1-b",
		},
		{
			name:        "zone region used as fallback",
			bastionZone: "",
			region:      "us-central1-a",
			want:        "us-central1-a",
		},
		{
			name:        "plain region cannot name a bastion",
			bastionZone: "",
			region:      "asia-southeast1",
			wantErr:     true,
		},
		{
			name:        "malformed region cannot name a bastion",
			bastionZone: "",
			region:      "us-central1-z",
			wantErr:     true,
		},
		{
			name:        "no zone and no region",
			bastionZone: "",
			region:      "",
			wantErr:     true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveBastionZone(tc.bastionZone, tc.region)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveBastionZone(%q, %q) = %q, want error", tc.bastionZone, tc.region, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveBastionZone(%q, %q) error = %v", tc.bastionZone, tc.region, err)
			}
			if got != tc.want {
				t.Errorf("resolveBastionZone(%q, %q) = %q, want %q", tc.bastionZone, tc.region, got, tc.want)
			}
		})
	}
}
