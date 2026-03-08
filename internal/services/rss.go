package services

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"21loader-cross/internal/core"
)

type RSSService struct {
	client *http.Client
}

func NewRSSService() *RSSService {
	return &RSSService{client: &http.Client{Timeout: 20 * time.Second}}
}

func (s *RSSService) FetchFeed(ctx context.Context, feedURL string) (core.RSSFeedEpisodesAPIResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(feedURL), nil)
	if err != nil {
		return core.RSSFeedEpisodesAPIResponse{}, fmt.Errorf("URL du flux RSS invalide")
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return core.RSSFeedEpisodesAPIResponse{}, fmt.Errorf("impossible de recuperer le flux RSS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return core.RSSFeedEpisodesAPIResponse{}, fmt.Errorf("impossible de recuperer le flux RSS: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return core.RSSFeedEpisodesAPIResponse{}, fmt.Errorf("lecture du flux RSS echouee")
	}

	out, err := parseRSSFeedData(data)
	if err != nil {
		return core.RSSFeedEpisodesAPIResponse{}, fmt.Errorf("le flux RSS n'a pas pu etre analyse")
	}
	return out, nil
}

const (
	itunesNamespace = "http://www.itunes.com/dtds/podcast-1.0.dtd"
	mediaNamespace  = "http://search.yahoo.com/mrss/"
	dcNamespace     = "http://purl.org/dc/elements/1.1/"
)

type rssParserState struct {
	channelTitle         string
	channelArtworkURL    string
	isInsideChannelImage bool
	currentItem          *rssItemDraft
	episodes             []core.RSSEpisodeDTO
	buffer               strings.Builder
}

type rssItemDraft struct {
	title           string
	publicationDate *time.Time
	enclosureURL    string
	mediaContentURL string
	link            string
	guid            string
	itemArtworkURL  string
	isInsideImage   bool
}

func parseRSSFeedData(data []byte) (core.RSSFeedEpisodesAPIResponse, error) {
	state := rssParserState{channelTitle: "Podcast"}
	decoder := xml.NewDecoder(bytes.NewReader(data))

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return core.RSSFeedEpisodesAPIResponse{}, err
		}

		switch t := token.(type) {
		case xml.StartElement:
			name := normalizeRSSElementName(t.Name)
			if name == "item" {
				state.currentItem = &rssItemDraft{
					title: "Episode",
				}
				state.buffer.Reset()
				continue
			}

			if state.currentItem == nil {
				handleRSSChannelStart(&state, name, t.Attr)
			} else {
				handleRSSItemStart(state.currentItem, name, t.Attr)
			}
			state.buffer.Reset()

		case xml.CharData:
			state.buffer.Write(t)

		case xml.EndElement:
			name := normalizeRSSElementName(t.Name)
			value := strings.TrimSpace(state.buffer.String())
			if state.currentItem == nil {
				handleRSSChannelEnd(&state, name, value)
			} else {
				handleRSSItemEnd(&state, name, value)
			}
			state.buffer.Reset()
		}
	}

	channelTitle := strings.TrimSpace(state.channelTitle)
	if channelTitle == "" {
		channelTitle = "Podcast"
	}

	return core.RSSFeedEpisodesAPIResponse{
		PodcastTitle:      channelTitle,
		PodcastArtworkURL: strings.TrimSpace(state.channelArtworkURL),
		Episodes:          state.episodes,
	}, nil
}

func handleRSSChannelStart(state *rssParserState, name string, attributes []xml.Attr) {
	switch name {
	case "image":
		state.isInsideChannelImage = true
	case "itunes:image":
		if href := attrValue(attributes, "href"); href != "" {
			state.channelArtworkURL = href
		}
	case "media:thumbnail":
		if u := attrValue(attributes, "url"); u != "" {
			state.channelArtworkURL = u
		}
	case "media:content", "content":
		u := attrValue(attributes, "url")
		if u == "" {
			return
		}
		if looksLikeImage(u, attrValue(attributes, "type"), attrValue(attributes, "medium")) {
			state.channelArtworkURL = u
		}
	}
}

func handleRSSChannelEnd(state *rssParserState, name string, value string) {
	switch name {
	case "title":
		if strings.TrimSpace(value) != "" && strings.TrimSpace(state.channelTitle) == "Podcast" {
			state.channelTitle = value
		}
	case "url":
		if state.isInsideChannelImage && strings.TrimSpace(value) != "" {
			state.channelArtworkURL = value
		}
	case "image":
		state.isInsideChannelImage = false
	}
}

