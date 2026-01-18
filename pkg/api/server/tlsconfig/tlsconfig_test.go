package tlsconfig

import (
	"crypto/tls"
	"os"
	"testing"
)

func TestParseTLSVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    uint16
		wantErr bool
	}{
		{"TLS 1.0", "1.0", tls.VersionTLS10, false},
		{"TLS 1.1", "1.1", tls.VersionTLS11, false},
		{"TLS 1.2", "1.2", tls.VersionTLS12, false},
		{"TLS 1.3", "1.3", tls.VersionTLS13, false},
		{"TLSv1.2", "TLSv1.2", tls.VersionTLS12, false},
		{"TLS1.3", "TLS1.3", tls.VersionTLS13, false},
		{"Numeric TLS 1.3", "772", tls.VersionTLS13, false},
		{"Numeric TLS 1.2", "771", tls.VersionTLS12, false},
		{"Invalid", "invalid", 0, true},
		{"Empty", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTLSVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTLSVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseTLSVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseCipherSuites(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []uint16
		wantErr bool
	}{
		{
			name:  "Single IANA name",
			input: "TLS_AES_128_GCM_SHA256",
			want:  []uint16{tls.TLS_AES_128_GCM_SHA256},
		},
		{
			name:  "Multiple IANA names",
			input: "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384",
			want:  []uint16{tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384},
		},
		{
			name:  "TLS 1.2 ciphers",
			input: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
			want:  []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384},
		},
		{
			name:  "Numeric IDs",
			input: "4865,4866",
			want:  []uint16{4865, 4866},
		},
		{
			name:  "Mixed formats with spaces",
			input: "TLS_AES_128_GCM_SHA256, 4866, TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			want:  []uint16{tls.TLS_AES_128_GCM_SHA256, 4866, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
		},
		{
			name:  "Empty string",
			input: "",
			want:  nil,
		},
		{
			name:    "All invalid",
			input:   "INVALID1,INVALID2",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCipherSuites(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCipherSuites() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("parseCipherSuites() length = %v, want %v", len(got), len(tt.want))
					return
				}
				for i := range got {
					if got[i] != tt.want[i] {
						t.Errorf("parseCipherSuites()[%d] = %v, want %v", i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}

func TestParseCurvePreferences(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []tls.CurveID
		wantErr bool
	}{
		{
			name:  "Single curve",
			input: "X25519",
			want:  []tls.CurveID{tls.X25519},
		},
		{
			name:  "Multiple curves",
			input: "X25519,P256,P384",
			want:  []tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384},
		},
		{
			name:  "PQC hybrid curve by name",
			input: "X25519Kyber768Draft00,X25519,P256",
			want:  []tls.CurveID{tls.CurveID(25497), tls.X25519, tls.CurveP256},
		},
		{
			name:  "Numeric IDs",
			input: "29,23",
			want:  []tls.CurveID{29, 23},
		},
		{
			name:  "PQC curve by numeric ID",
			input: "25497,29,23",
			want:  []tls.CurveID{25497, 29, 23},
		},
		{
			name:  "With spaces",
			input: "X25519, P256, P384",
			want:  []tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384},
		},
		{
			name:  "Empty string",
			input: "",
			want:  nil,
		},
		{
			name:    "All invalid",
			input:   "INVALID1,INVALID2",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCurvePreferences(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCurvePreferences() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("parseCurvePreferences() length = %v, want %v", len(got), len(tt.want))
					return
				}
				for i := range got {
					if got[i] != tt.want[i] {
						t.Errorf("parseCurvePreferences()[%d] = %v, want %v", i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}

func TestToTLSConfig(t *testing.T) {
	tests := []struct {
		name       string
		config     *Config
		wantMin    uint16
		wantCurves int
		wantErr    bool
	}{
		{
			name: "TLS 1.3 minimum",
			config: &Config{
				MinTLSVersion: "1.3",
			},
			wantMin: tls.VersionTLS13,
		},
		{
			name: "TLS 1.2 minimum",
			config: &Config{
				MinTLSVersion: "1.2",
			},
			wantMin: tls.VersionTLS12,
		},
		{
			name:    "Default (no config)",
			config:  &Config{},
			wantMin: tls.VersionTLS12, // Explicit TLS 1.2 default
		},
		{
			name: "With curve preferences",
			config: &Config{
				MinTLSVersion:    "1.3",
				CurvePreferences: "X25519,P256",
			},
			wantMin:    tls.VersionTLS13,
			wantCurves: 2,
		},
		{
			name: "With PQC curve",
			config: &Config{
				MinTLSVersion:    "1.3",
				CurvePreferences: "X25519Kyber768Draft00,X25519,P256",
			},
			wantMin:    tls.VersionTLS13,
			wantCurves: 3,
		},
		{
			name: "Invalid min version",
			config: &Config{
				MinTLSVersion: "invalid",
			},
			wantErr: true,
		},
		{
			name: "Invalid cipher suites",
			config: &Config{
				CipherSuites: "INVALID1,INVALID2",
			},
			wantErr: true,
		},
		{
			name: "Invalid curve preferences",
			config: &Config{
				CurvePreferences: "INVALID1,INVALID2",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.config.ToTLSConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("ToTLSConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.MinVersion != tt.wantMin {
					t.Errorf("ToTLSConfig().MinVersion = %v, want %v", got.MinVersion, tt.wantMin)
				}
				if len(got.CurvePreferences) != tt.wantCurves {
					t.Errorf("ToTLSConfig().CurvePreferences length = %v, want %v", len(got.CurvePreferences), tt.wantCurves)
				}
				if len(got.NextProtos) == 0 {
					t.Error("ToTLSConfig().NextProtos should not be empty")
				}
			}
		})
	}
}

func TestLoadFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want *Config
	}{
		{
			name: "All values set",
			env: map[string]string{
				"TLS_MIN_VERSION":       "1.3",
				"TLS_CIPHER_SUITES":     "TLS_AES_128_GCM_SHA256",
				"TLS_CURVE_PREFERENCES": "X25519,P256",
			},
			want: &Config{
				MinTLSVersion:    "1.3",
				CipherSuites:     "TLS_AES_128_GCM_SHA256",
				CurvePreferences: "X25519,P256",
			},
		},
		{
			name: "Empty environment",
			env:  map[string]string{},
			want: &Config{},
		},
		{
			name: "Only min version",
			env: map[string]string{
				"TLS_MIN_VERSION": "1.2",
			},
			want: &Config{
				MinTLSVersion: "1.2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string {
				return tt.env[key]
			}
			got := LoadFromEnv(getenv)
			if got.MinTLSVersion != tt.want.MinTLSVersion {
				t.Errorf("LoadFromEnv().MinTLSVersion = %v, want %v", got.MinTLSVersion, tt.want.MinTLSVersion)
			}
			if got.CipherSuites != tt.want.CipherSuites {
				t.Errorf("LoadFromEnv().CipherSuites = %v, want %v", got.CipherSuites, tt.want.CipherSuites)
			}
			if got.CurvePreferences != tt.want.CurvePreferences {
				t.Errorf("LoadFromEnv().CurvePreferences = %v, want %v", got.CurvePreferences, tt.want.CurvePreferences)
			}
		})
	}
}

func TestGetTLSVersionName(t *testing.T) {
	tests := []struct {
		version uint16
		want    string
	}{
		{tls.VersionTLS10, "TLS 1.0"},
		{tls.VersionTLS11, "TLS 1.1"},
		{tls.VersionTLS12, "TLS 1.2"},
		{tls.VersionTLS13, "TLS 1.3"},
		{0, "default"},
		{0x9999, "Unknown (0x9999)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := GetTLSVersionName(tt.version)
			if got != tt.want {
				t.Errorf("GetTLSVersionName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetCurveName(t *testing.T) {
	tests := []struct {
		curve tls.CurveID
		want  string
	}{
		{tls.X25519, "X25519"},
		{tls.CurveP256, "P256"},
		{tls.CurveP384, "P384"},
		{tls.CurveP521, "P521"},
		{tls.CurveID(25497), "X25519Kyber768Draft00"},
		{tls.CurveID(0x9999), "Unknown (0x9999)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := GetCurveName(tt.curve)
			if got != tt.want {
				t.Errorf("GetCurveName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetCipherSuiteName(t *testing.T) {
	tests := []struct {
		id   uint16
		want string
	}{
		{tls.TLS_AES_128_GCM_SHA256, "TLS_AES_128_GCM_SHA256"},
		{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"},
		{0x9999, "Unknown (0x9999)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := GetCipherSuiteName(tt.id)
			if got != tt.want {
				t.Errorf("GetCipherSuiteName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetCipherSuiteMap(t *testing.T) {
	// Verify that the cipher suite map is populated from Go's built-in functions
	cipherMap := getCipherSuiteMap()

	// Check that we have some expected ciphers
	expectedCiphers := []string{
		"TLS_AES_128_GCM_SHA256",
		"TLS_AES_256_GCM_SHA384",
		"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
	}

	for _, name := range expectedCiphers {
		if _, ok := cipherMap[name]; !ok {
			t.Errorf("Expected cipher %s not found in cipher map", name)
		}
	}
}

func TestHasEnvOverrides(t *testing.T) {
	// Save original values and restore after test
	origMinVersion := os.Getenv("TLS_MIN_VERSION")
	origCipherSuites := os.Getenv("TLS_CIPHER_SUITES")
	origCurvePrefs := os.Getenv("TLS_CURVE_PREFERENCES")
	defer func() {
		os.Setenv("TLS_MIN_VERSION", origMinVersion)
		os.Setenv("TLS_CIPHER_SUITES", origCipherSuites)
		os.Setenv("TLS_CURVE_PREFERENCES", origCurvePrefs)
	}()

	tests := []struct {
		name        string
		minVersion  string
		cipherSuite string
		curvePrefs  string
		want        bool
	}{
		{
			name: "No env vars set",
			want: false,
		},
		{
			name:       "Only TLS_MIN_VERSION set",
			minVersion: "1.3",
			want:       true,
		},
		{
			name:        "Only TLS_CIPHER_SUITES set",
			cipherSuite: "TLS_AES_128_GCM_SHA256",
			want:        true,
		},
		{
			name:       "Only TLS_CURVE_PREFERENCES set",
			curvePrefs: "X25519",
			want:       true,
		},
		{
			name:        "All env vars set",
			minVersion:  "1.3",
			cipherSuite: "TLS_AES_128_GCM_SHA256",
			curvePrefs:  "X25519",
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all TLS env vars
			os.Unsetenv("TLS_MIN_VERSION")
			os.Unsetenv("TLS_CIPHER_SUITES")
			os.Unsetenv("TLS_CURVE_PREFERENCES")

			// Set the ones specified in this test case
			if tt.minVersion != "" {
				os.Setenv("TLS_MIN_VERSION", tt.minVersion)
			}
			if tt.cipherSuite != "" {
				os.Setenv("TLS_CIPHER_SUITES", tt.cipherSuite)
			}
			if tt.curvePrefs != "" {
				os.Setenv("TLS_CURVE_PREFERENCES", tt.curvePrefs)
			}

			got := HasEnvOverrides()
			if got != tt.want {
				t.Errorf("HasEnvOverrides() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatCipherSuites(t *testing.T) {
	tests := []struct {
		name  string
		input []uint16
		want  string
	}{
		{
			name:  "Empty returns default",
			input: nil,
			want:  "default",
		},
		{
			name:  "Empty slice returns default",
			input: []uint16{},
			want:  "default",
		},
		{
			name:  "Single known cipher",
			input: []uint16{tls.TLS_AES_128_GCM_SHA256},
			want:  "TLS_AES_128_GCM_SHA256",
		},
		{
			name:  "Multiple known ciphers",
			input: []uint16{tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384},
			want:  "TLS_AES_128_GCM_SHA256, TLS_AES_256_GCM_SHA384",
		},
		{
			name:  "Unknown cipher shows hex ID",
			input: []uint16{0x9999},
			want:  "Unknown (0x9999)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatCipherSuites(tt.input)
			if got != tt.want {
				t.Errorf("FormatCipherSuites() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatCurvePreferences(t *testing.T) {
	tests := []struct {
		name  string
		input []tls.CurveID
		want  string
	}{
		{
			name:  "Empty returns default",
			input: nil,
			want:  "default",
		},
		{
			name:  "Empty slice returns default",
			input: []tls.CurveID{},
			want:  "default",
		},
		{
			name:  "Single known curve",
			input: []tls.CurveID{tls.X25519},
			want:  "X25519",
		},
		{
			name:  "Multiple known curves",
			input: []tls.CurveID{tls.X25519, tls.CurveP256},
			want:  "X25519, P256",
		},
		{
			name:  "Unknown curve shows hex ID",
			input: []tls.CurveID{0x9999},
			want:  "Unknown (0x9999)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatCurvePreferences(tt.input)
			if got != tt.want {
				t.Errorf("FormatCurvePreferences() = %v, want %v", got, tt.want)
			}
		})
	}
}
