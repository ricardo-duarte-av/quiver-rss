package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type entry struct {
	App     apiApp
	Details appDetails
	HasDet  bool
}

type service struct {
	cfg    config
	client *client

	mu      sync.RWMutex
	entries []entry
	updated time.Time
	details map[string]appDetails // key: slug + "@" + version
}

func newService(cfg config) *service {
	return &service{
		cfg:     cfg,
		client:  &client{cfg: cfg, http: &http.Client{Timeout: 30 * time.Second}},
		details: map[string]appDetails{},
	}
}

func (s *service) run(ctx context.Context) {
	t := time.NewTicker(s.cfg.PollInterval)
	defer t.Stop()
	for {
		if err := s.refresh(ctx); err != nil && ctx.Err() == nil {
			log.Printf("refresh failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func detailKey(a apiApp) string { return a.Slug + "@" + a.LastReleaseVersion }

func (s *service) refresh(ctx context.Context) error {
	apps, err := s.client.listRecomps(ctx, s.cfg.FeedSize)
	if err != nil {
		return err
	}

	s.mu.RLock()
	known := make(map[string]appDetails, len(s.details))
	for k, v := range s.details {
		known[k] = v
	}
	s.mu.RUnlock()

	// Only scrape pages for apps that are new or had a new release; be polite (sequential + delay).
	entries := make([]entry, 0, len(apps))
	fresh := map[string]appDetails{}
	fetched := 0
	for _, a := range apps {
		key := detailKey(a)
		d, ok := known[key]
		if !ok {
			if fetched > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(500 * time.Millisecond):
				}
			}
			fetched++
			var err error
			d, err = s.client.fetchDetails(ctx, a.Slug)
			if err != nil {
				log.Printf("details for %s: %v (will retry next poll)", a.Slug, err)
				entries = append(entries, entry{App: a})
				continue
			}
			ok = true
		}
		fresh[key] = d
		entries = append(entries, entry{App: a, Details: d, HasDet: ok})
	}

	s.mu.Lock()
	s.entries = entries
	s.details = fresh // drops apps that fell out of the feed
	s.updated = time.Now().UTC()
	s.mu.Unlock()
	log.Printf("refreshed: %d items, %d detail pages fetched", len(entries), fetched)
	return nil
}

// --- RSS rendering ---

type rss struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Content string     `xml:"xmlns:content,attr"`
	Atom    string     `xml:"xmlns:atom,attr"`
	Media   string     `xml:"xmlns:media,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	AtomLink    atomLink  `xml:"atom:link"`
	Description string    `xml:"description"`
	Language    string    `xml:"language"`
	LastBuild   string    `xml:"lastBuildDate,omitempty"`
	TTL         int       `xml:"ttl"`
	Items       []rssItem `xml:"item"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type rssItem struct {
	Title       string       `xml:"title"`
	Link        string       `xml:"link"`
	GUID        rssGUID      `xml:"guid"`
	PubDate     string       `xml:"pubDate,omitempty"`
	Author      string       `xml:"dc:creator,omitempty"`
	Categories  []string     `xml:"category"`
	Description cdata        `xml:"description"`
	Content     cdata        `xml:"content:encoded"`
	Enclosure   *enclosure   `xml:"enclosure,omitempty"`
	Media       []mediaThumb `xml:"media:content"`
}

type rssGUID struct {
	Value       string `xml:",chardata"`
	IsPermaLink bool   `xml:"isPermaLink,attr"`
}

type cdata struct {
	Text string `xml:",cdata"`
}

type enclosure struct {
	URL    string `xml:"url,attr"`
	Type   string `xml:"type,attr"`
	Length int    `xml:"length,attr"`
}

type mediaThumb struct {
	URL    string `xml:"url,attr"`
	Medium string `xml:"medium,attr"`
}

var itemTmpl = template.Must(template.New("item").Parse(`
{{- if .Header}}<p><img src="{{.Header}}" alt="{{.Name}}" style="max-width:100%"/></p>{{end}}
<table>
<tr><th align="left">Recomp</th><td>{{.Name}}</td></tr>
<tr><th align="left">Original game</th><td>{{.Game}}</td></tr>
<tr><th align="left">Original system</th><td>{{.System}}</td></tr>
<tr><th align="left">Latest version</th><td>{{if .Version}}{{.Version}}{{else}}—{{end}}</td></tr>
<tr><th align="left">Latest update</th><td>{{if .Updated}}{{.UpdatedAgo}} ({{.Updated}}){{else}}—{{end}}</td></tr>
<tr><th align="left">Developer</th><td>{{.Developer}}</td></tr>
<tr><th align="left">Repository</th><td>{{if .Repo}}<a href="{{.Repo}}">{{.Repo}}</a>{{else}}—{{end}}</td></tr>
<tr><th align="left">Quiver page</th><td><a href="{{.Link}}">{{.Link}}</a></td></tr>
</table>
{{- if .Description}}
<p>{{.Description}}</p>
{{- end}}
{{- if .Screenshots}}
<h3>Screenshots</h3>
{{range .Screenshots}}<p><img src="{{.}}" style="max-width:100%"/></p>
{{end}}
{{- end}}
{{- if .Readme}}
<h3>Article</h3>
{{.Readme}}
{{- end}}`))

type itemView struct {
	Name, Game, System, Version, Updated, UpdatedAgo string
	Developer, Repo, Link, Description, Header       string
	Screenshots                                      []string
	Readme                                           template.HTML
}

func (s *service) buildItem(e entry, now time.Time) rssItem {
	a := e.App
	link := s.client.appURL(a.Slug)
	name := a.ProjectName
	if name == "" {
		name = a.Name
	}
	games := make([]string, 0, len(a.Games))
	for _, g := range a.Games {
		games = append(games, g.Title)
	}
	game := strings.Join(games, ", ")
	if game == "" {
		game = a.Name
	}
	v := itemView{
		Name: name, Game: game, System: systemsOf(a.Consoles),
		Version: a.LastReleaseVersion, Developer: a.Developer.Name,
		Link: link, Description: a.Description, Header: a.LibraryArt.Header,
		Repo: e.Details.RepoURL, Screenshots: e.Details.Screenshots,
		// README HTML comes from the site's own server-side markdown render.
		Readme: template.HTML(e.Details.ReadmeHTML),
	}
	if rel := msToTime(a.LastReleaseAt); !rel.IsZero() {
		v.Updated = rel.Format("2006-01-02")
		v.UpdatedAgo = humanAgo(now.Sub(rel))
	}
	var body bytes.Buffer
	if err := itemTmpl.Execute(&body, v); err != nil {
		log.Printf("template %s: %v", a.Slug, err)
	}

	pub := msToTime(a.AddedAt)
	guid := a.Slug
	if s.cfg.Sort == "updated" {
		pub = msToTime(a.LastReleaseAt)
		guid = detailKey(a)
	}
	title := name
	if v.Version != "" {
		title += " " + v.Version
	}
	it := rssItem{
		Title:       fmt.Sprintf("%s (%s)", title, v.System),
		Link:        link,
		GUID:        rssGUID{Value: guid, IsPermaLink: false},
		Author:      a.Developer.Name,
		Description: cdata{body.String()},
		Content:     cdata{body.String()},
	}
	if !pub.IsZero() {
		it.PubDate = pub.Format(time.RFC1123Z)
	}
	if v.System != "" {
		it.Categories = append(it.Categories, v.System)
	}
	if game != "" {
		it.Categories = append(it.Categories, game)
	}
	imgs := e.Details.Screenshots
	if len(imgs) == 0 && a.LibraryArt.Header != "" {
		imgs = []string{a.LibraryArt.Header}
	}
	for _, img := range imgs {
		it.Media = append(it.Media, mediaThumb{URL: img, Medium: "image"})
	}
	if len(imgs) > 0 {
		it.Enclosure = &enclosure{URL: imgs[0], Type: imageType(imgs[0])}
	}
	return it
}

func imageType(u string) string {
	l := strings.ToLower(u)
	switch {
	case strings.Contains(l, ".png"):
		return "image/png"
	case strings.Contains(l, ".gif"):
		return "image/gif"
	case strings.Contains(l, ".webp"):
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

func humanAgo(d time.Duration) string {
	plural := func(n int, unit string) string {
		if n == 1 {
			return "1 " + unit + " ago"
		}
		return fmt.Sprintf("%d %ss ago", n, unit)
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour")
	case d < 30*24*time.Hour:
		return plural(int(d.Hours()/24), "day")
	case d < 365*24*time.Hour:
		return plural(int(d.Hours()/24/30), "month")
	default:
		return plural(int(d.Hours()/24/365), "year")
	}
}

func (s *service) render(selfURL string) ([]byte, error) {
	s.mu.RLock()
	entries, updated := s.entries, s.updated
	s.mu.RUnlock()

	now := time.Now().UTC()
	ch := rssChannel{
		Title:       "Quiver Launcher – Recomps",
		Link:        strings.TrimRight(s.cfg.SiteURL, "/") + "/recomps",
		AtomLink:    atomLink{Href: selfURL, Rel: "self", Type: "application/rss+xml"},
		Description: "Console games recompiled to run natively, from quiverlauncher.com/recomps",
		Language:    "en",
		TTL:         int(s.cfg.PollInterval.Minutes()),
	}
	if s.cfg.Sort == "updated" {
		ch.Title = "Quiver Launcher – Recomp releases"
	}
	if !updated.IsZero() {
		ch.LastBuild = updated.Format(time.RFC1123Z)
	}
	for _, e := range entries {
		ch.Items = append(ch.Items, s.buildItem(e, now))
	}
	out, err := xml.MarshalIndent(rss{
		Version: "2.0",
		Content: "http://purl.org/rss/1.0/modules/content/",
		Atom:    "http://www.w3.org/2005/Atom",
		Media:   "http://search.yahoo.com/mrss/",
		Channel: ch,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	// dc:creator needs its namespace too.
	out = bytes.Replace(out, []byte(`<rss `), []byte(`<rss xmlns:dc="http://purl.org/dc/elements/1.1/" `), 1)
	return append([]byte(xml.Header), out...), nil
}

// serveFeed answers every path and method with the RSS feed.
func (s *service) serveFeed(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}
	body, err := s.render(scheme + "://" + host + r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}
