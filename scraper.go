package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// apiApp is the subset of the catalog API item we use.
type apiApp struct {
	Slug               string   `json:"slug"`
	ProjectName        string   `json:"projectName"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Consoles           []string `json:"consoles"`
	AddedAt            int64    `json:"addedAt"`
	LastReleaseAt      int64    `json:"lastReleaseAt"`
	LastReleaseVersion string   `json:"lastReleaseVersion"`
	Artwork            string   `json:"artwork"`
	Developer          struct {
		Name string `json:"name"`
	} `json:"developer"`
	Games []struct {
		Title string `json:"title"`
		Slug  string `json:"slug"`
	} `json:"games"`
	LibraryArt struct {
		Header string `json:"header"`
	} `json:"libraryArt"`
}

type apiPage struct {
	Items      []apiApp `json:"items"`
	NextCursor *string  `json:"nextCursor"`
}

// appDetails is what we scrape from the app's HTML page.
type appDetails struct {
	RepoURL     string
	ReadmeHTML  string
	Screenshots []string
}

type client struct {
	cfg  config
	http *http.Client
}

func (c *client) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", u, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

// listRecomps returns up to n recomps from the catalog API, ordered by sort.
func (c *client) listRecomps(ctx context.Context, n int) ([]apiApp, error) {
	var out []apiApp
	cursor := ""
	for len(out) < n {
		q := url.Values{}
		q.Set("kinds", "recomp")
		q.Set("sort", c.cfg.Sort)
		q.Set("limit", fmt.Sprint(min(100, n-len(out))))
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		body, err := c.get(ctx, c.cfg.APIURL+"/apps?"+q.Encode())
		if err != nil {
			return nil, err
		}
		var page apiPage
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("decode catalog: %w", err)
		}
		out = append(out, page.Items...)
		if page.NextCursor == nil || *page.NextCursor == "" || len(page.Items) == 0 {
			break
		}
		cursor = *page.NextCursor
	}
	if len(out) > n {
		out = out[:n]
	}
	return out, nil
}

func (c *client) appURL(slug string) string {
	return strings.TrimRight(c.cfg.SiteURL, "/") + "/apps/" + url.PathEscape(slug)
}

// fetchDetails scrapes the server-rendered app page for the repo, README and screenshots.
func (c *client) fetchDetails(ctx context.Context, slug string) (appDetails, error) {
	body, err := c.get(ctx, c.appURL(slug))
	if err != nil {
		return appDetails{}, err
	}
	return parseDetails(body)
}

func parseDetails(body []byte) (appDetails, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return appDetails{}, err
	}
	var d appDetails
	var readme *html.Node

	walk(doc, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		switch n.Data {
		case "script":
			if attr(n, "type") == "application/ld+json" && n.FirstChild != nil {
				var ld struct {
					CodeRepository string `json:"codeRepository"`
				}
				if json.Unmarshal([]byte(n.FirstChild.Data), &ld) == nil && ld.CodeRepository != "" {
					d.RepoURL = ld.CodeRepository
				}
			}
		case "section":
			// <section><h2>README</h2><div class="markdown">…</div></section>
			if readme == nil && sectionHeading(n) == "README" {
				for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
					if ch.Type == html.ElementNode && ch.Data == "div" && hasClass(ch, "markdown") {
						readme = ch
					}
				}
			}
		}
		return true
	})

	// Fallback for the repo: the "Source:" link in the info list.
	if d.RepoURL == "" {
		walk(doc, func(n *html.Node) bool {
			if n.Type == html.ElementNode && n.Data == "li" && strings.HasPrefix(strings.TrimSpace(textOf(n)), "Source:") {
				walk(n, func(a *html.Node) bool {
					if a.Type == html.ElementNode && a.Data == "a" && d.RepoURL == "" {
						d.RepoURL = attr(a, "href")
					}
					return true
				})
			}
			return d.RepoURL == ""
		})
	}

	if readme != nil {
		seen := map[string]bool{}
		walk(readme, func(n *html.Node) bool {
			if n.Type == html.ElementNode && n.Data == "img" {
				src := attr(n, "src")
				if isScreenshot(src) && !seen[src] {
					seen[src] = true
					d.Screenshots = append(d.Screenshots, src)
				}
			}
			return true
		})
		collapseWhitespace(readme)
		var buf bytes.Buffer
		for ch := readme.FirstChild; ch != nil; ch = ch.NextSibling {
			_ = html.Render(&buf, ch)
		}
		d.ReadmeHTML = strings.TrimSpace(buf.String())
	}
	return d, nil
}

// collapseWhitespace replaces whitespace-only text nodes (the site's rendered README has
// long runs of blank lines) with a single newline, leaving <pre> contents untouched.
func collapseWhitespace(n *html.Node) {
	walk(n, func(n *html.Node) bool {
		if n.Type == html.ElementNode && (n.Data == "pre" || n.Data == "textarea") {
			return false
		}
		if n.Type == html.TextNode && strings.TrimSpace(n.Data) == "" && strings.Contains(n.Data, "\n") {
			n.Data = "\n"
		}
		return true
	})
}

// isScreenshot filters out badges, logos-as-SVG and other non-screenshot images.
func isScreenshot(src string) bool {
	u, err := url.Parse(src)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host, p := strings.ToLower(u.Host), strings.ToLower(u.Path)
	for _, bad := range []string{"shields.io", "badgen.net", "badge.fury.io", "codecov.io", "travis-ci", "circleci", "ko-fi.com", "buymeacoffee", "discordapp.com/api", "img.youtube.com"} {
		if strings.Contains(host+p, bad) {
			return false
		}
	}
	if strings.HasSuffix(p, ".svg") || strings.Contains(p, "/badge") || strings.Contains(p, "/workflows/") {
		return false
	}
	return true
}

func sectionHeading(sec *html.Node) string {
	for ch := sec.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type == html.ElementNode && (ch.Data == "h2" || ch.Data == "h3") {
			return strings.TrimSpace(textOf(ch))
		}
	}
	return ""
}

func walk(n *html.Node, fn func(*html.Node) bool) {
	if !fn(n) {
		return
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		walk(ch, fn)
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

func textOf(n *html.Node) string {
	var b strings.Builder
	walk(n, func(n *html.Node) bool {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		return true
	})
	return b.String()
}

func msToTime(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}
