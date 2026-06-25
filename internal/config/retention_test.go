package config

import "testing"

func TestRetentionConfig_Validate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     *RetentionConfig
		wantErr bool
	}{
		{"nil keeps everything", nil, false},
		{"all-zero is valid", &RetentionConfig{}, false},
		{"valid full policy", &RetentionConfig{KeepLast: 7, MaxAge: "720h", GFS: GFSConfig{Daily: 7, Weekly: 4, Monthly: 6}}, false},
		{"negative keep_last", &RetentionConfig{KeepLast: -1}, true},
		{"negative gfs tier", &RetentionConfig{GFS: GFSConfig{Weekly: -2}}, true},
		{"bad duration", &RetentionConfig{MaxAge: "not-a-duration"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Validate(); (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestEffectiveRetention_Precedence(t *testing.T) {
	def := &RetentionConfig{KeepLast: 7}
	prof := &RetentionConfig{KeepLast: 30}
	cfg := &Config{
		Defaults: Defaults{Retention: def},
		Profiles: map[string]ProfileConfig{
			"prod":    {Name: "prod", Retention: prof},
			"staging": {Name: "staging"}, // no override
		},
	}

	if got := cfg.EffectiveRetention("prod"); got == nil || got.KeepLast != 30 {
		t.Errorf("prod effective = %+v, want profile override keep_last=30", got)
	}
	if got := cfg.EffectiveRetention("staging"); got == nil || got.KeepLast != 7 {
		t.Errorf("staging effective = %+v, want defaults keep_last=7", got)
	}
	if got := cfg.EffectiveRetention("unknown"); got == nil || got.KeepLast != 7 {
		t.Errorf("unknown-profile effective = %+v, want defaults", got)
	}

	// No defaults block at all → nil (keep everything).
	empty := &Config{Profiles: map[string]ProfileConfig{}}
	if got := empty.EffectiveRetention("x"); got != nil {
		t.Errorf("no config effective = %+v, want nil (keep everything)", got)
	}
}
