package utils

import (
	"encoding/base64"
	"net/http"
	"slices"
	"strings"
)

func GetStreamUrl(slug string) string {
	return base64.URLEncoding.EncodeToString([]byte(slug))
}

func GetStreamSlugFromUrl(streamUID string) string {
	decoded, err := base64.URLEncoding.DecodeString(streamUID)
	if err != nil {
		return ""
	}
	return string(decoded)
}

func IsPlaylist(resp *http.Response) bool {
	knownMimeTypes := []string{
		"application/x-mpegurl",
		"text/plain",
		"audio/x-mpegurl",
		"audio/mpegurl",
		"application/vnd.apple.mpegurl",
	}

	return slices.Contains(knownMimeTypes, strings.ToLower(resp.Header.Get("Content-Type")))
}
