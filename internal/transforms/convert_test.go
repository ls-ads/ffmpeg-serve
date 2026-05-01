package transforms

import (
	"strings"
	"testing"
)

func TestFormatKind(t *testing.T) {
	cases := map[string]MediaKind{
		"jpg":   KindImage,
		"jpeg":  KindImage,
		"png":   KindImage,
		"webp":  KindImage,
		"avif":  KindImage,
		"mp4":   KindVideo,
		"mov":   KindVideo,
		"webm":  KindVideo,
		"mkv":   KindVideo,
		"gif":   KindVideo, // animated by default
		"mp3":   KindAudio,
		"aac":   KindAudio,
		"m4a":   KindAudio,
		"flac":  KindAudio,
		"wav":   KindAudio,
		"opus":  KindAudio,
		"ogg":   KindAudio,
		"xyz":   KindUnknown,
	}
	for in, want := range cases {
		if got := formatKind(in); got != want {
			t.Errorf("formatKind(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestResolveConvertTarget(t *testing.T) {
	cases := []struct {
		name     string
		kind     MediaKind
		target   string
		want     string
		errSub   string
	}{
		{"image → image stays", KindImage, "webp", "webp", ""},
		{"image → jpeg canonical", KindImage, "jpeg", "jpg", ""},
		{"image → audio rejected", KindImage, "mp3", "", "cannot convert"},
		{"image → video rejected", KindImage, "mp4", "", "cannot convert"},
		{"audio → audio stays", KindAudio, "mp3", "mp3", ""},
		{"audio → mp4 aliased to m4a", KindAudio, "mp4", "m4a", ""},
		{"audio → image rejected", KindAudio, "jpg", "", "cannot convert"},
		{"audio → video (gif) rejected", KindAudio, "gif", "", "cannot convert"},
		{"video → video stays", KindVideo, "webm", "webm", ""},
		{"video → gif fine", KindVideo, "gif", "gif", ""},
		{"video → audio rejected", KindVideo, "mp3", "", "cannot convert"},
		{"unknown input rejected", KindUnknown, "mp4", "", "could not classify"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveConvertTarget(tc.kind, tc.target)
			if tc.errSub != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tc.errSub)
				}
				if !strings.Contains(err.Error(), tc.errSub) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.errSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildConvertImage_KnownTargets(t *testing.T) {
	job := &stagedJob{inPath: "/tmp/in.png", outPath: "/tmp/out"}
	cases := map[string]string{
		"jpg":  "-q:v",
		"webp": "libwebp",
		"avif": "libaom-av1",
		"png":  "-compression_level",
	}
	for target, expectFragment := range cases {
		t.Run(target, func(t *testing.T) {
			args := buildConvertImage(target, job)
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, expectFragment) {
				t.Errorf("target %q: expected arg fragment %q, got: %s",
					target, expectFragment, joined)
			}
		})
	}
}

func TestBuildConvertAudio_KnownTargets(t *testing.T) {
	job := &stagedJob{inPath: "/tmp/in.wav", outPath: "/tmp/out"}
	cases := map[string][]string{
		"mp3":  {"libmp3lame", "192k"},
		"aac":  {"-c:a aac", "192k"},
		"m4a":  {"-c:a aac", "192k"},
		"opus": {"libopus", "128k"},
		"ogg":  {"libvorbis"},
		"flac": {"flac"},
		"wav":  {"pcm_s16le"},
	}
	for target, fragments := range cases {
		t.Run(target, func(t *testing.T) {
			args := buildConvertAudio(target, job)
			joined := strings.Join(args, " ")
			for _, f := range fragments {
				if !strings.Contains(joined, f) {
					t.Errorf("target %q missing %q. got: %s", target, f, joined)
				}
			}
			if !strings.Contains(joined, "-vn") {
				t.Errorf("audio convert should strip cover-art with -vn. got: %s", joined)
			}
		})
	}
}

func TestBuildConvertVideo_KnownTargets(t *testing.T) {
	job := &stagedJob{inPath: "/tmp/in.mp4", outPath: "/tmp/out"}
	probed := &ffprobeResult{}
	probed.Format.FormatName = "mov,mp4,m4a,3gp,3g2,mj2"

	cases := []struct {
		target    string
		fragments []string
	}{
		{"mp4", []string{"h264_nvenc", "+faststart"}},
		{"mov", []string{"h264_nvenc", "+faststart"}},
		{"webm", []string{"libvpx-vp9", "libopus"}},
		{"mkv", []string{"h264_nvenc"}},
		{"gif", []string{"palettegen", "paletteuse", "fps=15"}},
		{"avi", []string{"mpeg4", "ac3"}}, // LGPL-clean fallback
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			args := buildConvertVideo(tc.target, job, probed)
			joined := strings.Join(args, " ")
			for _, f := range tc.fragments {
				if !strings.Contains(joined, f) {
					t.Errorf("target %q missing %q. got: %s", tc.target, f, joined)
				}
			}
		})
	}
}

func TestParseConvertParams(t *testing.T) {
	if _, err := parseConvertParams(map[string]any{"to": 42}); err == nil {
		t.Error("expected error on non-string `to`")
	}
	got, err := parseConvertParams(map[string]any{"to": "webm"})
	if err != nil {
		t.Fatal(err)
	}
	if got.To != "webm" {
		t.Errorf("got %+v, want To=webm", got)
	}
}
