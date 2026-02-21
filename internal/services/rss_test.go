package services

import "testing"

func TestParseRSSFeedData_ExtractsEpisodeArtworkIndependently(t *testing.T) {
	const feed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"
  xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"
  xmlns:media="http://search.yahoo.com/mrss/"
  xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>Podcast Test</title>
    <itunes:image href="https://cdn.example.com/podcast-cover.jpg" />

    <item>
      <title>Episode 1</title>
      <itunes:image href="https://cdn.example.com/ep-1.jpg" />
      <enclosure url="https://cdn.example.com/ep-1.mp3" type="audio/mpeg" />
      <guid>https://example.com/ep-1</guid>
    </item>

    <item>
      <title>Episode 2</title>
      <media:thumbnail url="https://cdn.example.com/ep-2.jpg" />
      <enclosure url="https://cdn.example.com/ep-2.mp3" type="audio/mpeg" />
      <guid>https://example.com/ep-2</guid>
    </item>

    <item>
      <title>Episode 3</title>
      <media:content url="https://cdn.example.com/ep-3.webp" medium="image" />
      <media:content url="https://cdn.example.com/ep-3.mp3" medium="audio" />
      <guid>https://example.com/ep-3</guid>
    </item>

    <item>
      <title>Episode 4</title>
      <image>
        <url>https://cdn.example.com/ep-4.png</url>
      </image>
      <enclosure url="https://cdn.example.com/ep-4.mp3" type="audio/mpeg" />
      <guid>https://example.com/ep-4</guid>
    </item>

    <item>
      <title>Episode 5</title>
      <enclosure url="https://cdn.example.com/ep-5.mp3" type="audio/mpeg" />
      <guid>https://example.com/ep-5</guid>
    </item>
  </channel>
</rss>`

	parsed, err := parseRSSFeedData([]byte(feed))
	if err != nil {
		t.Fatalf("parseRSSFeedData returned error: %v", err)
	}

	if parsed.PodcastArtworkURL != "https://cdn.example.com/podcast-cover.jpg" {
		t.Fatalf("unexpected podcast artwork: %q", parsed.PodcastArtworkURL)
	}

	if len(parsed.Episodes) != 5 {
		t.Fatalf("unexpected episode count: %d", len(parsed.Episodes))
	}

	assertEpisodeArtwork := func(index int, expected string) {
		t.Helper()
		if got := parsed.Episodes[index].ArtworkURL; got != expected {
			t.Fatalf("episode[%d] artwork mismatch: got %q want %q", index, got, expected)
		}
	}

	assertEpisodeArtwork(0, "https://cdn.example.com/ep-1.jpg")
	assertEpisodeArtwork(1, "https://cdn.example.com/ep-2.jpg")
	assertEpisodeArtwork(2, "https://cdn.example.com/ep-3.webp")
	assertEpisodeArtwork(3, "https://cdn.example.com/ep-4.png")
	assertEpisodeArtwork(4, "")
}
