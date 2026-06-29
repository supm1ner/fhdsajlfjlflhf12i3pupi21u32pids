package account

import (
	"bytes"
	"errors"
)

// Image upload bounds (design.md D2). The caps keep the Postgres blob rows small;
// the content-type is validated by sniffing the magic bytes rather than trusting
// the multipart Content-Type header (which the client controls).
const (
	// MaxAvatarBytes caps an avatar upload at 512 KiB.
	MaxAvatarBytes = 512 * 1024
	// MaxBannerBytes caps a banner upload at 1 MiB.
	MaxBannerBytes = 1024 * 1024
)

// Image kinds.
const (
	KindAvatar = "avatar"
	KindBanner = "banner"
)

// Image validation errors (mapped to 400/413/415 by the handlers).
var (
	// ErrImageEmpty is returned for a zero-length upload.
	ErrImageEmpty = errors.New("image is empty")
	// ErrImageTooLarge is returned when the upload exceeds the kind's size cap.
	ErrImageTooLarge = errors.New("image is too large")
	// ErrImageUnsupportedType is returned when the bytes are not a supported image.
	ErrImageUnsupportedType = errors.New("image type is not supported (allowed: png, jpeg, webp)")
	// ErrUnknownImageKind is returned for an image kind other than avatar/banner.
	ErrUnknownImageKind = errors.New("unknown image kind")
)

// MaxBytesForKind returns the upload size cap for an image kind, or 0 for an
// unknown kind.
func MaxBytesForKind(kind string) int {
	switch kind {
	case KindAvatar:
		return MaxAvatarBytes
	case KindBanner:
		return MaxBannerBytes
	default:
		return 0
	}
}

// ValidKind reports whether kind is a supported profile-image kind.
func ValidKind(kind string) bool {
	return kind == KindAvatar || kind == KindBanner
}

// SniffContentType inspects the leading magic bytes of data and returns the
// canonical content type for the supported image formats, or "" when the data is
// not a recognized PNG/JPEG/WebP. This deliberately does NOT trust any
// client-supplied Content-Type: the byte signature is authoritative.
//
//	PNG : 89 50 4E 47 0D 0A 1A 0A
//	JPEG: FF D8 FF
//	WebP: "RIFF" .... "WEBP" (RIFF container with a WEBP fourCC at offset 8)
func SniffContentType(data []byte) string {
	switch {
	case len(data) >= 8 && bytes.Equal(data[:8], pngMagic):
		return "image/png"
	case len(data) >= 3 && bytes.Equal(data[:3], jpegMagic):
		return "image/jpeg"
	case len(data) >= 12 && bytes.Equal(data[:4], riffMagic) && bytes.Equal(data[8:12], webpMagic):
		return "image/webp"
	default:
		return ""
	}
}

var (
	pngMagic  = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
	riffMagic = []byte("RIFF")
	webpMagic = []byte("WEBP")
)

// ValidateImage enforces the size cap and the magic-byte content-type allow-list
// for an uploaded image of the given kind. On success it returns the canonical
// content type (sniffed from the bytes) to store and serve.
func ValidateImage(kind string, data []byte) (contentType string, err error) {
	if !ValidKind(kind) {
		return "", ErrUnknownImageKind
	}
	if len(data) == 0 {
		return "", ErrImageEmpty
	}
	if len(data) > MaxBytesForKind(kind) {
		return "", ErrImageTooLarge
	}
	ct := SniffContentType(data)
	if ct == "" {
		return "", ErrImageUnsupportedType
	}
	return ct, nil
}
