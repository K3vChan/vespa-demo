# Vespa Film Search Demo

A demo app showing how [Vespa](https://vespa.ai) handles personalized search ranking. Users search a catalog of films and get results re-ranked based on their genre and tag preferences.

- Search films by keyword (powered by Vespa BM25)
- Set per-user preferences — like or dislike any genre or tag
- See search results re-ranked in real time based on those preferences
- View watch history with star ratings (1-5)
- Get personalized film recommendations (top 5 unwatched films)

## Prerequisites

- [Go](https://go.dev/) 1.21+
- [Node.js](https://nodejs.org/) 18+ (for the React frontend)
- [Vespa CLI](https://docs.vespa.ai/en/vespa-cli.html) with a running Vespa instance on `localhost:8080`
- Python 3 (optional, for the seed script)

## Quick Start

```bash
# 1. Start Vespa (Docker example)
vespa config set target local
docker run --detach --name vespa --hostname vespa-container \
  --publish 8080:8080 --publish 19071:19071 \
  vespaengine/vespa

# 2. Deploy the Vespa application
vespa deploy vespa-app

# 3. Feed film documents
vespa feed feed.json
# or use the Python seed script:
python3 seed.py

# 4. Build the React frontend
cd frontend && npm install && npm run build && cd ..

# 5. Build and run the Go server
go build && ./vespa-demo
# or:
go run main.go
```

Open [http://localhost:3000](http://localhost:3000) in your browser.

### Development mode

For frontend development with hot reload:

```bash
# Terminal 1 — Go API server
go run main.go

# Terminal 2 — Vite dev server (proxies /api to :3000)
cd frontend && npm run dev
```

Then open [http://localhost:5173](http://localhost:5173).

## How It Works

### Personalized Ranking

Every preference item (genre or tag) can be in one of three states: **neutral**, **liked**, or **disliked**. The UI represents these as clickable pills that cycle through the states.

When a user searches, the Go server translates their preferences into four Vespa query-time tensors:

| Tensor | Effect | Weight |
|--------|--------|--------|
| `genre_boost` | Liked genres ranked higher | +10 |
| `genre_penalty` | Disliked genres ranked lower | -10 |
| `tag_boost` | Liked tags ranked higher | +5 |
| `tag_penalty` | Disliked tags ranked lower | -5 |

These are added on top of the base BM25 text relevance score. The ranking expression in `vespa-app/schemas/film.sd`:

```
bm25(title) + bm25(description)
+ if (tensorFromLabels(attribute(genre), genre) * query(genre_boost)   > 0, 10, 0)
- if (tensorFromLabels(attribute(genre), genre) * query(genre_penalty) > 0, 10, 0)
+ if (tensorFromLabels(attribute(tags), tag)    * query(tag_boost)     > 0,  5, 0)
- if (tensorFromLabels(attribute(tags), tag)    * query(tag_penalty)   > 0,  5, 0)
```

### Data Flow

```
React App (frontend/)
    |
    |  fetch /api/search?q=inception&user=1
    v
Go Server (main.go :3000)
    |
    |  1. Read Alex's preferences from SQLite
    |  2. Build Vespa query with tensor params
    |  3. GET http://localhost:8080/search/?...
    v
Vespa (:8080)
    |
    |  BM25 text match + personalized re-ranking
    v
Results returned to browser
```

## Project Structure

```
.
├── main.go                    # Go HTTP server (API + static file serving)
├── frontend/                  # React + Vite frontend
│   ├── src/
│   │   ├── App.jsx            # Root component
│   │   ├── api/client.js      # API client functions
│   │   ├── context/           # React context for app state
│   │   ├── hooks/             # Custom hooks (search, debounce)
│   │   └── components/
│   │       ├── Header/        # App header
│   │       ├── Controls/      # User selector + search input
│   │       ├── Preferences/   # Genre/tag preference pills
│   │       ├── SearchResults/ # Film cards with relevance scores
│   │       ├── Recommendations/ # Top 5 personalized film picks
│   │       └── History/       # Flat watch list with star ratings
│   ├── vite.config.js         # Vite config (builds to ../static/dist)
│   └── package.json
├── static/                    # Served by Go (includes built frontend)
├── vespa-app/
│   ├── schemas/
│   │   └── film.sd            # Vespa document schema + rank profile
│   └── services.xml           # Vespa services configuration
├── feed.json                  # 100 film documents for Vespa
├── seed.py                    # Python script to feed/reload Vespa data
├── go.mod / go.sum            # Go module files
└── CLAUDE.md                  # Claude Code project instructions
```

## Film Data

100 films across 10 genres:

| Genre |
|-------|
| Action |
| Comedy |
| Drama |
| Sci-Fi |
| Horror |
| Romance |
| Thriller |
| Animation |
| Adventure |
| Crime |

Tags: `classic`, `oscar-winner`, `cult-favorite`, `blockbuster`, `indie`, `adaptation`, `sequel`, `ensemble-cast`, `visually-stunning`, `thought-provoking`

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/search?q=...&user=...` | Search films via Vespa with personalized ranking |
| `GET` | `/api/users` | List all users with their preferences |
| `PUT` | `/api/users/{id}/preferences` | Update a user's preferences |
| `GET` | `/api/users/{id}/history` | Get a user's watch history with ratings |
| `POST` | `/api/users/{id}/history` | Add a film to watch history |
| `GET` | `/api/users/{id}/recommendations` | Get top 5 unwatched film recommendations |

### Preference format

```json
{
  "preferences": [
    { "type": "genre", "value": "Action", "state": "like" },
    { "type": "tag", "value": "blockbuster", "state": "like" },
    { "type": "genre", "value": "Romance", "state": "dislike" }
  ]
}
```

## Reloading Film Data

Edit `feed.json` then re-seed Vespa:

```bash
# Upsert all documents
python3 seed.py

# Or wipe and reload from scratch
python3 seed.py --clean

# Or use the Vespa CLI directly
vespa feed feed.json
```

## Seed Users

The SQLite database is created at startup and pre-seeded with five users:

| User | Liked Genres | Disliked Genres | Liked Tags |
|------|-------------|-----------------|------------|
| Alex | Action, Sci-Fi | Romance | blockbuster, visually-stunning |
| Maria | Drama, Romance | Horror | oscar-winner, thought-provoking |
| Jake | Comedy, Animation, Adventure | Drama | cult-favorite, blockbuster |
| Priya | Thriller, Crime | Comedy | thought-provoking, classic |
| Sam | Sci-Fi, Adventure | Horror | visually-stunning, adaptation |
