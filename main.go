package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// --- Constants ---

const (
	PrefTypeGenre = "genre"
	PrefTypeTag   = "tag"
	PrefStateLike    = "like"
	PrefStateDislike = "dislike"

	maxQueryLength      = 500
	maxRequestBodyBytes = 1 * 1024 * 1024 // 1MB
)

var (
	db *sql.DB

	vespaBaseURL = getEnv("VESPA_URL", "http://localhost:8080")

	vespaClient = &http.Client{
		Timeout: 10 * time.Second,
	}

	validGenres = map[string]bool{
		"Action": true, "Comedy": true, "Drama": true, "Sci-Fi": true,
		"Horror": true, "Romance": true, "Thriller": true, "Animation": true,
		"Adventure": true, "Crime": true,
	}

	validTags = map[string]bool{
		"classic": true, "oscar-winner": true, "cult-favorite": true, "blockbuster": true,
		"indie": true, "adaptation": true, "sequel": true, "ensemble-cast": true,
		"visually-stunning": true, "thought-provoking": true,
	}
)

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// --- Vespa response types ---

type VespaResponse struct {
	Root struct {
		Fields struct {
			TotalCount int `json:"totalCount"`
		} `json:"fields"`
		Children []VespaHit `json:"children"`
	} `json:"root"`
}

type VespaHit struct {
	ID        string  `json:"id"`
	Relevance float64 `json:"relevance"`
	Fields    struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Genre       string   `json:"genre"`
		Director    string   `json:"director"`
		Year        int      `json:"year"`
		Rating      float64  `json:"rating"`
		Tags        []string `json:"tags"`
		Cast        []string `json:"cast"`
	} `json:"fields"`
}

// --- API response types ---

type Preference struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	State string `json:"state"`
}

type UserResponse struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Preferences []Preference `json:"preferences"`
}

type WatchHistoryEntry struct {
	FilmID     string   `json:"film_id"`
	FilmTitle  string   `json:"film_title"`
	FilmGenre  string   `json:"film_genre"`
	FilmYear   int      `json:"film_year"`
	FilmTags   []string `json:"film_tags"`
	UserRating int      `json:"user_rating"`
}

type PreferencesRequest struct {
	Preferences []Preference `json:"preferences"`
}

type AddHistoryRequest struct {
	FilmID     string   `json:"film_id"`
	FilmTitle  string   `json:"film_title"`
	FilmGenre  string   `json:"film_genre"`
	FilmYear   int      `json:"film_year"`
	FilmTags   []string `json:"film_tags"`
	UserRating int      `json:"user_rating"`
}

// --- Validation helpers ---

