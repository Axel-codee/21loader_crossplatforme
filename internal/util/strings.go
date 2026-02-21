package util

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

var spaceRe = regexp.MustCompile(`\s+`)

func Trim(v string) string {
	return strings.TrimSpace(v)
}

func NonEmptyTrimmed(v string) (string, bool) {
	t := strings.TrimSpace(v)
	if t == "" {
		return "", false
	}
	return t, true
}

func SanitizePathComponent(value string, maxLength int) string {
	if maxLength <= 0 {
		maxLength = 96
	}
	clean := strings.Map(func(r rune) rune {
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*', '\n', '\r', '\t':
			return '_'
		default:
			if unicode.IsControl(r) {
				return '_'
			}
			return r
		}
	}, value)
	clean = spaceRe.ReplaceAllString(clean, " ")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		clean = "item"
	}
	runes := []rune(clean)
	if len(runes) > maxLength {
		clean = string(runes[:maxLength])
	}
	return clean
}

func LooksLikeYouTubeURL(value string) bool {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u.Host == "" {
		return false
	}
	h := strings.ToLower(u.Host)
	return strings.Contains(h, "youtube.com") || strings.Contains(h, "youtu.be")
}

func LooksLikeYouTubeCollectionURL(value string) bool {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u.Host == "" {
		return false
	}
	h := strings.ToLower(u.Host)
	if strings.Contains(h, "youtu.be") {
		return false
	}
	p := strings.ToLower(u.Path)
	q := u.Query()
	if strings.HasPrefix(p, "/@") || strings.HasPrefix(p, "/channel/") || strings.HasPrefix(p, "/c/") || strings.HasPrefix(p, "/user/") {
		return true
	}
	if p == "/watch" {
		return false
	}
	if p == "/playlist" || q.Get("list") != "" {
		return true
	}
	return false
}

func LooksLikeRSSURL(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(lower, "/feed") || strings.Contains(lower, "rss") || strings.HasSuffix(lower, ".xml")
}

type QobuzResourceType string

const (
	QobuzAlbum    QobuzResourceType = "album"
	QobuzArtist   QobuzResourceType = "artist"
	QobuzTrack    QobuzResourceType = "track"
	QobuzPlaylist QobuzResourceType = "playlist"
	QobuzLabel    QobuzResourceType = "label"
	QobuzUnknown  QobuzResourceType = "unknown"
)

func QobuzResourceTypeFromURL(value string) (QobuzResourceType, bool) {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u.Host == "" {
		return QobuzUnknown, false
	}
	if !strings.Contains(strings.ToLower(u.Host), "qobuz.com") {
		return QobuzUnknown, false
	}
	parts := strings.Split(strings.Trim(strings.ToLower(u.Path), "/"), "/")
	for _, p := range parts {
		switch p {
		case "album":
			return QobuzAlbum, true
		case "artist":
			return QobuzArtist, true
		case "track":
			return QobuzTrack, true
		case "playlist":
			return QobuzPlaylist, true
		case "label":
			return QobuzLabel, true
		}
	}
	return QobuzUnknown, true
}

func QobuzResourceIdentifier(value string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u.Host == "" {
		return "", false
	}
	if !strings.Contains(strings.ToLower(u.Host), "qobuz.com") {
		return "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, p := range parts {
		switch strings.ToLower(p) {
		case "album", "artist", "track", "playlist", "label":
			if i+1 >= len(parts) {
				return "", false
			}
			id := strings.TrimSpace(parts[len(parts)-1])
			if id == "" {
				return "", false
			}
			return id, true
		}
	}
	return "", false
}
