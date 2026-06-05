package feed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const rssFixture = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Example RSS</title>
    <link>https://example.com</link>
    <description>An &lt;b&gt;example&lt;/b&gt; feed</description>
    <language>pt-BR</language>
    <item>
      <title>First post</title>
      <link>https://example.com/1</link>
      <guid>https://example.com/1</guid>
      <description>&lt;p&gt;Hello &lt;strong&gt;world&lt;/strong&gt;&lt;/p&gt;</description>
      <pubDate>Mon, 02 Jan 2006 15:04:05 GMT</pubDate>
    </item>
    <item>
      <title>Second post</title>
      <link>https://example.com/2</link>
      <description>Plain text</description>
      <pubDate>Tue, 03 Jan 2006 15:04:05 GMT</pubDate>
    </item>
  </channel>
</rss>`

const atomFixture = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Example Atom</title>
  <link href="https://example.com/atom"/>
  <updated>2006-01-02T15:04:05Z</updated>
  <author><name>Jane Doe</name></author>
  <entry>
    <title>Atom entry</title>
    <link href="https://example.com/atom/1"/>
    <id>urn:uuid:1</id>
    <updated>2006-01-02T15:04:05Z</updated>
    <published>2006-01-02T15:04:05Z</published>
    <summary>Atom summary</summary>
  </entry>
</feed>`

const jsonFeedFixture = `{
  "version": "https://jsonfeed.org/version/1.1",
  "title": "Example JSON Feed",
  "home_page_url": "https://example.com/json",
  "items": [
    { "id": "1", "url": "https://example.com/json/1", "title": "JSON item", "content_html": "<p>Body</p>", "summary": "A summary", "date_published": "2006-01-02T15:04:05Z" }
  ]
}`

const podcastFixture = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
  <channel>
    <title>Example Podcast</title>
    <link>https://example.com/podcast</link>
    <description>A show</description>
    <itunes:author>The Host</itunes:author>
    <itunes:explicit>no</itunes:explicit>
    <itunes:owner><itunes:name>Owner Name</itunes:name><itunes:email>owner@example.com</itunes:email></itunes:owner>
    <itunes:category text="Technology"/>
    <item>
      <title>Episode 1</title>
      <link>https://example.com/podcast/1</link>
      <description>First episode</description>
      <pubDate>Mon, 02 Jan 2006 15:04:05 GMT</pubDate>
      <enclosure url="https://example.com/audio/1.mp3" length="123456" type="audio/mpeg"/>
      <itunes:duration>00:42:10</itunes:duration>
      <itunes:episode>1</itunes:episode>
      <itunes:season>1</itunes:season>
      <itunes:episodeType>full</itunes:episodeType>
    </item>
  </channel>
