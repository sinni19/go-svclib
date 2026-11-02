// Command covercheck fails when the total statement coverage, or the coverage
// of any package, in a Go cover profile is below a minimum.
//
// Usage:
//
//	go run ./internal/tools/covercheck -min 85 -exclude /internal/gen/ coverage.out
//
// A profile made with -coverpkg lists a block once per test binary; a block
// counts as covered when any binary covers it.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
)

type block struct {
	pkg   string
	stmts int
	hit   bool
}

type result struct {
	pkg          string
	covered, all int
}

func (r result) percent() float64 {
	if r.all == 0 {
		return 100
	}

	return 100 * float64(r.covered) / float64(r.all)
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run returns the exit code: 0 when coverage meets the minimum, 1 when it
// does not, and 2 for bad arguments or an unreadable profile.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("covercheck", flag.ContinueOnError)
	fs.SetOutput(stderr)

	minPct := fs.Float64("min", 85, "minimum coverage percent for each package and the total")
	exclude := fs.String("exclude", "", "comma-separated substrings; matching files are ignored")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if fs.NArg() != 1 {
		_, _ = fmt.Fprintln(stderr, "usage: covercheck [-min N] [-exclude a,b] profile")

		return 2
	}

	f, err := os.Open(fs.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)

		return 2
	}
	defer func() { _ = f.Close() }()

	pkgs, total, err := check(f, splitNonEmpty(*exclude))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)

		return 2
	}

	if !report(stdout, pkgs, total, *minPct) {
		return 1
	}

	return 0
}

func splitNonEmpty(s string) []string {
	var out []string

	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}

	return out
}

// check returns the coverage of each package, sorted by path, and the total.
func check(r io.Reader, exclude []string) ([]result, result, error) {
	blocks := map[string]*block{}
	sc := bufio.NewScanner(r)

	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if line == 1 && strings.HasPrefix(text, "mode:") || text == "" {
			continue
		}

		// Format: file:startLine.col,endLine.col numStmts count
		fields := strings.Fields(text)
		if len(fields) != 3 {
			return nil, result{}, fmt.Errorf("line %d: bad profile line %q", line, text)
		}

		key := fields[0]

		file, _, ok := strings.Cut(key, ":")
		if !ok {
			return nil, result{}, fmt.Errorf("line %d: no position in %q", line, key)
		}

		if slices.ContainsFunc(exclude, func(e string) bool { return strings.Contains(file, e) }) {
			continue
		}

		stmts, err1 := strconv.Atoi(fields[1])
		count, err2 := strconv.Atoi(fields[2])

		if err := errors.Join(err1, err2); err != nil {
			return nil, result{}, fmt.Errorf("line %d: %w", line, err)
		}

		b, seen := blocks[key]
		if !seen {
			b = &block{pkg: path.Dir(file), stmts: stmts}
			blocks[key] = b
		}

		b.hit = b.hit || count > 0
	}

	if err := sc.Err(); err != nil {
		return nil, result{}, fmt.Errorf("read profile: %w", err)
	}

	byPkg := map[string]*result{}
	total := result{pkg: "total"}

	for _, b := range blocks {
		r, ok := byPkg[b.pkg]
		if !ok {
			r = &result{pkg: b.pkg}
			byPkg[b.pkg] = r
		}

		r.all += b.stmts
		total.all += b.stmts

		if b.hit {
			r.covered += b.stmts
			total.covered += b.stmts
		}
	}

	out := make([]result, 0, len(byPkg))
	for _, r := range byPkg {
		out = append(out, *r)
	}

	slices.SortFunc(out, func(a, b result) int { return strings.Compare(a.pkg, b.pkg) })

	return out, total, nil
}

// report writes one line for each package and the total, and reports
// whether all of them meet minPct.
func report(w io.Writer, pkgs []result, total result, minPct float64) bool {
	pass := true

	for _, r := range append(pkgs, total) {
		status := "ok"
		if r.percent() < minPct {
			status = "FAIL"
			pass = false
		}

		_, _ = fmt.Fprintf(w, "%-4s %6.1f%%  %s\n", status, r.percent(), r.pkg)
	}

	_, _ = fmt.Fprintf(w, "minimum %.0f%%\n", minPct)

	return pass
}
