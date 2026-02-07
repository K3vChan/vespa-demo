export async function fetchUsers() {
  const resp = await fetch('/api/users');
  return resp.json();
}

export async function savePreferences(userId, preferences) {
  const resp = await fetch(`/api/users/${userId}/preferences`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ preferences }),
  });
  return resp.json();
}

export async function fetchHistory(userId) {
  const resp = await fetch(`/api/users/${userId}/history`);
  return resp.json();
}

export async function searchFilms(query, userId, preferences) {
  const params = new URLSearchParams({ q: query || '*', user: userId || '' });
  if (preferences) {
    params.set('prefs', JSON.stringify(preferences));
  }
  const resp = await fetch(`/api/search?${params}`);
  return resp.json();
}

export async function fetchRecommendations(userId) {
  const resp = await fetch(`/api/users/${userId}/recommendations`);
  return resp.json();
}