</rss>`

func TestParseFeedRSS_StripHTML(t *testing.T) {
	feed, err := parseFeed(strings.NewReader(rssFixture), parseOptions{MaxItems: 20, StripHTML: true})
	if err != nil {
		t.Fatalf("parseFeed: %v", err)
	}
	if feed.Title != "Example RSS" {
		t.Errorf("title = %q", feed.Title)
	}
	if feed.Language != "pt-BR" {
		t.Errorf("language = %q", feed.Language)
	}
	if feed.Description != "An example feed" {
		t.Errorf("description stripped = %q", feed.Description)
	}
	if feed.ItemCount != 2 || len(feed.Items) != 2 {
		t.Fatalf("item count = %d", feed.ItemCount)
	}
	if got := feed.Items[0].Summary; got != "Hello world" {
		t.Errorf("item summary stripped = %q", got)
	}
	if feed.Items[0].Published != "2006-01-02T15:04:05Z" {
		t.Errorf("published normalized = %q", feed.Items[0].Published)
	}
	if feed.IsPodcast {
		t.Error("rss should not be flagged as podcast")
	}
}

func TestParseFeedRSS_NoStrip(t *testing.T) {
	feed, err := parseFeed(strings.NewReader(rssFixture), parseOptions{MaxItems: 20, StripHTML: false})
	if err != nil {
		t.Fatalf("parseFeed: %v", err)
	}
	if !strings.Contains(feed.Items[0].Summary, "<strong>") {
		t.Errorf("expected raw HTML preserved, got %q", feed.Items[0].Summary)
	}
}

func TestParseFeedMaxItems(t *testing.T) {
	feed, err := parseFeed(strings.NewReader(rssFixture), parseOptions{MaxItems: 1, StripHTML: true})
	if err != nil {
		t.Fatalf("parseFeed: %v", err)
	}
	if feed.ItemCount != 1 {
		t.Errorf("expected 1 item, got %d", feed.ItemCount)
	}
}

func TestParseFeedSince(t *testing.T) {
	since := time.Date(2006, 1, 3, 0, 0, 0, 0, time.UTC)
	feed, err := parseFeed(strings.NewReader(rssFixture), parseOptions{MaxItems: 20, StripHTML: true, Since: &since})
	if err != nil {
		t.Fatalf("parseFeed: %v", err)
	}
	if feed.ItemCount != 1 {
		t.Fatalf("expected 1 item after since, got %d", feed.ItemCount)
	}
	if feed.Items[0].Title != "Second post" {
		t.Errorf("expected Second post, got %q", feed.Items[0].Title)
	}
}

func TestParseFeedAtom(t *testing.T) {
	feed, err := parseFeed(strings.NewReader(atomFixture), parseOptions{MaxItems: 20, StripHTML: true})
	if err != nil {
		t.Fatalf("parseFeed: %v", err)
	}
	if feed.Title != "Example Atom" {
		t.Errorf("title = %q", feed.Title)
	}
	if len(feed.Items) != 1 || feed.Items[0].Title != "Atom entry" {
		t.Fatalf("unexpected items: %+v", feed.Items)
	}
	if len(feed.Authors) != 1 || feed.Authors[0] != "Jane Doe" {
		t.Errorf("authors = %v", feed.Authors)
	}
}

func TestParseFeedJSON_IncludeContent(t *testing.T) {
	feed, err := parseFeed(strings.NewReader(jsonFeedFixture), parseOptions{MaxItems: 20, StripHTML: true, IncludeContent: true})
	if err != nil {
		t.Fatalf("parseFeed: %v", err)
	}
	if feed.Title != "Example JSON Feed" {
		t.Errorf("title = %q", feed.Title)
	}
	if len(feed.Items) != 1 {
		t.Fatalf("items = %d", len(feed.Items))
	}
	if feed.Items[0].Content != "Body" {
		t.Errorf("content (stripped) = %q", feed.Items[0].Content)
	}
}

func TestParseFeedJSON_ExcludeContent(t *testing.T) {
	feed, err := parseFeed(strings.NewReader(jsonFeedFixture), parseOptions{MaxItems: 20, StripHTML: true, IncludeContent: false})
	if err != nil {
		t.Fatalf("parseFeed: %v", err)
	}
	if feed.Items[0].Content != "" {
		t.Errorf("content should be empty when not included, got %q", feed.Items[0].Content)
	}
}

func TestParseFeedPodcast(t *testing.T) {
	feed, err := parseFeed(strings.NewReader(podcastFixture), parseOptions{MaxItems: 20, StripHTML: true})
	if err != nil {
		t.Fatalf("parseFeed: %v", err)
	}
	if !feed.IsPodcast {
		t.Fatal("expected is_podcast=true")
	}
	if feed.Podcast == nil {
		t.Fatal("expected podcast feed block")
	}
	if feed.Podcast.Author != "The Host" {
		t.Errorf("podcast author = %q", feed.Podcast.Author)
	}
	if feed.Podcast.Owner != "Owner Name" {
		t.Errorf("podcast owner = %q", feed.Podcast.Owner)
	}
	if len(feed.Podcast.Categories) != 1 || feed.Podcast.Categories[0] != "Technology" {
		t.Errorf("podcast categories = %v", feed.Podcast.Categories)
	}
	if len(feed.Items) != 1 {
		t.Fatalf("items = %d", len(feed.Items))
	}
	it := feed.Items[0]
	if len(it.Enclosures) != 1 || it.Enclosures[0].Type != "audio/mpeg" {
		t.Errorf("enclosures = %+v", it.Enclosures)
	}
	if it.Podcast == nil {
		t.Fatal("expected item podcast block")
	}
	if it.Podcast.Duration != "00:42:10" || it.Podcast.Episode != "1" || it.Podcast.Season != "1" || it.Podcast.EpisodeType != "full" {
		t.Errorf("item podcast = %+v", it.Podcast)
	}
}

func TestFeedReadExecute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rssFixture))
	}))
	defer srv.Close()

	tool := NewFeedRead(nil)
	tool.allowPrivateHosts = true // httptest usa 127.0.0.1

	args, _ := json.Marshal(map[string]any{"url": srv.URL, "max_items": 5})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Content)
	}
	var parsed CanonicalFeed
	if err := json.Unmarshal([]byte(res.Content), &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v\n%s", err, res.Content)
	}
	if parsed.Title != "Example RSS" || parsed.ItemCount != 2 {
		t.Errorf("unexpected parsed feed: title=%q items=%d", parsed.Title, parsed.ItemCount)
	}
	if res.Metadata["item_count"] != 2 {
		t.Errorf("metadata item_count = %v", res.Metadata["item_count"])
	}
}

func TestFeedReadExecuteRejectsPrivateHost(t *testing.T) {
	tool := NewFeedRead(nil) // allowPrivateHosts = false
	args, _ := json.Marshal(map[string]any{"url": "http://localhost:9999/feed.xml"})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.IsError {
		t.Error("expected error for private host")
	}
}

func TestFeedReadExecuteInvalidScheme(t *testing.T) {
	tool := NewFeedRead(nil)
	args, _ := json.Marshal(map[string]any{"url": "ftp://example.com/feed"})
	res, _ := tool.Execute(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for non-http scheme")
	}
}
