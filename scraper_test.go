package main

import (
	"os"
	"strings"
	"testing"
)

func TestParseDetails(t *testing.T) {
	body, err := os.ReadFile("testdata/fighters-megamix.html")
	if err != nil {
		t.Fatal(err)
	}
	d, err := parseDetails(body)
	if err != nil {
		t.Fatal(err)
	}
	if d.RepoURL != "https://github.com/tmcleroy/fighters-megamix-recomp" {
		t.Errorf("repo = %q", d.RepoURL)
	}
	want := []string{
		"https://raw.githubusercontent.com/tmcleroy/fighters-megamix-recomp/main/docs/screenshots/widescreen.jpg",
		"https://raw.githubusercontent.com/tmcleroy/fighters-megamix-recomp/main/docs/screenshots/fight.jpg",
		"https://raw.githubusercontent.com/tmcleroy/fighters-megamix-recomp/main/docs/screenshots/select.jpg",
	}
	if strings.Join(d.Screenshots, "\n") != strings.Join(want, "\n") {
		t.Errorf("screenshots = %v", d.Screenshots)
	}
	if !strings.Contains(d.ReadmeHTML, "static recompilation") {
		t.Errorf("readme missing article text")
	}
	if consoleName("saturn") != "Sega Saturn" || consoleName("og xbox") != "Xbox" {
		t.Error("console names")
	}
}

func TestReadmeHasNoBlankLineRuns(t *testing.T) {
	body, err := os.ReadFile("testdata/fighters-megamix.html")
	if err != nil {
		t.Fatal(err)
	}
	d, err := parseDetails(body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(d.ReadmeHTML, "\n\n") {
		t.Errorf("README still contains blank lines")
	}
}
