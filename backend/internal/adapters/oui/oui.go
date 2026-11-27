// Package oui finds the manufacturer of a MAC address in the IEEE registry
// (MA-L /24, MA-M /28, MA-S /36), embedded in the binary.
//
// Refresh the data with `make oui` (go run ./internal/adapters/oui/gen).
package oui

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"net"
	"strings"
	"sync"
)

//go:embed oui.tsv.gz
var data []byte

// DB is the vendor lookup. The zero value isn't usable: call Load.
type DB struct {
	// By prefix length in hex digits: 9 (/36), 7 (/28), 6 (/24)
	byLen map[int]map[string]string
}

var (
	once   sync.Once
	shared *DB
	err    error
)

// Load parses the embedded registry once (a few tens of milliseconds).
func Load() (*DB, error) {
	once.Do(func() { shared, err = parse(data) })
	return shared, err
}

func parse(gz []byte) (*DB, error) {
	r, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	db := &DB{byLen: map[int]map[string]string{6: {}, 7: {}, 9: {}}}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		prefix, name, ok := strings.Cut(sc.Text(), "\t")
		if !ok {
			continue
		}
		if m, ok := db.byLen[len(prefix)]; ok {
			m[prefix] = name
		}
	}
	return db, sc.Err()
}

// Vendor returns the manufacturer of the MAC, or "" when it is unknown or
// locally administered (a random MAC carries no vendor).
func (db *DB) Vendor(mac string) string {
	hw, err := net.ParseMAC(mac)
	if err != nil || len(hw) != 6 || hw[0]&0x02 != 0 {
		return ""
	}
	hex := strings.ToUpper(strings.NewReplacer(":", "").Replace(hw.String()))
	for _, n := range []int{9, 7, 6} { // longest prefix first
		if name, ok := db.byLen[n][hex[:n]]; ok {
			return name
		}
	}
	return ""
}

// Len is the number of prefixes loaded.
func (db *DB) Len() int {
	n := 0
	for _, m := range db.byLen {
		n += len(m)
	}
	return n
}
