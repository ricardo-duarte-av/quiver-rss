# quiver-rss

A service that offers an RSS feed of new recomps on [Quiver Launcher](https://quiverlauncher.com/recomps), which has no RSS of its own.

Every request to the HTTP port returns the feed, whatever the path or method (`/`, `/feed.xml`, `/anything`).

Each item contains:

- Recomp name (e.g. *Fighters Megamix Recomp*)
- Original game (*Fighters Megamix*) and original system (*Sega Saturn*)
- Latest version (*v0.1.0-beta.4*) and latest update (*4 days ago (2026-11-13)*)
- Project repository URL
- Screenshots (from the project README, also given as `media:content` / `enclosure`)
- The full article (README) text

## How it works

- The recomp list comes from the catalog API the site itself uses (`api.quiverlauncher.com/api/v1/apps?kinds=recomp`).
- The repository, README and screenshots are taken from each app's page (`quiverlauncher.com/apps/<slug>`). A page is only fetched when a recomp first appears or gets a new version, so routine polls make a single request.

## Run

```sh
docker compose up -d
curl http://localhost:8080/
```

or `go run .`

## Feed format

- `<description>` is short and works well in chat: game and system, version and last update, repo link, screenshot links and a one-paragraph description.
- `<content:encoded>` has the full article: header image, details table, inline screenshots and the whole README. Full feed readers show this one.

### maubot/rss

[maubot/rss](https://github.com/maubot/rss) reads `<description>` into `$summary`. Suggested template:

```
!rss template <feed id> ### [$title]($link)
$summary
```

## Configuration (environment variables)

| Variable        | Default | Description |
|-----------------|---------|-------------|
| `PORT`          | `8080`  | HTTP port |
| `POLL_INTERVAL` | `30m`   | How often to check the site (Go duration, min `1m`) |
| `FEED_SIZE`     | `50`    | Number of recomps in the feed |
| `SORT`          | `added` | `added`: newest recomps. `updated`: newest releases (one item per new version) |

## Docker image

On every push, `.github/workflows/docker.yml` runs the tests and builds and pushes a multi-arch (amd64/arm64) image to
`ghcr.io/ricardo-duarte-av/quiver-rss`, tagged with the branch name and short SHA, and as `latest` on the default branch.
