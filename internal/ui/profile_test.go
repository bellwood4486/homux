package ui

import (
	"bytes"
	"testing"
)

func TestRenderProfileList_MatchesSpecExample(t *testing.T) {
	var out bytes.Buffer
	RenderProfileList(&out, []string{"work", "personal"}, "work")

	want := "Profiles:\n\n" +
		"  personal\n" +
		"* work\n\n" +
		"Active profile: work\n"
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
}

func TestRenderProfileList_NoActiveProfile(t *testing.T) {
	var out bytes.Buffer
	RenderProfileList(&out, []string{"work", "personal"}, "")

	want := "Profiles:\n\n" +
		"  personal\n" +
		"  work\n\n" +
		"Active profile: (none)\n"
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
}

func TestRenderProfileList_NoProfilesDefined(t *testing.T) {
	var out bytes.Buffer
	RenderProfileList(&out, nil, "")

	want := "Profiles:\n\n" +
		"  (none)\n\n" +
		"Active profile: (none)\n"
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
}

func TestRenderProfileSwitch_ApplyNeeded(t *testing.T) {
	var out bytes.Buffer
	RenderProfileSwitch(&out, "personal", "work", true)

	want := "Active profile: personal -> work\n\n" +
		"Run \"homux apply\" to update HOME.\n"
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
}

func TestRenderProfileSwitch_NoApplyNeeded(t *testing.T) {
	var out bytes.Buffer
	RenderProfileSwitch(&out, "", "work", false)

	want := "Active profile: (none) -> work\n\n" +
		"HOME already matches this profile.\n"
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
}
