package account

import (
	"bytes"
	"errors"
	"testing"
)

func TestSniffContentType(t *testing.T) {
	t.Parallel()
	png := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, 0x00, 0x01)
	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, 0x00)
	webp := append(append([]byte("RIFF"), 0x10, 0x00, 0x00, 0x00), []byte("WEBPVP8 ")...)
	gif := []byte("GIF89a")

	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"png", png, "image/png"},
		{"jpeg", jpeg, "image/jpeg"},
		{"webp", webp, "image/webp"},
		{"gif rejected", gif, ""},
		{"empty", nil, ""},
		{"too short png prefix", []byte{0x89, 0x50}, ""},
		// RIFF container that is NOT a WebP (e.g. a WAV) must be rejected.
		{"riff non-webp", append(append([]byte("RIFF"), 0x10, 0x00, 0x00, 0x00), []byte("WAVEfmt ")...), ""},
	}
	for _, c := range cases {
		if got := SniffContentType(c.data); got != c.want {
			t.Errorf("%s: SniffContentType = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestValidateImage(t *testing.T) {
	t.Parallel()
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	// A valid small PNG.
	smallPNG := append(append([]byte{}, pngHeader...), bytes.Repeat([]byte{0x00}, 64)...)
	ct, err := ValidateImage(KindAvatar, smallPNG)
	if err != nil {
		t.Fatalf("valid avatar PNG: unexpected err %v", err)
	}
	if ct != "image/png" {
		t.Errorf("content type = %q, want image/png", ct)
	}

	// Unknown kind.
	if _, err := ValidateImage("hero", smallPNG); !errors.Is(err, ErrUnknownImageKind) {
		t.Errorf("unknown kind err = %v, want ErrUnknownImageKind", err)
	}

	// Empty.
	if _, err := ValidateImage(KindAvatar, nil); !errors.Is(err, ErrImageEmpty) {
		t.Errorf("empty err = %v, want ErrImageEmpty", err)
	}

	// Unsupported type (valid size, bad magic bytes).
	if _, err := ValidateImage(KindAvatar, []byte("GIF89a-not-an-image")); !errors.Is(err, ErrImageUnsupportedType) {
		t.Errorf("unsupported type err = %v, want ErrImageUnsupportedType", err)
	}

	// Over the avatar cap (512 KiB): valid PNG header but too many bytes.
	overAvatar := append(append([]byte{}, pngHeader...), bytes.Repeat([]byte{0x00}, MaxAvatarBytes)...)
	if _, err := ValidateImage(KindAvatar, overAvatar); !errors.Is(err, ErrImageTooLarge) {
		t.Errorf("oversize avatar err = %v, want ErrImageTooLarge", err)
	}

	// The same byte count is under the banner cap (1 MiB), so it is accepted as a
	// banner — proving the cap is per-kind.
	if _, err := ValidateImage(KindBanner, overAvatar); err != nil {
		t.Errorf("banner just over avatar cap should pass, got %v", err)
	}
}

func TestMaxBytesForKind(t *testing.T) {
	t.Parallel()
	if MaxBytesForKind(KindAvatar) != MaxAvatarBytes {
		t.Errorf("avatar cap = %d, want %d", MaxBytesForKind(KindAvatar), MaxAvatarBytes)
	}
	if MaxBytesForKind(KindBanner) != MaxBannerBytes {
		t.Errorf("banner cap = %d, want %d", MaxBytesForKind(KindBanner), MaxBannerBytes)
	}
	if MaxBytesForKind("nope") != 0 {
		t.Errorf("unknown kind cap = %d, want 0", MaxBytesForKind("nope"))
	}
}
