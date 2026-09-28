package application

import (
	"archive/zip"
	"bytes"
	"errors"
	"testing"
)

func TestArchiveEntryPathNormalizesSeparatorsAndDotPrefix(t *testing.T) {
	cases := map[string]string{
		`assets\pic.png`: "assets/pic.png",
		"./card.json":    "card.json",
		"assets/i.png":   "assets/i.png",
		"x":              "x",
	}
	for in, want := range cases {
		if got := archiveEntryPath(in); got != want {
			t.Errorf("archiveEntryPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadZipEntryRejectsOversizedEntry(t *testing.T) {
	f := &zip.File{FileHeader: zip.FileHeader{Name: "big.bin"}}
	f.UncompressedSize64 = 100
	if _, err := readZipEntry(f, 10); !errors.Is(err, errCardArchiveEntryTooLarge) {
		t.Fatalf("want errCardArchiveEntryTooLarge, got %v", err)
	}
}

func TestCheckArchiveEntryCountEnforcesLimit(t *testing.T) {
	atLimit := &zip.Reader{File: make([]*zip.File, maxCardArchiveEntries)}
	if err := checkArchiveEntryCount(atLimit); err != nil {
		t.Fatalf("at limit should pass, got %v", err)
	}
	tooMany := &zip.Reader{File: make([]*zip.File, maxCardArchiveEntries+1)}
	if err := checkArchiveEntryCount(tooMany); !errors.Is(err, errCardArchiveTooManyEntries) {
		t.Fatalf("want errCardArchiveTooManyEntries, got %v", err)
	}
	if _, err := cardArchiveEntries(tooMany); !errors.Is(err, errCardArchiveTooManyEntries) {
		t.Fatalf("cardArchiveEntries want errCardArchiveTooManyEntries, got %v", err)
	}
}

func TestCardArchiveEntriesSkipsDirsAndNormalizesPaths(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if _, err := zw.Create("assets/"); err != nil {
		t.Fatal(err)
	}
	w, err := zw.Create(`assets\pic.png`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("data")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	byPath, err := cardArchiveEntries(zr)
	if err != nil {
		t.Fatalf("cardArchiveEntries: %v", err)
	}
	if len(byPath) != 1 {
		t.Fatalf("want 1 file (dirs skipped), got %d: %v", len(byPath), byPath)
	}
	if byPath["assets/pic.png"] == nil {
		t.Fatalf("path not normalized: %v", byPath)
	}
}
