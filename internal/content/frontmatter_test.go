package content

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestParseFrontMatter_Valid(t *testing.T) {
	input := `---
title: "My Post"
slug: "my-post"
date: 2026-03-22
draft: false
tags: ["go", "blog"]
categories: ["tech"]
author: "Jane"
description: "A great post"
template: "post.html"
image: "/img/hero.jpg"
---
# Hello World
`
	fm, body, err := ParseFrontMatter([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm == nil {
		t.Fatal("expected front matter, got nil")
	}
	if fm.Title != "My Post" {
		t.Errorf("title = %q, want %q", fm.Title, "My Post")
	}
	if fm.Slug != "my-post" {
		t.Errorf("slug = %q, want %q", fm.Slug, "my-post")
	}
	if fm.Date.Year() != 2026 || fm.Date.Month() != 3 || fm.Date.Day() != 22 {
		t.Errorf("date = %v, want 2026-03-22", fm.Date)
	}
	if fm.Draft {
		t.Error("draft should be false")
	}
	if len(fm.Tags) != 2 || fm.Tags[0] != "go" || fm.Tags[1] != "blog" {
		t.Errorf("tags = %v, want [go blog]", fm.Tags)
	}
	if len(fm.Categories) != 1 || fm.Categories[0] != "tech" {
		t.Errorf("categories = %v, want [tech]", fm.Categories)
	}
	if fm.Author != "Jane" {
		t.Errorf("author = %q, want %q", fm.Author, "Jane")
	}
	if fm.Description != "A great post" {
		t.Errorf("description = %q, want %q", fm.Description, "A great post")
	}
	if fm.Template != "post.html" {
		t.Errorf("template = %q, want %q", fm.Template, "post.html")
	}
	if fm.Image != "/img/hero.jpg" {
		t.Errorf("image = %q, want %q", fm.Image, "/img/hero.jpg")
	}
	if !strings.Contains(string(body), "# Hello World") {
		t.Errorf("body = %q, expected markdown content", string(body))
	}
}

func TestParseFrontMatter_Minimal(t *testing.T) {
	input := `---
title: "Quick Note"
date: 2026-01-15
---
Some content.
`
	fm, body, err := ParseFrontMatter([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Title != "Quick Note" {
		t.Errorf("title = %q, want %q", fm.Title, "Quick Note")
	}
	expected := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if !fm.Date.Equal(expected) {
		t.Errorf("date = %v, want %v", fm.Date, expected)
	}
	if !strings.Contains(string(body), "Some content.") {
		t.Errorf("body = %q, expected content", string(body))
	}
}

func TestParseFrontMatter_NoFrontMatter(t *testing.T) {
	input := "# Just Markdown\n\nNo front matter here."
	fm, body, err := ParseFrontMatter([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm != nil {
		t.Errorf("expected nil front matter, got %+v", fm)
	}
	if string(body) != input {
		t.Errorf("body should equal input when no front matter")
	}
}

func TestParseFrontMatter_MissingClose(t *testing.T) {
	input := "---\ntitle: \"Broken\"\nNo closing delimiter."
	_, _, err := ParseFrontMatter([]byte(input))
	if err == nil {
		t.Fatal("expected error for missing closing ---")
	}
	if !strings.Contains(err.Error(), "missing closing") {
		t.Errorf("error = %q, want mention of missing closing", err.Error())
	}
}

func TestParseFrontMatter_EmptyBody(t *testing.T) {
	input := "---\ntitle: \"No Body\"\ndate: 2026-06-01\n---\n"
	fm, body, err := ParseFrontMatter([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm == nil {
		t.Fatal("expected front matter, got nil")
	}
	if fm.Title != "No Body" {
		t.Errorf("title = %q, want %q", fm.Title, "No Body")
	}
	if len(body) != 0 {
		t.Errorf("expected empty body, got %q", string(body))
	}
}

func TestParseFrontMatter_Tags(t *testing.T) {
	input := `---
title: "Tagged"
date: 2026-01-01
tags: ["alpha", "beta", "gamma"]
categories: ["one", "two"]
---
Body.
`
	fm, _, err := ParseFrontMatter([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fm.Tags) != 3 {
		t.Fatalf("expected 3 tags, got %d: %v", len(fm.Tags), fm.Tags)
	}
	want := []string{"alpha", "beta", "gamma"}
	for i, tag := range fm.Tags {
		if tag != want[i] {
			t.Errorf("tag[%d] = %q, want %q", i, tag, want[i])
		}
	}
	if len(fm.Categories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(fm.Categories))
	}
}

func TestParseFrontMatter_HorizontalRule(t *testing.T) {
	data := []byte("----\nNot front matter")
	fm, body, err := ParseFrontMatter(data)
	if err != nil {
		t.Fatal(err)
	}
	if fm != nil {
		t.Error("---- should not be parsed as front matter")
	}
	if string(body) != string(data) {
		t.Error("body should be original data")
	}
}

func TestParseFrontMatter_ClosingDelimiterInYAML(t *testing.T) {
	data := []byte("---\ntitle: \"has---inside\"\ndescription: \"test\"\n---\nbody")
	fm, body, err := ParseFrontMatter(data)
	if err != nil {
		t.Fatal(err)
	}
	if fm == nil {
		t.Fatal("expected front matter")
	}
	if fm.Title != "has---inside" {
		t.Errorf("title = %q", fm.Title)
	}
	if !bytes.HasPrefix(body, []byte("body")) {
		t.Errorf("body = %q", body)
	}
}

func TestParseFrontMatter_SizeLimit(t *testing.T) {
	huge := make([]byte, 0, 70*1024)
	huge = append(huge, []byte("---\ntitle: ")...)
	huge = append(huge, bytes.Repeat([]byte("x"), 65*1024)...)
	huge = append(huge, []byte("\n---\nbody")...)
	_, _, err := ParseFrontMatter(huge)
	if err == nil {
		t.Fatal("expected size limit error")
	}
}

func TestReadingTimeMinutes(t *testing.T) {
	tests := []struct {
		name     string
		words    int
		wantMins int
	}{
		{"short_post", 50, 1},
		{"one_minute", 200, 1},
		{"two_minutes", 400, 2},
		{"five_minutes", 1000, 5},
		{"long_post", 3000, 15},
		{"empty", 0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := strings.Repeat("word ", tt.words)
			got := ReadingTimeMinutes(text)
			if got != tt.wantMins {
				t.Errorf("ReadingTimeMinutes(%d words) = %d, want %d", tt.words, got, tt.wantMins)
			}
		})
	}
}