func isValidPreferenceValue(s string) bool {
	if len(s) == 0 || len(s) > 50 {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// --- Database setup ---

func initDB() {
	var err error
	db, err = sql.Open("sqlite", "vespa-demo.db")
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}

	if err := db.Ping(); err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS user_preferences (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			pref_type TEXT NOT NULL,
			pref_value TEXT NOT NULL,
			pref_state TEXT NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id)
		);
		CREATE TABLE IF NOT EXISTS watch_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			film_id TEXT NOT NULL,
			film_title TEXT NOT NULL,
			film_genre TEXT NOT NULL,
			film_year INTEGER NOT NULL,
			film_tags TEXT NOT NULL,
			user_rating INTEGER NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id)
		);
	`)
	if err != nil {
		log.Fatal("Failed to create tables:", err)
	}

	// Seed users if empty
	var count int
	db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if count == 0 {
		seedData()
	}

	slog.Info("Database connection established")
}

func seedData() {
	type seedUser struct {
		id    string
		name  string
		likes []Preference
	}

	users := []seedUser{
		{
			id: "1", name: "Alex",
			likes: []Preference{
				{Type: "genre", Value: "Action", State: "like"},
				{Type: "genre", Value: "Sci-Fi", State: "like"},
				{Type: "genre", Value: "Romance", State: "dislike"},
				{Type: "tag", Value: "blockbuster", State: "like"},
				{Type: "tag", Value: "visually-stunning", State: "like"},
			},
		},
		{
			id: "2", name: "Maria",
			likes: []Preference{
				{Type: "genre", Value: "Drama", State: "like"},
				{Type: "genre", Value: "Romance", State: "like"},
				{Type: "genre", Value: "Horror", State: "dislike"},
				{Type: "tag", Value: "oscar-winner", State: "like"},
				{Type: "tag", Value: "thought-provoking", State: "like"},
			},
		},
		{
			id: "3", name: "Jake",
			likes: []Preference{
				{Type: "genre", Value: "Comedy", State: "like"},
				{Type: "genre", Value: "Animation", State: "like"},
				{Type: "genre", Value: "Adventure", State: "like"},
				{Type: "genre", Value: "Drama", State: "dislike"},
				{Type: "tag", Value: "cult-favorite", State: "like"},
				{Type: "tag", Value: "blockbuster", State: "like"},
			},
		},
		{
			id: "4", name: "Priya",
			likes: []Preference{
				{Type: "genre", Value: "Thriller", State: "like"},
				{Type: "genre", Value: "Crime", State: "like"},
				{Type: "genre", Value: "Comedy", State: "dislike"},
				{Type: "tag", Value: "thought-provoking", State: "like"},
				{Type: "tag", Value: "classic", State: "like"},
			},
		},
		{
			id: "5", name: "Sam",
			likes: []Preference{
				{Type: "genre", Value: "Sci-Fi", State: "like"},
				{Type: "genre", Value: "Adventure", State: "like"},
				{Type: "genre", Value: "Horror", State: "dislike"},
				{Type: "tag", Value: "visually-stunning", State: "like"},
				{Type: "tag", Value: "adaptation", State: "like"},
			},
		},
	}

	genreFilms := map[string][]string{
		"Drama":     {"1", "5", "7", "8", "15", "90", "99"},
		"Crime":     {"2", "4", "10", "19", "47", "56", "65", "72", "81", "88", "97"},
		"Action":    {"3", "13", "18", "21", "34", "43", "52", "61", "68", "77", "84", "93"},
		"Sci-Fi":    {"6", "9", "14", "29", "40", "50", "59", "75", "91", "100"},
		"Thriller":  {"11", "16", "24", "35", "41", "51", "60", "76", "92"},
		"Animation": {"12", "17", "23", "37", "45", "54", "63", "70", "79", "86", "95"},
		"Horror":    {"22", "25", "30", "36", "44", "53", "62", "69", "78", "85", "94"},
		"Adventure": {"26", "31", "38", "46", "55", "64", "71", "80", "87", "96"},
		"Romance":   {"20", "27", "33", "42", "49", "58", "67", "74", "83"},
		"Comedy":    {"28", "32", "39", "48", "57", "66", "73", "82", "89", "98"},
	}

	type filmMeta struct {
		title string
		genre string
		year  int
		tags  []string
	}
	filmDB := map[string]filmMeta{
		"1":   {"The Shawshank Redemption", "Drama", 1994, []string{"classic", "adaptation"}},
		"2":   {"The Godfather", "Crime", 1972, []string{"classic", "oscar-winner", "adaptation"}},
		"3":   {"The Dark Knight", "Action", 2008, []string{"blockbuster", "sequel", "visually-stunning"}},
		"4":   {"Pulp Fiction", "Crime", 1994, []string{"classic", "oscar-winner", "ensemble-cast"}},
		"5":   {"Schindler's List", "Drama", 1993, []string{"classic", "oscar-winner", "thought-provoking"}},
		"6":   {"Inception", "Sci-Fi", 2010, []string{"blockbuster", "visually-stunning", "thought-provoking"}},
		"7":   {"Fight Club", "Drama", 1999, []string{"cult-favorite", "adaptation", "thought-provoking"}},
		"8":   {"Forrest Gump", "Drama", 1994, []string{"classic", "oscar-winner", "adaptation"}},
		"9":   {"The Matrix", "Sci-Fi", 1999, []string{"blockbuster", "visually-stunning", "cult-favorite"}},
		"10":  {"Goodfellas", "Crime", 1990, []string{"classic", "adaptation", "ensemble-cast"}},
		"11":  {"The Silence of the Lambs", "Thriller", 1991, []string{"classic", "oscar-winner", "adaptation"}},
		"12":  {"Spirited Away", "Animation", 2001, []string{"oscar-winner", "visually-stunning", "classic"}},
		"13":  {"Saving Private Ryan", "Action", 1998, []string{"classic", "oscar-winner", "visually-stunning"}},
		"14":  {"Interstellar", "Sci-Fi", 2014, []string{"blockbuster", "visually-stunning", "thought-provoking"}},
		"15":  {"The Green Mile", "Drama", 1999, []string{"adaptation", "classic", "thought-provoking"}},
		"16":  {"Se7en", "Thriller", 1995, []string{"classic", "thought-provoking", "visually-stunning"}},
		"17":  {"The Lion King", "Animation", 1994, []string{"classic", "oscar-winner", "blockbuster"}},
		"18":  {"Gladiator", "Action", 2000, []string{"oscar-winner", "blockbuster", "visually-stunning"}},
		"19":  {"The Departed", "Crime", 2006, []string{"oscar-winner", "ensemble-cast", "adaptation"}},
		"20":  {"When Harry Met Sally", "Romance", 1989, []string{"classic", "thought-provoking"}},
		"21":  {"Die Hard", "Action", 1988, []string{"classic", "blockbuster", "cult-favorite"}},
		"22":  {"The Shining", "Horror", 1980, []string{"classic", "adaptation", "cult-favorite"}},
		"23":  {"Toy Story", "Animation", 1995, []string{"classic", "blockbuster", "visually-stunning"}},
		"24":  {"The Prestige", "Thriller", 2006, []string{"adaptation", "thought-provoking", "ensemble-cast"}},
		"25":  {"Alien", "Horror", 1979, []string{"classic", "visually-stunning", "oscar-winner"}},
		"26":  {"Back to the Future", "Adventure", 1985, []string{"classic", "blockbuster", "cult-favorite"}},
		"27":  {"The Notebook", "Romance", 2004, []string{"adaptation", "classic", "blockbuster"}},
		"28":  {"Superbad", "Comedy", 2007, []string{"cult-favorite", "ensemble-cast", "blockbuster"}},
		"29":  {"Blade Runner 2049", "Sci-Fi", 2017, []string{"sequel", "visually-stunning", "thought-provoking"}},
		"30":  {"The Exorcist", "Horror", 1973, []string{"classic", "oscar-winner", "adaptation"}},
		"31":  {"Raiders of the Lost Ark", "Adventure", 1981, []string{"classic", "blockbuster", "oscar-winner"}},
		"32":  {"The Big Lebowski", "Comedy", 1998, []string{"cult-favorite", "classic", "ensemble-cast"}},
		"33":  {"Titanic", "Romance", 1997, []string{"oscar-winner", "blockbuster", "visually-stunning"}},
		"34":  {"Mad Max: Fury Road", "Action", 2015, []string{"oscar-winner", "visually-stunning", "blockbuster"}},
		"35":  {"Parasite", "Thriller", 2019, []string{"oscar-winner", "thought-provoking", "visually-stunning"}},
		"36":  {"Get Out", "Horror", 2017, []string{"oscar-winner", "thought-provoking", "indie"}},
		"37":  {"WALL-E", "Animation", 2008, []string{"oscar-winner", "visually-stunning", "thought-provoking"}},
		"38":  {"The Princess Bride", "Adventure", 1987, []string{"classic", "cult-favorite", "adaptation"}},
		"39":  {"Groundhog Day", "Comedy", 1993, []string{"classic", "cult-favorite", "thought-provoking"}},
		"40":  {"2001: A Space Odyssey", "Sci-Fi", 1968, []string{"classic", "visually-stunning", "thought-provoking"}},
		"41":  {"Zodiac", "Thriller", 2007, []string{"adaptation", "ensemble-cast", "thought-provoking"}},
		"42":  {"Pride & Prejudice", "Romance", 2005, []string{"adaptation", "visually-stunning"}},
		"43":  {"The Terminator", "Action", 1984, []string{"classic", "cult-favorite", "blockbuster"}},
		"44":  {"Psycho", "Horror", 1960, []string{"classic", "thought-provoking"}},
		"45":  {"Finding Nemo", "Animation", 2003, []string{"oscar-winner", "blockbuster", "visually-stunning"}},
		"46":  {"Jurassic Park", "Adventure", 1993, []string{"blockbuster", "visually-stunning", "classic"}},
		"47":  {"The Usual Suspects", "Crime", 1995, []string{"oscar-winner", "cult-favorite", "ensemble-cast"}},
		"48":  {"Bridesmaids", "Comedy", 2011, []string{"blockbuster", "ensemble-cast"}},
		"49":  {"Eternal Sunshine of the Spotless Mind", "Romance", 2004, []string{"oscar-winner", "indie", "thought-provoking"}},
		"50":  {"Arrival", "Sci-Fi", 2016, []string{"adaptation", "thought-provoking", "visually-stunning"}},
		"51":  {"No Country for Old Men", "Thriller", 2007, []string{"oscar-winner", "adaptation", "thought-provoking"}},
		"52":  {"John Wick", "Action", 2014, []string{"blockbuster", "cult-favorite", "visually-stunning"}},
		"53":  {"It", "Horror", 2017, []string{"adaptation", "blockbuster", "ensemble-cast"}},
		"54":  {"Up", "Animation", 2009, []string{"oscar-winner", "visually-stunning", "thought-provoking"}},
		"55":  {"The Lord of the Rings: The Fellowship of the Ring", "Adventure", 2001, []string{"oscar-winner", "adaptation", "blockbuster"}},
		"56":  {"Heat", "Crime", 1995, []string{"classic", "ensemble-cast", "visually-stunning"}},
		"57":  {"Airplane!", "Comedy", 1980, []string{"classic", "cult-favorite", "ensemble-cast"}},
		"58":  {"Before Sunrise", "Romance", 1995, []string{"indie", "thought-provoking", "classic"}},
		"59":  {"Ex Machina", "Sci-Fi", 2014, []string{"oscar-winner", "indie", "thought-provoking"}},
		"60":  {"Oldboy", "Thriller", 2003, []string{"cult-favorite", "visually-stunning", "thought-provoking"}},
		"61":  {"Kill Bill: Volume 1", "Action", 2003, []string{"cult-favorite", "visually-stunning", "blockbuster"}},
		"62":  {"Hereditary", "Horror", 2018, []string{"indie", "visually-stunning", "thought-provoking"}},
		"63":  {"Coco", "Animation", 2017, []string{"oscar-winner", "visually-stunning", "blockbuster"}},
		"64":  {"Indiana Jones and the Last Crusade", "Adventure", 1989, []string{"classic", "blockbuster", "sequel"}},
		"65":  {"City of God", "Crime", 2002, []string{"visually-stunning", "thought-provoking"}},
		"66":  {"The Hangover", "Comedy", 2009, []string{"blockbuster", "ensemble-cast", "cult-favorite"}},
		"67":  {"La La Land", "Romance", 2016, []string{"oscar-winner", "visually-stunning", "blockbuster"}},
		"68":  {"Terminator 2: Judgment Day", "Action", 1991, []string{"sequel", "blockbuster", "visually-stunning"}},
		"69":  {"The Thing", "Horror", 1982, []string{"cult-favorite", "classic", "visually-stunning"}},
		"70":  {"Inside Out", "Animation", 2015, []string{"oscar-winner", "blockbuster", "thought-provoking"}},
		"71":  {"The Lord of the Rings: The Return of the King", "Adventure", 2003, []string{"oscar-winner", "blockbuster", "sequel"}},
		"72":  {"Reservoir Dogs", "Crime", 1992, []string{"cult-favorite", "indie", "ensemble-cast"}},
		"73":  {"Anchorman", "Comedy", 2004, []string{"cult-favorite", "ensemble-cast", "blockbuster"}},
		"74":  {"10 Things I Hate About You", "Romance", 1999, []string{"adaptation", "cult-favorite", "classic"}},
		"75":  {"Dune", "Sci-Fi", 2021, []string{"adaptation", "blockbuster", "visually-stunning"}},
		"76":  {"Memento", "Thriller", 2000, []string{"indie", "cult-favorite", "thought-provoking"}},
		"77":  {"The Avengers", "Action", 2012, []string{"blockbuster", "sequel", "ensemble-cast"}},
		"78":  {"A Quiet Place", "Horror", 2018, []string{"blockbuster", "thought-provoking", "visually-stunning"}},
		"79":  {"Ratatouille", "Animation", 2007, []string{"oscar-winner", "visually-stunning", "thought-provoking"}},
		"80":  {"Pirates of the Caribbean", "Adventure", 2003, []string{"blockbuster", "visually-stunning"}},
		"81":  {"The Godfather Part II", "Crime", 1974, []string{"classic", "oscar-winner", "sequel"}},
		"82":  {"The Grand Budapest Hotel", "Comedy", 2014, []string{"oscar-winner", "visually-stunning", "ensemble-cast"}},
		"83":  {"Casablanca", "Romance", 1942, []string{"classic", "oscar-winner", "thought-provoking"}},
		"84":  {"The Bourne Identity", "Action", 2002, []string{"adaptation", "blockbuster", "ensemble-cast"}},
		"85":  {"The Conjuring", "Horror", 2013, []string{"blockbuster", "adaptation", "visually-stunning"}},
		"86":  {"Spider-Man: Into the Spider-Verse", "Animation", 2018, []string{"oscar-winner", "visually-stunning", "blockbuster"}},
		"87":  {"Star Wars: The Empire Strikes Back", "Adventure", 1980, []string{"classic", "sequel", "blockbuster"}},
		"88":  {"Fargo", "Crime", 1996, []string{"oscar-winner", "cult-favorite", "thought-provoking"}},
		"89":  {"Ghostbusters", "Comedy", 1984, []string{"classic", "blockbuster", "ensemble-cast"}},
		"90":  {"Moonlight", "Drama", 2016, []string{"oscar-winner", "indie", "thought-provoking"}},
		"91":  {"Gravity", "Sci-Fi", 2013, []string{"oscar-winner", "visually-stunning", "blockbuster"}},
		"92":  {"Gone Girl", "Thriller", 2014, []string{"adaptation", "thought-provoking", "blockbuster"}},
		"93":  {"Top Gun: Maverick", "Action", 2022, []string{"sequel", "blockbuster", "visually-stunning"}},
		"94":  {"Midsommar", "Horror", 2019, []string{"indie", "visually-stunning", "cult-favorite"}},
		"95":  {"The Incredibles", "Animation", 2004, []string{"oscar-winner", "blockbuster", "ensemble-cast"}},
		"96":  {"The Lord of the Rings: The Two Towers", "Adventure", 2002, []string{"blockbuster", "sequel", "visually-stunning"}},
		"97":  {"Sicario", "Crime", 2015, []string{"visually-stunning", "thought-provoking", "ensemble-cast"}},
		"98":  {"Monty Python and the Holy Grail", "Comedy", 1975, []string{"classic", "cult-favorite", "indie"}},
		"99":  {"12 Angry Men", "Drama", 1957, []string{"classic", "thought-provoking", "ensemble-cast"}},
		"100": {"Everything Everywhere All at Once", "Sci-Fi", 2022, []string{"oscar-winner", "visually-stunning", "indie"}},
	}

	tx, err := db.Begin()
	if err != nil {
		log.Fatal("Failed to begin seed transaction:", err)
	}
	defer tx.Rollback()

	for _, u := range users {
		tx.Exec("INSERT INTO users (id, name) VALUES (?, ?)", u.id, u.name)
		for _, p := range u.likes {
			tx.Exec("INSERT INTO user_preferences (user_id, pref_type, pref_value, pref_state) VALUES (?, ?, ?, ?)",
				u.id, p.Type, p.Value, p.State)
		}

		var likedGenres, dislikedGenres []string
		for _, p := range u.likes {
			if p.Type == "genre" && p.State == "like" {
				likedGenres = append(likedGenres, p.Value)
			}
			if p.Type == "genre" && p.State == "dislike" {
				dislikedGenres = append(dislikedGenres, p.Value)
			}
		}

		watched := map[string]bool{}
		for _, g := range likedGenres {
			films := genreFilms[g]
			n := 4
			if n > len(films) {
				n = len(films)
			}
			perm := rand.Perm(len(films))
			for i := 0; i < n; i++ {
				fid := films[perm[i]]
				if !watched[fid] {
					watched[fid] = true
					fm := filmDB[fid]
					tagsJSON, _ := json.Marshal(fm.tags)
					rating := 4 + rand.Intn(2)
					tx.Exec("INSERT INTO watch_history (user_id, film_id, film_title, film_genre, film_year, film_tags, user_rating) VALUES (?, ?, ?, ?, ?, ?, ?)",
						u.id, fid, fm.title, fm.genre, fm.year, string(tagsJSON), rating)
				}
			}
		}
		dislikedSet := map[string]bool{}
		for _, g := range dislikedGenres {
			dislikedSet[g] = true
		}
		likedSet := map[string]bool{}
		for _, g := range likedGenres {
			likedSet[g] = true
		}
		var neutralGenres []string
		for g := range genreFilms {
			if !likedSet[g] && !dislikedSet[g] {
				neutralGenres = append(neutralGenres, g)
			}
		}
		count := 0
		for _, g := range neutralGenres {
			if count >= 5 {
				break
			}
			films := genreFilms[g]
			if len(films) > 0 {
				fid := films[rand.Intn(len(films))]
				if !watched[fid] {
					watched[fid] = true
					fm := filmDB[fid]
					tagsJSON, _ := json.Marshal(fm.tags)
					rating := 2 + rand.Intn(3)
					tx.Exec("INSERT INTO watch_history (user_id, film_id, film_title, film_genre, film_year, film_tags, user_rating) VALUES (?, ?, ?, ?, ?, ?, ?)",
						u.id, fid, fm.title, fm.genre, fm.year, string(tagsJSON), rating)
					count++
				}
			}
		}
		for _, g := range dislikedGenres {
			films := genreFilms[g]
			if len(films) > 0 {
				fid := films[rand.Intn(len(films))]
				if !watched[fid] {
					watched[fid] = true
					fm := filmDB[fid]
					tagsJSON, _ := json.Marshal(fm.tags)
					rating := 1 + rand.Intn(2)
					tx.Exec("INSERT INTO watch_history (user_id, film_id, film_title, film_genre, film_year, film_tags, user_rating) VALUES (?, ?, ?, ?, ?, ?, ?)",
						u.id, fid, fm.title, fm.genre, fm.year, string(tagsJSON), rating)
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		log.Fatal("Failed to commit seed data:", err)
	}

	slog.Info("Seeded database with 5 users and watch history")
}

// --- Database helpers ---

func getUserPreferences(ctx context.Context, userID string) []Preference {
	rows, err := db.QueryContext(ctx, "SELECT pref_type, pref_value, pref_state FROM user_preferences WHERE user_id = ?", userID)
	if err != nil {
		slog.Error("Failed to query preferences", "user_id", userID, "error", err)
		return []Preference{}
	}
	defer rows.Close()

	var prefs []Preference
	for rows.Next() {
		var p Preference
		if err := rows.Scan(&p.Type, &p.Value, &p.State); err != nil {
			slog.Error("Failed to scan preference row", "user_id", userID, "error", err)
			continue
		}
		prefs = append(prefs, p)
	}
	if err := rows.Err(); err != nil {
		slog.Error("Error iterating preference rows", "user_id", userID, "error", err)
		return []Preference{}
	}
	if prefs == nil {
		prefs = []Preference{}
	}
	return prefs
}

func getWatchedFilmIDs(ctx context.Context, userID string) map[string]bool {
	rows, err := db.QueryContext(ctx, "SELECT film_id FROM watch_history WHERE user_id = ?", userID)
	if err != nil {
		slog.Error("Failed to query watch history", "user_id", userID, "error", err)
		return map[string]bool{}
	}
	defer rows.Close()

	watched := map[string]bool{}
	for rows.Next() {
		var filmID string
		if err := rows.Scan(&filmID); err == nil {
			watched[filmID] = true
		}
	}
	return watched
}

// --- Vespa query building ---

func buildVespaQuery(query string, prefs []Preference, hits int) string {
	params := url.Values{}
	yql := "select * from film where true"
	if query != "*" {
		yql = "select * from film where userQuery()"
		params.Set("query", query)
	}

	params.Set("yql", yql)
	params.Set("hits", fmt.Sprintf("%d", hits))

	var genreLike, genreDislike, tagLike, tagDislike []string
	for _, p := range prefs {
		switch {
		case p.Type == PrefTypeGenre && p.State == PrefStateLike:
			genreLike = append(genreLike, p.Value)
		case p.Type == PrefTypeGenre && p.State == PrefStateDislike:
			genreDislike = append(genreDislike, p.Value)
		case p.Type == PrefTypeTag && p.State == PrefStateLike:
			tagLike = append(tagLike, p.Value)
		case p.Type == PrefTypeTag && p.State == PrefStateDislike:
			tagDislike = append(tagDislike, p.Value)
		}
	}

	if len(prefs) > 0 {
		params.Set("ranking.profile", "personalized")

		if len(genreLike) > 0 {
			var parts []string
			for _, g := range genreLike {
				if !isValidPreferenceValue(g) {
					continue
				}
				parts = append(parts, fmt.Sprintf("{genre:%s}:1", g))
			}
			if len(parts) > 0 {
				params.Set("input.query(genre_boost)", "{"+strings.Join(parts, ",")+"}")
			}
		}
		if len(genreDislike) > 0 {
			var parts []string
			for _, g := range genreDislike {
				if !isValidPreferenceValue(g) {
					continue
				}
				parts = append(parts, fmt.Sprintf("{genre:%s}:1", g))
			}
			if len(parts) > 0 {
				params.Set("input.query(genre_penalty)", "{"+strings.Join(parts, ",")+"}")
			}
		}
		if len(tagLike) > 0 {
			var parts []string
			for _, t := range tagLike {
				if !isValidPreferenceValue(t) {
					continue
				}
				parts = append(parts, fmt.Sprintf("{tag:%s}:1", t))
			}
			if len(parts) > 0 {
				params.Set("input.query(tag_boost)", "{"+strings.Join(parts, ",")+"}")
			}
		}
		if len(tagDislike) > 0 {
			var parts []string
			for _, t := range tagDislike {
				if !isValidPreferenceValue(t) {
					continue
				}
				parts = append(parts, fmt.Sprintf("{tag:%s}:1", t))
			}
			if len(parts) > 0 {
				params.Set("input.query(tag_penalty)", "{"+strings.Join(parts, ",")+"}")
			}
		}
	}

	return vespaBaseURL + "/search/?" + params.Encode()
}

// --- HTTP handlers ---

func handleSearch(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	query := r.URL.Query().Get("q")
	userID := r.URL.Query().Get("user")

	if query == "" {
		query = "*"
	} else if len(query) > maxQueryLength {
		http.Error(w, fmt.Sprintf("Query too long (max %d characters)", maxQueryLength), http.StatusBadRequest)
		return
	}

	prefsJSON := r.URL.Query().Get("prefs")
	var prefs []Preference
	if prefsJSON != "" {
		if err := json.Unmarshal([]byte(prefsJSON), &prefs); err != nil {
			slog.Warn("Failed to unmarshal override preferences", "error", err)
			prefs = getUserPreferences(r.Context(), userID)
		}
	} else {
		prefs = getUserPreferences(r.Context(), userID)
	}

	vespaURL := buildVespaQuery(query, prefs, 100)

	resp, err := vespaClient.Get(vespaURL)
	if err != nil {
		slog.Error("Vespa query failed", "error", err, "query", query)
		http.Error(w, "Vespa query failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("Vespa returned error", "status", resp.StatusCode, "body", string(body))
		http.Error(w, fmt.Sprintf("Vespa error (status %d)", resp.StatusCode), http.StatusBadGateway)
		return
	}

	var vespaResp VespaResponse
	if err := json.NewDecoder(resp.Body).Decode(&vespaResp); err != nil {
		http.Error(w, "Failed to parse Vespa response: "+err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Info("Search completed", "query", query, "user_id", userID, "duration_ms", time.Since(start).Milliseconds())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vespaResp)
}

func handleUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := db.QueryContext(r.Context(), "SELECT id, name FROM users ORDER BY id")
	if err != nil {
		http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []UserResponse
	for rows.Next() {
		var u UserResponse
		if err := rows.Scan(&u.ID, &u.Name); err != nil {
			slog.Error("Failed to scan user row", "error", err)
			continue
		}
		u.Preferences = getUserPreferences(r.Context(), u.ID)
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		slog.Error("Error iterating user rows", "error", err)
		http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

func handleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")

	var exists int
	if err := db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM users WHERE id = ?", userID).Scan(&exists); err != nil {
		slog.Error("Failed to check user existence", "user_id", userID, "error", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	if exists == 0 {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	var req PreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	for _, p := range req.Preferences {
		if p.Type != PrefTypeGenre && p.Type != PrefTypeTag {
			http.Error(w, "Invalid preference type: "+p.Type, http.StatusBadRequest)
			return
		}
		if p.State != PrefStateLike && p.State != PrefStateDislike {
			http.Error(w, "Invalid preference state: "+p.State, http.StatusBadRequest)
			return
		}
		if p.Type == PrefTypeGenre && !validGenres[p.Value] {
			http.Error(w, "Invalid genre: "+p.Value, http.StatusBadRequest)
			return
		}
		if p.Type == PrefTypeTag && !validTags[p.Value] {
			http.Error(w, "Invalid tag: "+p.Value, http.StatusBadRequest)
			return
		}
	}

	tx, err := db.Begin()
	if err != nil {
		slog.Error("Failed to begin transaction", "error", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM user_preferences WHERE user_id = ?", userID); err != nil {
		slog.Error("Failed to delete preferences", "user_id", userID, "error", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	for _, p := range req.Preferences {
		if _, err := tx.Exec("INSERT INTO user_preferences (user_id, pref_type, pref_value, pref_state) VALUES (?, ?, ?, ?)",
			userID, p.Type, p.Value, p.State); err != nil {
			slog.Error("Failed to insert preference", "user_id", userID, "error", err)
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transaction", "user_id", userID, "error", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	slog.Info("Preferences updated", "user_id", userID, "count", len(req.Preferences))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")

	rows, err := db.QueryContext(r.Context(),
		"SELECT film_id, film_title, film_genre, film_year, film_tags, user_rating FROM watch_history WHERE user_id = ? ORDER BY id DESC",
		userID,
	)
	if err != nil {
		http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var history []WatchHistoryEntry
	for rows.Next() {
		var e WatchHistoryEntry
		var tagsJSON string
		if err := rows.Scan(&e.FilmID, &e.FilmTitle, &e.FilmGenre, &e.FilmYear, &tagsJSON, &e.UserRating); err != nil {
			slog.Error("Failed to scan history row", "user_id", userID, "error", err)
			continue
		}
		if err := json.Unmarshal([]byte(tagsJSON), &e.FilmTags); err != nil {
			e.FilmTags = []string{}
		}
		if e.FilmTags == nil {
			e.FilmTags = []string{}
		}
		history = append(history, e)
	}
	if err := rows.Err(); err != nil {
		slog.Error("Error iterating history rows", "user_id", userID, "error", err)
		http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if history == nil {
		history = []WatchHistoryEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

func handleAddHistory(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")

	var exists int
	if err := db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM users WHERE id = ?", userID).Scan(&exists); err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	if exists == 0 {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	var req AddHistoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.UserRating < 1 || req.UserRating > 5 {
		http.Error(w, "Rating must be between 1 and 5", http.StatusBadRequest)
		return
	}

	tagsJSON, _ := json.Marshal(req.FilmTags)
	if _, err := db.ExecContext(r.Context(),
		"INSERT INTO watch_history (user_id, film_id, film_title, film_genre, film_year, film_tags, user_rating) VALUES (?, ?, ?, ?, ?, ?, ?)",
		userID, req.FilmID, req.FilmTitle, req.FilmGenre, req.FilmYear, string(tagsJSON), req.UserRating,
	); err != nil {
		http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleRecommendations(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")

	prefs := getUserPreferences(r.Context(), userID)
	watchedMap := getWatchedFilmIDs(r.Context(), userID)

	// Request extra hits to account for client-side watched-film filtering
	vespaURL := buildVespaQuery("*", prefs, len(watchedMap)+5)

	resp, err := vespaClient.Get(vespaURL)
	if err != nil {
		slog.Error("Vespa query failed for recommendations", "error", err)
		http.Error(w, "Vespa query failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("Vespa returned error for recommendations", "status", resp.StatusCode, "body", string(body))
		http.Error(w, fmt.Sprintf("Vespa error (status %d)", resp.StatusCode), http.StatusBadGateway)
		return
	}

	var vespaResp VespaResponse
	if err := json.NewDecoder(resp.Body).Decode(&vespaResp); err != nil {
		http.Error(w, "Failed to parse Vespa response: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Filter out watched films and take top 5
	var recs []VespaHit
	for _, hit := range vespaResp.Root.Children {
		filmID := ""
		if parts := strings.Split(hit.ID, "::"); len(parts) == 2 {
			filmID = parts[1]
		}
		if !watchedMap[filmID] {
			recs = append(recs, hit)
			if len(recs) >= 5 {
				break
			}
		}
	}

	result := VespaResponse{}
	result.Root.Fields.TotalCount = len(recs)
	result.Root.Children = recs

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := db.Ping(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "unhealthy",
			"error":  err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

func main() {
	initDB()
	defer db.Close()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /api/search", handleSearch)
	mux.HandleFunc("GET /api/users", handleUsers)
	mux.HandleFunc("PUT /api/users/{id}/preferences", handleUpdatePreferences)
	mux.HandleFunc("GET /api/users/{id}/history", handleHistory)
	mux.HandleFunc("POST /api/users/{id}/history", handleAddHistory)
	mux.HandleFunc("GET /api/users/{id}/recommendations", handleRecommendations)
	mux.Handle("GET /", http.FileServer(http.Dir("static/dist")))

	fmt.Println("Server running at http://localhost:3000")
	log.Fatal(http.ListenAndServe(":3000", mux))
}