// Command gen rebuilds oui.tsv.gz from the IEEE registries (MA-L, MA-M,
// MA-S): one "PREFIX<TAB>Vendor" line per assignment, the prefix in hex
// (6, 7 or 9 digits for /24, /28 and /36).
//
//	go run ./internal/adapters/oui/gen            # downloads from IEEE
//	go run ./internal/adapters/oui/gen a.csv ...  # from local copies
package main

import (
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

var sources = []string{
	"https://standards-oui.ieee.org/oui/oui.csv",
	"https://standards-oui.ieee.org/oui28/mam.csv",
	"https://standards-oui.ieee.org/oui36/oui36.csv",
}

const out = "internal/adapters/oui/oui.tsv.gz"

func main() {
	inputs := os.Args[1:]
	if len(inputs) == 0 {
		inputs = sources
	}
	entries := map[string]string{}
	for _, in := range inputs {
		if err := read(in, entries); err != nil {
			fmt.Fprintln(os.Stderr, in+":", err)
			os.Exit(1)
		}
	}
	if len(entries) < 30000 {
		fmt.Fprintf(os.Stderr, "only %d entries: the registries look incomplete\n", len(entries))
		os.Exit(1)
	}
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	f, err := os.Create(out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	gz, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
	gz.ModTime = time.Time{} // reproducible output
	for _, k := range keys {
		fmt.Fprintf(gz, "%s\t%s\n", k, entries[k])
	}
	if err := gz.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := f.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%s: %d prefixes\n", out, len(keys))
}

func read(src string, entries map[string]string) error {
	var r io.ReadCloser
	if strings.HasPrefix(src, "https://") {
		req, _ := http.NewRequest(http.MethodGet, src, nil)
		req.Header.Set("User-Agent", "vartija-oui-gen")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		r = resp.Body
	} else {
		f, err := os.Open(src)
		if err != nil {
			return err
		}
		r = f
	}
	defer r.Close()

	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	if _, err := cr.Read(); err != nil { // header
		return err
	}
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if len(rec) < 3 {
			continue
		}
		prefix := strings.ToUpper(strings.TrimSpace(rec[1]))
		name := Clean(rec[2])
		if prefix == "" || name == "" || strings.EqualFold(name, "Private") || strings.EqualFold(name, "IEEE Registration Authority") {
			continue
		}
		entries[prefix] = name
	}
}

var (
	spaces = regexp.MustCompile(`\s+`)
	// Corporate suffixes, removed from the end (repeatedly)
	suffix = regexp.MustCompile(`(?i)[,.]?\s+(inc|incorporated|corp|corporation|co|company|ltd|limited|llc|l\.l\.c|gmbh|ag|s\.?a|s\.?p\.?a|b\.?v|n\.?v|oy|ab|as|a/s|kg|pte|pty|plc|srl|s\.r\.l|sas|sarl|ltda|kk|k\.k)\.?$`)
	coLtd  = regexp.MustCompile(`(?i)[,.]?\s*co\.?,?\s*ltd\.?$`)
)

// Clean shortens a registered name for display: "Espressif Inc." becomes
// "Espressif", "TP-LINK TECHNOLOGIES CO.,LTD." becomes "TP-LINK TECHNOLOGIES".
func Clean(name string) string {
	name = strings.TrimSpace(spaces.ReplaceAllString(name, " "))
	for i := 0; i < 4; i++ {
		before := name
		name = coLtd.ReplaceAllString(name, "")
		name = suffix.ReplaceAllString(name, "")
		name = strings.TrimRight(strings.TrimSpace(name), ",.")
		if name == before {
			break
		}
	}
	return name
}
