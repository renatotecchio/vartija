package main

import "testing"

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"Espressif Inc.":                  "Espressif",
		"TP-LINK TECHNOLOGIES CO.,LTD.":   "TP-LINK TECHNOLOGIES",
		"Apple, Inc.":                     "Apple",
		"Raspberry Pi Trading Ltd":        "Raspberry Pi Trading",
		"  Routerboard.com  ":             "Routerboard.com",
		"Samsung Electronics Co.,Ltd":     "Samsung Electronics",
		"Hon Hai Precision Ind. Co.,Ltd.": "Hon Hai Precision Ind",
		"Intel Corporate":                 "Intel Corporate",
		"Ubiquiti Inc":                    "Ubiquiti",
		"Google, Inc.":                    "Google",
		"AVM Audiovisuelles Marketing und Computersysteme GmbH": "AVM Audiovisuelles Marketing und Computersysteme",
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}
