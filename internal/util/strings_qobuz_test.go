package util

import "testing"

func TestQobuzResourceTypeFromURLPlaylist(t *testing.T) {
	rt, ok := QobuzResourceTypeFromURL("https://play.qobuz.com/playlist/57551803")
	if !ok {
		t.Fatalf("expected qobuz URL to be accepted")
	}
	if rt != QobuzPlaylist {
		t.Fatalf("unexpected resource type: got=%q want=%q", rt, QobuzPlaylist)
	}
}

func TestQobuzResourceTypeFromURLArtistWithLocalePath(t *testing.T) {
	rt, ok := QobuzResourceTypeFromURL("https://play.qobuz.com/fr-fr/artist/12345")
	if !ok {
		t.Fatalf("expected qobuz URL to be accepted")
	}
	if rt != QobuzArtist {
		t.Fatalf("unexpected resource type: got=%q want=%q", rt, QobuzArtist)
	}
}

func TestQobuzResourceIdentifierFromAlbumSlugURL(t *testing.T) {
	id, ok := QobuzResourceIdentifier("https://play.qobuz.com/album/the-hypnoflip-invasion/3700398716473")
	if !ok {
		t.Fatalf("expected album URL identifier to be extracted")
	}
	if id != "3700398716473" {
		t.Fatalf("unexpected album identifier: got=%q", id)
	}
}

func TestQobuzResourceIdentifierFromPlaylistURL(t *testing.T) {
	id, ok := QobuzResourceIdentifier("https://play.qobuz.com/playlist/57551803")
	if !ok {
		t.Fatalf("expected playlist URL identifier to be extracted")
	}
	if id != "57551803" {
		t.Fatalf("unexpected playlist identifier: got=%q", id)
	}
}
