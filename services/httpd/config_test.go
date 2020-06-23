package httpd_test

import (
	"reflect"
	"strings"
	"testing"
	"text/template"

	"github.com/BurntSushi/toml"
	"github.com/influxdata/influxdb/services/httpd"
)

// TestConfig_ParseHeaders ensures we can properly extract the user supplied
// headers from the configuration file.
//
// This test is a success if we get the data out as we put into the TOML
// values.
//
func TestConfig_ParseHeaders(t *testing.T) {
	expectedHeaders := map[string]string{
		"X-BestOperatingSystem": "FreeBSD",
		"X-Hacker":              "If you're reading this, you're a hacker",
		"X-PoweredBy":           "Nerd Rage",
	}

	// build a toml snippet that reflects the structure we have above
	tomlData := func() (string, error) {
		tmpl := template.New("default")
		v := `
[headers]
{{- range $hdr, $val := .}}
{{$hdr}} = "{{$val}}"
{{- end -}}
`
		if _, err := tmpl.Parse(v); err != nil {
			return "", err
		}

		sb := &strings.Builder{}

		tmpl.Execute(sb, expectedHeaders)
		return sb.String(), nil
	}

	tomlConfig, err := tomlData()
	if err != nil {
		t.Fatal(err)
	}

	c := httpd.Config{}
	if _, err := toml.Decode(tomlConfig, &c); err != nil {
		t.Fatal(err)
	}

	// place results into a map so we check if it is DeepEqual() to
	// expectedHeaders

	if !reflect.DeepEqual(expectedHeaders, c.HTTPHeaders) {
		t.Fatalf("could not properly marshal nested slices; got %#v, expected %#v", c.HTTPHeaders, expectedHeaders)
	}
}

func TestConfig_Parse(t *testing.T) {
	// Parse configuration.
	var c httpd.Config
	if _, err := toml.Decode(`
enabled = true
bind-address = ":8080"
auth-enabled = true
log-enabled = true
write-tracing = true
https-enabled = true
https-certificate = "/dev/null"
unix-socket-enabled = true
bind-socket = "/var/run/influxdb.sock"
max-body-size = 100
`, &c); err != nil {
		t.Fatal(err)
	}

	// Validate configuration.
	if !c.Enabled {
		t.Fatalf("unexpected enabled: %v", c.Enabled)
	} else if c.BindAddress != ":8080" {
		t.Fatalf("unexpected bind address: %s", c.BindAddress)
	} else if !c.AuthEnabled {
		t.Fatalf("unexpected auth enabled: %v", c.AuthEnabled)
	} else if !c.LogEnabled {
		t.Fatalf("unexpected log enabled: %v", c.LogEnabled)
	} else if !c.WriteTracing {
		t.Fatalf("unexpected write tracing: %v", c.WriteTracing)
	} else if !c.HTTPSEnabled {
		t.Fatalf("unexpected https enabled: %v", c.HTTPSEnabled)
	} else if c.HTTPSCertificate != "/dev/null" {
		t.Fatalf("unexpected https certificate: %v", c.HTTPSCertificate)
	} else if !c.UnixSocketEnabled {
		t.Fatalf("unexpected unix socket enabled: %v", c.UnixSocketEnabled)
	} else if c.BindSocket != "/var/run/influxdb.sock" {
		t.Fatalf("unexpected bind unix socket: %v", c.BindSocket)
	} else if c.MaxBodySize != 100 {
		t.Fatalf("unexpected max-body-size: %v", c.MaxBodySize)
	}
}

func TestConfig_WriteTracing(t *testing.T) {
	c := httpd.Config{WriteTracing: true}
	s := httpd.NewService(c)
	if !s.Handler.Config.WriteTracing {
		t.Fatalf("write tracing was not set")
	}
}

func TestConfig_StatusFilter(t *testing.T) {
	for i, tt := range []struct {
		cfg     string
		status  int
		matches bool
	}{
		{
			cfg:     ``,
			status:  200,
			matches: true,
		},
		{
			cfg:     ``,
			status:  404,
			matches: true,
		},
		{
			cfg:     ``,
			status:  500,
			matches: true,
		},
		{
			cfg: `
access-log-status-filters = []
`,
			status:  200,
			matches: true,
		},
		{
			cfg: `
access-log-status-filters = []
`,
			status:  404,
			matches: true,
		},
		{
			cfg: `
access-log-status-filters = []
`,
			status:  500,
			matches: true,
		},
		{
			cfg: `
access-log-status-filters = ["4xx"]
`,
			status:  200,
			matches: false,
		},
		{
			cfg: `
access-log-status-filters = ["4xx"]
`,
			status:  404,
			matches: true,
		},
		{
			cfg: `
access-log-status-filters = ["4xx"]
`,
			status:  400,
			matches: true,
		},
		{
			cfg: `
access-log-status-filters = ["4xx"]
`,
			status:  500,
			matches: false,
		},
		{
			cfg: `
access-log-status-filters = ["4xx", "5xx"]
`,
			status:  200,
			matches: false,
		},
		{
			cfg: `
access-log-status-filters = ["4xx", "5xx"]
`,
			status:  404,
			matches: true,
		},
		{
			cfg: `
access-log-status-filters = ["4xx", "5xx"]
`,
			status:  400,
			matches: true,
		},
		{
			cfg: `
access-log-status-filters = ["4xx", "5xx"]
`,
			status:  500,
			matches: true,
		},
		{
			cfg: `
access-log-status-filters = ["400"]
`,
			status:  400,
			matches: true,
		},
		{
			cfg: `
access-log-status-filters = ["400"]
`,
			status:  404,
			matches: false,
		},
		{
			cfg: `
access-log-status-filters = ["40x"]
`,
			status:  400,
			matches: true,
		},
		{
			cfg: `
access-log-status-filters = ["40x"]
`,
			status:  404,
			matches: true,
		},
		{
			cfg: `
access-log-status-filters = ["40x"]
`,
			status:  419,
			matches: false,
		},
	} {
		// Parse configuration.
		var c httpd.Config
		if _, err := toml.Decode(tt.cfg, &c); err != nil {
			t.Fatal(err)
		}

		if got, want := httpd.StatusFilters(c.AccessLogStatusFilters).Match(tt.status), tt.matches; got != want {
			t.Errorf("%d. status was not filtered correctly: got=%v want=%v", i, got, want)
		}
	}
}

func TestConfig_StatusFilter_Error(t *testing.T) {
	for i, tt := range []struct {
		cfg string
		err string
	}{
		{
			cfg: `
access-log-status-filters = ["xxx"]
`,
			err: "status filter must be a digit that starts with 1-5 optionally followed by X characters",
		},
		{
			cfg: `
access-log-status-filters = ["4x4"]
`,
			err: "status filter must be a digit that starts with 1-5 optionally followed by X characters",
		},
		{
			cfg: `
access-log-status-filters = ["6xx"]
`,
			err: "status filter must be a digit that starts with 1-5 optionally followed by X characters",
		},
		{
			cfg: `
access-log-status-filters = ["0xx"]
`,
			err: "status filter must be a digit that starts with 1-5 optionally followed by X characters",
		},
		{
			cfg: `
access-log-status-filters = ["4xxx"]
`,
			err: "status filter must be exactly 3 characters long",
		},
	} {
		// Parse configuration.
		var c httpd.Config
		if _, err := toml.Decode(tt.cfg, &c); err == nil {
			t.Errorf("%d. expected error", i)
		} else if got, want := err.Error(), tt.err; got != want {
			t.Errorf("%d. config parsing error was not correct: got=%q want=%q", i, got, want)
		}
	}
}
