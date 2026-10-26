package play

import (
	"strings"
	"testing"
)

func TestHttpsLinksPlayAsTheyAre(t *testing.T) {
	got, err := Resolve("https://example.com/films/a.mp4?sig=1", "")
	if err != nil || got != "https://example.com/films/a.mp4?sig=1" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestOtherReferencesNeedAPresignCommand(t *testing.T) {
	if _, err := Resolve("films/a.mp4", ""); err == nil || !strings.Contains(err.Error(), "AGUATV_PRESIGN") {
		t.Fatalf("expected a pointer to AGUATV_PRESIGN, got %v", err)
	}
	got, err := Resolve("films/it's;a.mp4", "printf 'https://example.com/signed/%s\\n'")
	if err != nil || got != "https://example.com/signed/films/it's;a.mp4" {
		t.Fatalf("the reference should arrive as one last argument, got %q, %v", got, err)
	}
	if _, err := Resolve("films/a.mp4", "echo http://example.com/a.mp4"); err == nil {
		t.Fatal("a non-https answer should be refused")
	}
	got, err = Resolve("https://example.com/page/a.mp4", "printf 'https://example.com/signed\\n'")
	if err != nil || got != "https://example.com/signed" {
		t.Fatalf("with AGUATV_PRESIGN set, URLs go through it too: %q %v", got, err)
	}
	if _, err := Resolve("films/a.mp4", "false"); err == nil {
		t.Fatal("a failing presign command should be an error")
	}
}

func TestTitleAndTypeIgnoreTheQuery(t *testing.T) {
	ref := "https://example.com/films/talk.mkv?X-Amz-Signature=abc"
	if Title(ref) != "talk.mkv" || Type(ref) != "video/x-matroska" {
		t.Fatalf("got %q %q", Title(ref), Type(ref))
	}
	if Title("s3://bucket/films/talk.mp4") != "talk.mp4" || Type("films/x") != "video/*" {
		t.Fatal("bare and s3 references")
	}
}

func TestPlayCommandQuotesTheLink(t *testing.T) {
	cmd := Command("https://example.com/a.mp4?X-Amz-Signature=abc&X-Amz-Date=1", "it's.mp4", "video/mp4")
	if !strings.Contains(cmd, "-d 'https://example.com/a.mp4?X-Amz-Signature=abc&X-Amz-Date=1'") {
		t.Fatalf("link not quoted: %s", cmd)
	}
	if !strings.Contains(cmd, `--es title 'it'\''s.mp4'`) {
		t.Fatalf("title not quoted: %s", cmd)
	}
}
