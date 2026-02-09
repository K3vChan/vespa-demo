package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// =============================================================================
// Tier 1: Pure functions
// =============================================================================

func TestIsValidPreferenceValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid alpha", "Action", true},
		{"valid hyphen", "cult-favorite", true},
		{"valid mixed case hyphen", "Sci-Fi", true},
		{"valid underscore", "foo_bar", true},
		{"valid single char", "a", true},
		{"valid numeric", "123", true},
		{"valid alphanumeric", "abc123", true},
		{"empty string", "", false},
		{"too long", strings.Repeat("a", 51), false},
		{"exactly 50 chars", strings.Repeat("a", 50), true},
		{"space", "foo bar", false},
		{"semicolon", "foo;bar", false},
		{"angle bracket", "<script>", false},
		{"ampersand", "foo&bar", false},
		{"slash", "foo/bar", false},
		{"dot", "foo.bar", false},
		{"equals", "foo=bar", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidPreferenceValue(tt.input)
			if got != tt.want {
				t.Errorf("isValidPreferenceValue(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	const testKey = "TEST_GETENV_KEY_12345"

	t.Run("returns env value when set", func(t *testing.T) {
		os.Setenv(testKey, "custom-value")
		t.Cleanup(func() { os.Unsetenv(testKey) })

		got := getEnv(testKey, "default")
		if got != "custom-value" {
			t.Errorf("getEnv(%q, %q) = %q, want %q", testKey, "default", got, "custom-value")
		}
	})

	t.Run("returns default when unset", func(t *testing.T) {
		os.Unsetenv(testKey)

		got := getEnv(testKey, "fallback")
		if got != "fallback" {
			t.Errorf("getEnv(%q, %q) = %q, want %q", testKey, "fallback", got, "fallback")
		}
	})

	t.Run("returns default when empty", func(t *testing.T) {
		os.Setenv(testKey, "")
		t.Cleanup(func() { os.Unsetenv(testKey) })

		got := getEnv(testKey, "fallback")
		if got != "fallback" {
			t.Errorf("getEnv(%q, %q) = %q, want %q", testKey, "fallback", got, "fallback")
		}
	})
}

// =============================================================================
// Tier 2: buildVespaQuery
// =============================================================================

func TestBuildVespaQuery(t *testing.T) {
	oldURL := vespaBaseURL
	t.Cleanup(func() { vespaBaseURL = oldURL })
	vespaBaseURL = "http://test:8080"

	parseQuery := func(t *testing.T, rawURL string) url.Values {
		t.Helper()
		u, err := url.Parse(rawURL)
		if err != nil {
			t.Fatalf("failed to parse URL: %v", err)
		}
		return u.Query()
	}

	t.Run("wildcard query uses where true", func(t *testing.T) {
		result := buildVespaQuery("*", nil, 10)
		params := parseQuery(t, result)
		yql := params.Get("yql")
		if !strings.Contains(yql, "where true") {
			t.Errorf("wildcard YQL should contain 'where true', got: %s", yql)
		}
		if params.Get("query") != "" {
			t.Error("wildcard query should not set 'query' param")
		}
		if params.Get("hits") != "10" {
			t.Errorf("hits should be 10, got: %s", params.Get("hits"))
		}
	})

	t.Run("text query uses userQuery", func(t *testing.T) {
		result := buildVespaQuery("dark knight", nil, 20)
		params := parseQuery(t, result)
		yql := params.Get("yql")
		if !strings.Contains(yql, "userQuery()") {
			t.Errorf("text query YQL should contain 'userQuery()', got: %s", yql)
		}
		if params.Get("query") != "dark knight" {
			t.Errorf("query param should be 'dark knight', got: %s", params.Get("query"))
		}
	})

	t.Run("genre likes set ranking and genre_boost", func(t *testing.T) {
		prefs := []Preference{
			{Type: "genre", Value: "Action", State: "like"},
			{Type: "genre", Value: "Sci-Fi", State: "like"},
		}
		result := buildVespaQuery("*", prefs, 10)
		params := parseQuery(t, result)

		if params.Get("ranking.profile") != "personalized" {
			t.Errorf("expected ranking.profile=personalized, got: %s", params.Get("ranking.profile"))
		}
		boost := params.Get("input.query(genre_boost)")
		if !strings.Contains(boost, "{genre:Action}:1") {
			t.Errorf("genre_boost should contain Action, got: %s", boost)
		}
		if !strings.Contains(boost, "{genre:Sci-Fi}:1") {
			t.Errorf("genre_boost should contain Sci-Fi, got: %s", boost)
		}
	})

	t.Run("genre dislikes set genre_penalty", func(t *testing.T) {
		prefs := []Preference{
			{Type: "genre", Value: "Horror", State: "dislike"},
		}
		result := buildVespaQuery("*", prefs, 10)
		params := parseQuery(t, result)

		penalty := params.Get("input.query(genre_penalty)")
		if !strings.Contains(penalty, "{genre:Horror}:1") {
			t.Errorf("genre_penalty should contain Horror, got: %s", penalty)
		}
	})

	t.Run("tag likes and dislikes set tensors", func(t *testing.T) {
		prefs := []Preference{
			{Type: "tag", Value: "blockbuster", State: "like"},
			{Type: "tag", Value: "indie", State: "dislike"},
		}
		result := buildVespaQuery("*", prefs, 10)
		params := parseQuery(t, result)

		tagBoost := params.Get("input.query(tag_boost)")
		if !strings.Contains(tagBoost, "{tag:blockbuster}:1") {
			t.Errorf("tag_boost should contain blockbuster, got: %s", tagBoost)
		}
		tagPenalty := params.Get("input.query(tag_penalty)")
		if !strings.Contains(tagPenalty, "{tag:indie}:1") {
			t.Errorf("tag_penalty should contain indie, got: %s", tagPenalty)
		}
	})

	t.Run("mixed preferences set all four tensors", func(t *testing.T) {
		prefs := []Preference{
			{Type: "genre", Value: "Action", State: "like"},
			{Type: "genre", Value: "Romance", State: "dislike"},
			{Type: "tag", Value: "classic", State: "like"},
			{Type: "tag", Value: "sequel", State: "dislike"},
		}
		result := buildVespaQuery("test", prefs, 10)
		params := parseQuery(t, result)

		if params.Get("ranking.profile") != "personalized" {
			t.Error("expected ranking.profile=personalized")
		}
		if !strings.Contains(params.Get("input.query(genre_boost)"), "Action") {
			t.Error("missing genre_boost for Action")
		}
		if !strings.Contains(params.Get("input.query(genre_penalty)"), "Romance") {
			t.Error("missing genre_penalty for Romance")
		}
		if !strings.Contains(params.Get("input.query(tag_boost)"), "classic") {
			t.Error("missing tag_boost for classic")
		}
		if !strings.Contains(params.Get("input.query(tag_penalty)"), "sequel") {
			t.Error("missing tag_penalty for sequel")
		}
	})

	t.Run("empty preferences no ranking profile", func(t *testing.T) {
		result := buildVespaQuery("*", []Preference{}, 10)
		params := parseQuery(t, result)

		if params.Get("ranking.profile") != "" {
			t.Errorf("empty prefs should not set ranking.profile, got: %s", params.Get("ranking.profile"))
		}
	})

	t.Run("nil preferences no ranking profile", func(t *testing.T) {
		result := buildVespaQuery("*", nil, 10)
		params := parseQuery(t, result)

		if params.Get("ranking.profile") != "" {
			t.Errorf("nil prefs should not set ranking.profile, got: %s", params.Get("ranking.profile"))
		}
	})

	t.Run("YQL never references id field", func(t *testing.T) {
		result := buildVespaQuery("*", nil, 10)
		params := parseQuery(t, result)
		yql := params.Get("yql")
		if strings.Contains(yql, "id =") || strings.Contains(yql, "id=") {
			t.Errorf("YQL should not reference 'id' field (not in Vespa schema), got: %s", yql)
		}
	})

	t.Run("invalid preference values skipped", func(t *testing.T) {
		prefs := []Preference{
			{Type: "genre", Value: "Action", State: "like"},
			{Type: "genre", Value: "bad value!", State: "like"}, // invalid: has space and !
		}
		result := buildVespaQuery("*", prefs, 10)
		params := parseQuery(t, result)

		boost := params.Get("input.query(genre_boost)")
		if !strings.Contains(boost, "Action") {
			t.Errorf("valid value should be present, got: %s", boost)
		}
		if strings.Contains(boost, "bad value!") {
			t.Errorf("invalid value should be skipped, got: %s", boost)
		}
	})

	t.Run("hits parameter passed through", func(t *testing.T) {
		result := buildVespaQuery("*", nil, 42)
		params := parseQuery(t, result)
		if params.Get("hits") != "42" {
			t.Errorf("hits should be 42, got: %s", params.Get("hits"))
		}
	})

	t.Run("base URL is used", func(t *testing.T) {
		result := buildVespaQuery("*", nil, 10)
		if !strings.HasPrefix(result, "http://test:8080/search/?") {
			t.Errorf("URL should start with vespaBaseURL, got: %s", result)
		}
	})
}

// =============================================================================
// Tier 3: HTTP handlers
// =============================================================================

func setupTestDB(t *testing.T) {
	t.Helper()
	var err error
	testDB, err := sql.Open("sqlite", "file::memory:?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	_, err = testDB.Exec(`
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
		t.Fatalf("failed to create tables: %v", err)
	}

	// Seed test user
	testDB.Exec("INSERT INTO users (id, name) VALUES ('1', 'TestUser')")
	testDB.Exec("INSERT INTO user_preferences (user_id, pref_type, pref_value, pref_state) VALUES ('1', 'genre', 'Action', 'like')")
	testDB.Exec("INSERT INTO user_preferences (user_id, pref_type, pref_value, pref_state) VALUES ('1', 'tag', 'classic', 'dislike')")
	tagsJSON, _ := json.Marshal([]string{"blockbuster", "visually-stunning"})
	testDB.Exec("INSERT INTO watch_history (user_id, film_id, film_title, film_genre, film_year, film_tags, user_rating) VALUES ('1', 'film-1', 'Test Film', 'Action', 2020, ?, 4)", string(tagsJSON))

	oldDB := db
	db = testDB
	t.Cleanup(func() {
		testDB.Close()
		db = oldDB
	})
}

func TestHandleHealth(t *testing.T) {
	setupTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "healthy" {
		t.Errorf("expected status=healthy, got %s", resp["status"])
	}
}

func TestHandleUsers(t *testing.T) {
	setupTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	w := httptest.NewRecorder()

	handleUsers(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var users []UserResponse
	if err := json.NewDecoder(w.Body).Decode(&users); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0].Name != "TestUser" {
		t.Errorf("expected name=TestUser, got %s", users[0].Name)
	}
	if len(users[0].Preferences) != 2 {
		t.Errorf("expected 2 preferences, got %d", len(users[0].Preferences))
	}
}

func TestHandleUpdatePreferences(t *testing.T) {
	t.Run("valid update", func(t *testing.T) {
		setupTestDB(t)

		body := `{"preferences":[{"type":"genre","value":"Comedy","state":"like"},{"type":"tag","value":"indie","state":"dislike"}]}`
		req := httptest.NewRequest(http.MethodPut, "/api/users/1/preferences", strings.NewReader(body))
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()

		handleUpdatePreferences(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		// Verify preferences were saved
		prefs := getUserPreferences(context.Background(), "1")
		if len(prefs) != 2 {
			t.Fatalf("expected 2 preferences after update, got %d", len(prefs))
		}
		found := false
		for _, p := range prefs {
			if p.Type == "genre" && p.Value == "Comedy" && p.State == "like" {
				found = true
			}
		}
		if !found {
			t.Error("expected Comedy genre like preference after update")
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		setupTestDB(t)

		body := `{"preferences":[{"type":"invalid","value":"Action","state":"like"}]}`
		req := httptest.NewRequest(http.MethodPut, "/api/users/1/preferences", strings.NewReader(body))
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()

		handleUpdatePreferences(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("invalid genre value", func(t *testing.T) {
		setupTestDB(t)

		body := `{"preferences":[{"type":"genre","value":"NotAGenre","state":"like"}]}`
		req := httptest.NewRequest(http.MethodPut, "/api/users/1/preferences", strings.NewReader(body))
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()

		handleUpdatePreferences(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("invalid tag value", func(t *testing.T) {
		setupTestDB(t)

		body := `{"preferences":[{"type":"tag","value":"not-a-tag","state":"like"}]}`
		req := httptest.NewRequest(http.MethodPut, "/api/users/1/preferences", strings.NewReader(body))
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()

		handleUpdatePreferences(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("invalid state", func(t *testing.T) {
		setupTestDB(t)

		body := `{"preferences":[{"type":"genre","value":"Action","state":"neutral"}]}`
		req := httptest.NewRequest(http.MethodPut, "/api/users/1/preferences", strings.NewReader(body))
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()

		handleUpdatePreferences(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("user not found", func(t *testing.T) {
		setupTestDB(t)

		body := `{"preferences":[]}`
		req := httptest.NewRequest(http.MethodPut, "/api/users/999/preferences", strings.NewReader(body))
		req.SetPathValue("id", "999")
		w := httptest.NewRecorder()

		handleUpdatePreferences(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})
}

func TestHandleHistory(t *testing.T) {
	setupTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/users/1/history", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	handleHistory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var history []WatchHistoryEntry
	if err := json.NewDecoder(w.Body).Decode(&history); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}
	if history[0].FilmTitle != "Test Film" {
		t.Errorf("expected film_title=Test Film, got %s", history[0].FilmTitle)
	}
	if history[0].UserRating != 4 {
		t.Errorf("expected user_rating=4, got %d", history[0].UserRating)
	}
	if len(history[0].FilmTags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(history[0].FilmTags))
	}
}

func TestHandleHistory_EmptyUser(t *testing.T) {
	setupTestDB(t)
	// Add user with no history
	db.Exec("INSERT INTO users (id, name) VALUES ('2', 'EmptyUser')")

	req := httptest.NewRequest(http.MethodGet, "/api/users/2/history", nil)
	req.SetPathValue("id", "2")
	w := httptest.NewRecorder()

	handleHistory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var history []WatchHistoryEntry
	json.NewDecoder(w.Body).Decode(&history)
	if len(history) != 0 {
		t.Errorf("expected empty history, got %d entries", len(history))
	}
}

func TestHandleAddHistory(t *testing.T) {
	t.Run("valid add", func(t *testing.T) {
		setupTestDB(t)

		body := `{"film_id":"film-99","film_title":"New Film","film_genre":"Comedy","film_year":2023,"film_tags":["indie"],"user_rating":3}`
		req := httptest.NewRequest(http.MethodPost, "/api/users/1/history", strings.NewReader(body))
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()

		handleAddHistory(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		// Verify it was added
		var count int
		db.QueryRow("SELECT COUNT(*) FROM watch_history WHERE user_id = '1'").Scan(&count)
		if count != 2 {
			t.Errorf("expected 2 history entries after add, got %d", count)
		}
	})

	t.Run("rating too low", func(t *testing.T) {
		setupTestDB(t)

		body := `{"film_id":"film-99","film_title":"Bad","film_genre":"Comedy","film_year":2023,"film_tags":[],"user_rating":0}`
		req := httptest.NewRequest(http.MethodPost, "/api/users/1/history", strings.NewReader(body))
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()

		handleAddHistory(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for rating 0, got %d", w.Code)
		}
	})

	t.Run("rating too high", func(t *testing.T) {
		setupTestDB(t)

		body := `{"film_id":"film-99","film_title":"Bad","film_genre":"Comedy","film_year":2023,"film_tags":[],"user_rating":6}`
		req := httptest.NewRequest(http.MethodPost, "/api/users/1/history", strings.NewReader(body))
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()

		handleAddHistory(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for rating 6, got %d", w.Code)
		}
	})

	t.Run("user not found", func(t *testing.T) {
		setupTestDB(t)

		body := `{"film_id":"film-99","film_title":"X","film_genre":"Comedy","film_year":2023,"film_tags":[],"user_rating":3}`
		req := httptest.NewRequest(http.MethodPost, "/api/users/999/history", strings.NewReader(body))
		req.SetPathValue("id", "999")
		w := httptest.NewRecorder()

		handleAddHistory(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})
}

func TestHandleSearch(t *testing.T) {
	setupTestDB(t)

	// Mock Vespa server
	vespaResp := VespaResponse{}
	vespaResp.Root.Fields.TotalCount = 1
	vespaResp.Root.Children = []VespaHit{
		{
			ID:        "id:films:film::3",
			Relevance: 10.5,
			Fields: struct {
				Title       string   `json:"title"`
				Description string   `json:"description"`
				Genre       string   `json:"genre"`
				Director    string   `json:"director"`
				Year        int      `json:"year"`
				Rating      float64  `json:"rating"`
				Tags        []string `json:"tags"`
				Cast        []string `json:"cast"`
			}{
				Title: "The Dark Knight",
				Genre: "Action",
				Year:  2008,
				Tags:  []string{"blockbuster"},
			},
		},
	}

	mockVespa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(vespaResp)
	}))
	defer mockVespa.Close()

	oldURL := vespaBaseURL
	vespaBaseURL = mockVespa.URL
	t.Cleanup(func() { vespaBaseURL = oldURL })

	req := httptest.NewRequest(http.MethodGet, "/api/search?q=dark+knight&user=1", nil)
	w := httptest.NewRecorder()

	handleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result VespaResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.Root.Fields.TotalCount != 1 {
		t.Errorf("expected totalCount=1, got %d", result.Root.Fields.TotalCount)
	}
	if len(result.Root.Children) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(result.Root.Children))
	}
	if result.Root.Children[0].Fields.Title != "The Dark Knight" {
		t.Errorf("expected title=The Dark Knight, got %s", result.Root.Children[0].Fields.Title)
	}
}

func TestHandleSearch_EmptyQuery(t *testing.T) {
	setupTestDB(t)

	mockVespa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the query that reaches Vespa uses wildcard
		yql := r.URL.Query().Get("yql")
		if !strings.Contains(yql, "where true") {
			t.Errorf("empty query should use 'where true', got yql: %s", yql)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"root":{"fields":{"totalCount":0},"children":[]}}`)
	}))
	defer mockVespa.Close()

	oldURL := vespaBaseURL
	vespaBaseURL = mockVespa.URL
	t.Cleanup(func() { vespaBaseURL = oldURL })

	req := httptest.NewRequest(http.MethodGet, "/api/search", nil)
	w := httptest.NewRecorder()

	handleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleSearch_QueryTooLong(t *testing.T) {
	setupTestDB(t)

	longQuery := strings.Repeat("a", maxQueryLength+1)
	req := httptest.NewRequest(http.MethodGet, "/api/search?q="+longQuery, nil)
	w := httptest.NewRecorder()

	handleSearch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for long query, got %d", w.Code)
	}
}

func TestHandleRecommendations(t *testing.T) {
	setupTestDB(t)

	// Add a second watched film
	tagsJSON, _ := json.Marshal([]string{"classic"})
	db.Exec("INSERT INTO watch_history (user_id, film_id, film_title, film_genre, film_year, film_tags, user_rating) VALUES ('1', 'film-2', 'Watched Film 2', 'Drama', 2019, ?, 3)", string(tagsJSON))

	// Mock Vespa returns 8 films, some of which the user has watched
	vespaResp := VespaResponse{}
	vespaResp.Root.Fields.TotalCount = 8
	var hits []VespaHit
	for i := 1; i <= 8; i++ {
		hits = append(hits, VespaHit{
			ID:        fmt.Sprintf("id:films:film::film-%d", i),
			Relevance: float64(100 - i),
			Fields: struct {
				Title       string   `json:"title"`
				Description string   `json:"description"`
				Genre       string   `json:"genre"`
				Director    string   `json:"director"`
				Year        int      `json:"year"`
				Rating      float64  `json:"rating"`
				Tags        []string `json:"tags"`
				Cast        []string `json:"cast"`
			}{
				Title: fmt.Sprintf("Film %d", i),
				Genre: "Action",
				Year:  2020,
			},
		})
	}
	vespaResp.Root.Children = hits

	mockVespa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(vespaResp)
	}))
	defer mockVespa.Close()

	oldURL := vespaBaseURL
	vespaBaseURL = mockVespa.URL
	t.Cleanup(func() { vespaBaseURL = oldURL })

	req := httptest.NewRequest(http.MethodGet, "/api/users/1/recommendations", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	handleRecommendations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result VespaResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// film-1 and film-2 are watched, so they should be filtered out
	for _, hit := range result.Root.Children {
		filmID := ""
		if parts := strings.Split(hit.ID, "::"); len(parts) == 2 {
			filmID = parts[1]
		}
		if filmID == "film-1" || filmID == "film-2" {
			t.Errorf("watched film %s should have been filtered out", filmID)
		}
	}

	if len(result.Root.Children) > 5 {
		t.Errorf("expected max 5 recommendations, got %d", len(result.Root.Children))
	}

	if result.Root.Fields.TotalCount != len(result.Root.Children) {
		t.Errorf("totalCount (%d) should match children length (%d)", result.Root.Fields.TotalCount, len(result.Root.Children))
	}
}

// =============================================================================
// Integration tests: DB → buildVespaQuery → Vespa mock → response
// These verify the fix for the "Field 'id' does not exist" Vespa error.
// The old code built YQL like: id = 'id:films:film::1' which Vespa rejected
// because 'id' is not a document field. The fix removes YQL-based ID exclusion
// and relies on client-side filtering in handleRecommendations.
// =============================================================================

func TestIntegration_RecommendationsQueryHasNoIDField(t *testing.T) {
	setupTestDB(t)

	// Add several watched films so the old code would have generated id filters
	for i := 1; i <= 10; i++ {
		tagsJSON, _ := json.Marshal([]string{"classic"})
		db.Exec(
			"INSERT INTO watch_history (user_id, film_id, film_title, film_genre, film_year, film_tags, user_rating) VALUES ('1', ?, ?, 'Action', 2020, ?, 4)",
			fmt.Sprintf("film-%d", i+100), fmt.Sprintf("Watched %d", i), string(tagsJSON),
		)
	}

	var capturedYQL string
	mockVespa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedYQL = r.URL.Query().Get("yql")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(VespaResponse{})
	}))
	defer mockVespa.Close()

	oldURL := vespaBaseURL
	vespaBaseURL = mockVespa.URL
	t.Cleanup(func() { vespaBaseURL = oldURL })

	req := httptest.NewRequest(http.MethodGet, "/api/users/1/recommendations", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	handleRecommendations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// The critical assertion: YQL must NOT contain "id =" references
	// This was the root cause of the Vespa 400 error
	if strings.Contains(capturedYQL, "id =") || strings.Contains(capturedYQL, "id=") {
		t.Errorf("YQL must not reference 'id' field (Vespa schema has no 'id' field), got: %s", capturedYQL)
	}
	if !strings.Contains(capturedYQL, "where true") {
		t.Errorf("recommendations wildcard query should use 'where true', got: %s", capturedYQL)
	}
}

func TestIntegration_RecommendationsRequestsEnoughHits(t *testing.T) {
	setupTestDB(t)

	// User has 1 watched film from setupTestDB. Add 4 more.
	for i := 2; i <= 5; i++ {
		tagsJSON, _ := json.Marshal([]string{"classic"})
		db.Exec(
			"INSERT INTO watch_history (user_id, film_id, film_title, film_genre, film_year, film_tags, user_rating) VALUES ('1', ?, ?, 'Action', 2020, ?, 3)",
			fmt.Sprintf("film-%d", i), fmt.Sprintf("Watched %d", i), string(tagsJSON),
		)
	}
	// Now user has 5 watched films

	var capturedHits string
	mockVespa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHits = r.URL.Query().Get("hits")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(VespaResponse{})
	}))
	defer mockVespa.Close()

	oldURL := vespaBaseURL
	vespaBaseURL = mockVespa.URL
	t.Cleanup(func() { vespaBaseURL = oldURL })

	req := httptest.NewRequest(http.MethodGet, "/api/users/1/recommendations", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	handleRecommendations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// With 5 watched films, hits should be at least 10 (5 watched + 5 desired)
	if capturedHits == "5" {
		t.Errorf("hits should be > 5 to account for client-side filtering of %d watched films, got: %s", 5, capturedHits)
	}
}

func TestIntegration_RecommendationsFiltersWatchedClientSide(t *testing.T) {
	setupTestDB(t)

	// User has film-1 watched from setupTestDB. Add film-3.
	tagsJSON, _ := json.Marshal([]string{"indie"})
	db.Exec(
		"INSERT INTO watch_history (user_id, film_id, film_title, film_genre, film_year, film_tags, user_rating) VALUES ('1', 'film-3', 'Another Watched', 'Drama', 2021, ?, 5)",
		string(tagsJSON),
	)

	// Mock returns films including watched ones (since Vespa no longer filters them)
	vespaResp := VespaResponse{}
	for i := 1; i <= 7; i++ {
		vespaResp.Root.Children = append(vespaResp.Root.Children, VespaHit{
			ID:        fmt.Sprintf("id:films:film::film-%d", i),
			Relevance: float64(100 - i),
			Fields: struct {
				Title       string   `json:"title"`
				Description string   `json:"description"`
				Genre       string   `json:"genre"`
				Director    string   `json:"director"`
				Year        int      `json:"year"`
				Rating      float64  `json:"rating"`
				Tags        []string `json:"tags"`
				Cast        []string `json:"cast"`
			}{
				Title: fmt.Sprintf("Film %d", i),
				Genre: "Action",
				Year:  2020,
			},
		})
	}
	vespaResp.Root.Fields.TotalCount = len(vespaResp.Root.Children)

	mockVespa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(vespaResp)
	}))
	defer mockVespa.Close()

	oldURL := vespaBaseURL
	vespaBaseURL = mockVespa.URL
	t.Cleanup(func() { vespaBaseURL = oldURL })

	req := httptest.NewRequest(http.MethodGet, "/api/users/1/recommendations", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	handleRecommendations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result VespaResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	// Watched films (film-1, film-3) must not appear in results
	watchedIDs := map[string]bool{"film-1": true, "film-3": true}
	for _, hit := range result.Root.Children {
		if parts := strings.Split(hit.ID, "::"); len(parts) == 2 {
			if watchedIDs[parts[1]] {
				t.Errorf("watched film %s should not appear in recommendations", parts[1])
			}
		}
	}

	// Should get exactly 5 (7 from Vespa minus 2 watched = 5)
	if len(result.Root.Children) != 5 {
		t.Errorf("expected 5 recommendations, got %d", len(result.Root.Children))
	}
}

func TestIntegration_PreferencesFlowThroughToVespaQuery(t *testing.T) {
	setupTestDB(t)

	// Update preferences via the handler
	body := `{"preferences":[{"type":"genre","value":"Sci-Fi","state":"like"},{"type":"genre","value":"Horror","state":"dislike"},{"type":"tag","value":"visually-stunning","state":"like"}]}`
	putReq := httptest.NewRequest(http.MethodPut, "/api/users/1/preferences", strings.NewReader(body))
	putReq.SetPathValue("id", "1")
	putW := httptest.NewRecorder()
	handleUpdatePreferences(putW, putReq)
	if putW.Code != http.StatusOK {
		t.Fatalf("failed to update preferences: %d %s", putW.Code, putW.Body.String())
	}

	// Now call recommendations and verify the Vespa query includes the new preferences
	var capturedParams url.Values
	mockVespa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedParams = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(VespaResponse{})
	}))
	defer mockVespa.Close()

	oldURL := vespaBaseURL
	vespaBaseURL = mockVespa.URL
	t.Cleanup(func() { vespaBaseURL = oldURL })

	req := httptest.NewRequest(http.MethodGet, "/api/users/1/recommendations", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	handleRecommendations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify the preferences from the DB made it into the Vespa query
	if capturedParams.Get("ranking.profile") != "personalized" {
		t.Errorf("expected ranking.profile=personalized, got: %s", capturedParams.Get("ranking.profile"))
	}
	genreBoost := capturedParams.Get("input.query(genre_boost)")
	if !strings.Contains(genreBoost, "Sci-Fi") {
		t.Errorf("genre_boost should contain Sci-Fi from DB preferences, got: %s", genreBoost)
	}
	genrePenalty := capturedParams.Get("input.query(genre_penalty)")
	if !strings.Contains(genrePenalty, "Horror") {
		t.Errorf("genre_penalty should contain Horror from DB preferences, got: %s", genrePenalty)
	}
	tagBoost := capturedParams.Get("input.query(tag_boost)")
	if !strings.Contains(tagBoost, "visually-stunning") {
		t.Errorf("tag_boost should contain visually-stunning from DB preferences, got: %s", tagBoost)
	}

	// YQL must be clean (no id field references)
	yql := capturedParams.Get("yql")
	if strings.Contains(yql, "id =") {
		t.Errorf("YQL must not reference 'id' field, got: %s", yql)
	}
}

func TestIntegration_SearchWithDBPreferences(t *testing.T) {
	setupTestDB(t)
	// User 1 has: genre=Action/like, tag=classic/dislike from setupTestDB

	var capturedParams url.Values
	mockVespa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedParams = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(VespaResponse{})
	}))
	defer mockVespa.Close()

	oldURL := vespaBaseURL
	vespaBaseURL = mockVespa.URL
	t.Cleanup(func() { vespaBaseURL = oldURL })

	req := httptest.NewRequest(http.MethodGet, "/api/search?q=adventure&user=1", nil)
	w := httptest.NewRecorder()

	handleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Preferences from DB should flow into the search query
	if capturedParams.Get("ranking.profile") != "personalized" {
		t.Errorf("expected personalized ranking, got: %s", capturedParams.Get("ranking.profile"))
	}
	if !strings.Contains(capturedParams.Get("input.query(genre_boost)"), "Action") {
		t.Errorf("genre_boost should include Action from DB, got: %s", capturedParams.Get("input.query(genre_boost)"))
	}
	if !strings.Contains(capturedParams.Get("input.query(tag_penalty)"), "classic") {
		t.Errorf("tag_penalty should include classic from DB, got: %s", capturedParams.Get("input.query(tag_penalty)"))
	}

	// Verify text query was forwarded
	if capturedParams.Get("query") != "adventure" {
		t.Errorf("expected query=adventure, got: %s", capturedParams.Get("query"))
	}
}
