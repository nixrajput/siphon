package config

import "testing"

func TestStorageConfig_Validate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     StorageConfig
		wantErr bool
	}{
		{"empty defaults to local", StorageConfig{}, false},
		{"explicit local", StorageConfig{Type: "local"}, false},
		{"s3 with bucket", StorageConfig{Type: "s3", Bucket: "dumps"}, false},
		{"s3 without bucket", StorageConfig{Type: "s3"}, true},
		{"unknown type", StorageConfig{Type: "gcs"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
