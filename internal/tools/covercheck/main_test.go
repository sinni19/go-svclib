package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const profile = `mode: atomic
example.com/m/a/a.go:1.1,2.1 3 1
example.com/m/a/a.go:3.1,4.1 1 0
example.com/m/b/b.go:1.1,2.1 2 0
example.com/m/b/b.go:1.1,2.1 2 5
example.com/m/gen/g.go:1.1,2.1 10 0
`

func TestCheck(t *testing.T) {
	tests := []struct {
		name      string
		exclude   []string
		wantPkgs  string
		wantTotal string
	}{
		{
			name: "blocks repeated across binaries count once, covered if any hit",
			// a: 3 of 4; b: 2 of 2 (one of two copies hit); gen: 0 of 10.
			wantPkgs:  "example.com/m/a=3/4 example.com/m/b=2/2 example.com/m/gen=0/10",
			wantTotal: "5/16",
		},
		{
			name:      "excluded files are ignored",
			exclude:   []string{"/gen/"},
			wantPkgs:  "example.com/m/a=3/4 example.com/m/b=2/2",
			wantTotal: "5/6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkgs, total, err := check(strings.NewReader(profile), tt.exclude)
			if err != nil {
				t.Fatal(err)
			}

			var got []string
			for _, p := range pkgs {
				got = append(got, p.pkg+"="+frac(p))
			}

			if strings.Join(got, " ") != tt.wantPkgs || frac(total) != tt.wantTotal {
				t.Errorf("got %v total %s; want %s total %s", got, frac(total), tt.wantPkgs, tt.wantTotal)
			}
		})
	}
}

func frac(r result) string {
	return strconv.Itoa(r.covered) + "/" + strconv.Itoa(r.all)
}

func TestCheckBadProfile(t *testing.T) {
	for _, in := range []string{
		"mode: set\nfoo\n",
		"mode: set\nnocolon 1 1\n",
		"mode: set\na.go:1.1,2.1 x 1\n",
	} {
		if _, _, err := check(strings.NewReader(in), nil); err == nil {
			t.Errorf("check(%q) error = nil", in)
		}
	}
}

func TestReport(t *testing.T) {
	pkgs := []result{{pkg: "a", covered: 9, all: 10}, {pkg: "b", covered: 8, all: 10}, {pkg: "empty"}}
	total := result{pkg: "total", covered: 17, all: 20}

	var out bytes.Buffer

	// b (80%) and the total (85%) decide the result at each minimum.
	if !report(&out, pkgs, total, 80) {
		t.Errorf("report(80) failed:\n%s", out.String())
	}

	out.Reset()

	if report(&out, pkgs, total, 85) {
		t.Errorf("report(85) passed with b at 80%%:\n%s", out.String())
	}

	if !strings.Contains(out.String(), "FAIL   80.0%  b") || !strings.Contains(out.String(), "ok    100.0%  empty") {
		t.Errorf("unexpected report:\n%s", out.String())
	}
}

func TestSplitNonEmpty(t *testing.T) {
	if got := strings.Join(splitNonEmpty(" a, ,b,"), "|"); got != "a|b" {
		t.Errorf("splitNonEmpty = %q", got)
	}
}

func TestRun(t *testing.T) {
	prof := filepath.Join(t.TempDir(), "c.out")
	if err := os.WriteFile(prof, []byte(profile), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "meets minimum", args: []string{"-min", "70", "-exclude", "/gen/", prof}, want: 0},
		{name: "package a (75%) below minimum", args: []string{"-min", "80", "-exclude", "/gen/", prof}, want: 1},
		{name: "no profile", args: nil, want: 2},
		{name: "unknown flag", args: []string{"-x", prof}, want: 2},
		{name: "missing file", args: []string{prof + ".missing"}, want: 2},
		{name: "bad profile", args: []string{"main_test.go"}, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer

			if got := run(tt.args, &out, &errOut); got != tt.want {
				t.Errorf("run(%v) = %d, want %d\nstdout: %s\nstderr: %s", tt.args, got, tt.want, out.String(), errOut.String())
			}
		})
	}
}