func handleRSSItemStart(item *rssItemDraft, name string, attributes []xml.Attr) {
	switch name {
	case "image":
		item.isInsideImage = true
	case "itunes:image":
		if item.itemArtworkURL == "" {
			if href := attrValue(attributes, "href"); href != "" {
				item.itemArtworkURL = href
			}
		}
	case "media:thumbnail":
		if item.itemArtworkURL == "" {
			if u := attrValue(attributes, "url"); u != "" {
				item.itemArtworkURL = u
			}
		}
	case "media:content", "content":
		u := attrValue(attributes, "url")
		if u == "" {
			return
		}
		if looksLikeImage(u, attrValue(attributes, "type"), attrValue(attributes, "medium")) {
			if item.itemArtworkURL == "" {
				item.itemArtworkURL = u
			}
			return
		}
		if item.mediaContentURL == "" {
			item.mediaContentURL = u
		}
	case "enclosure":
		u := attrValue(attributes, "url")
		if u == "" {
			return
		}
		if !looksLikeImage(u, attrValue(attributes, "type"), "") && item.enclosureURL == "" {
			item.enclosureURL = u
		}
	}
}

func handleRSSItemEnd(state *rssParserState, name string, value string) {
	item := state.currentItem
	if item == nil {
		return
	}

	switch name {
	case "title":
		if strings.TrimSpace(value) != "" {
			item.title = value
		}
	case "pubdate", "dc:date":
		if date := parseRSSDate(value); date != nil && item.publicationDate == nil {
			item.publicationDate = date
		}
	case "link":
		if strings.TrimSpace(value) != "" {
			item.link = value
		}
	case "guid":
		if strings.TrimSpace(value) != "" {
			item.guid = value
		}
	case "url":
		if item.isInsideImage && strings.TrimSpace(value) != "" && item.itemArtworkURL == "" {
			item.itemArtworkURL = value
		}
	case "itunes:image":
		if item.itemArtworkURL == "" && strings.TrimSpace(value) != "" {
			item.itemArtworkURL = value
		}
	case "image":
		item.isInsideImage = false
	case "item":
		title := strings.TrimSpace(item.title)
		if title == "" {
			title = "Episode"
		}
		mediaURL := firstNonEmpty(item.enclosureURL, item.mediaContentURL, item.guid)
		fallback := firstNonEmpty(item.link, item.guid)
		artwork := strings.TrimSpace(item.itemArtworkURL)
		seed := fmt.Sprintf("%s|%d|%s|%s", title, len(state.episodes), mediaURL, fallback)
		state.episodes = append(state.episodes, core.RSSEpisodeDTO{
			ID:              seed,
			Title:           title,
			PublicationDate: item.publicationDate,
			MediaURL:        mediaURL,
			FallbackLink:    fallback,
			ArtworkURL:      artwork,
		})
		state.currentItem = nil
	}
}

func normalizeRSSElementName(name xml.Name) string {
	local := strings.ToLower(strings.TrimSpace(name.Local))
	space := strings.TrimSpace(name.Space)
	spaceLower := strings.ToLower(space)
	if strings.Contains(local, ":") {
		return local
	}

	switch {
	case local == "image" && (space == itunesNamespace || spaceLower == strings.ToLower(itunesNamespace) || spaceLower == "itunes"):
		return "itunes:image"
	case local == "thumbnail" && (space == mediaNamespace || spaceLower == strings.ToLower(mediaNamespace) || spaceLower == "media"):
		return "media:thumbnail"
	case local == "content" && (space == mediaNamespace || spaceLower == strings.ToLower(mediaNamespace) || spaceLower == "media"):
		return "media:content"
	case local == "date" && (space == dcNamespace || spaceLower == strings.ToLower(dcNamespace) || spaceLower == "dc"):
		return "dc:date"
	default:
		return local
	}
}

func attrValue(attributes []xml.Attr, key string) string {
	needle := strings.ToLower(strings.TrimSpace(key))
	for _, attribute := range attributes {
		if strings.ToLower(strings.TrimSpace(attribute.Name.Local)) == needle {
			return strings.TrimSpace(attribute.Value)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func looksLikeImage(url, mimeType, medium string) bool {
	mt := strings.ToLower(strings.TrimSpace(mimeType))
	if strings.HasPrefix(mt, "image/") {
		return true
	}
	if strings.ToLower(strings.TrimSpace(medium)) == "image" {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(url))
	base := lower
	if idx := strings.IndexAny(base, "?#"); idx >= 0 {
		base = base[:idx]
	}
	return strings.HasSuffix(base, ".jpg") ||
		strings.HasSuffix(base, ".jpeg") ||
		strings.HasSuffix(base, ".png") ||
		strings.HasSuffix(base, ".webp") ||
		strings.HasSuffix(base, ".gif") ||
		strings.HasSuffix(base, ".avif") ||
		strings.HasSuffix(base, ".heic") ||
		strings.HasSuffix(base, ".heif")
}

func parseRSSDate(v string) *time.Time {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02",
	}
	for _, f := range formats {
		t, err := time.Parse(f, v)
		if err == nil {
			u := t.UTC()
			return &u
		}
	}
	return nil
}
