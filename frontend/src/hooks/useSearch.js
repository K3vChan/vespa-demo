import { useEffect, useRef } from 'react';
import { searchFilms } from '../api/client';
import { useAppState, useAppDispatch } from '../context/AppContext';
import { useDebounce } from './useDebounce';

export function useSearch() {
  const { searchQuery, currentUserId, prefState, searchVersion } = useAppState();
  const dispatch = useAppDispatch();
  const debouncedQuery = useDebounce(searchQuery, 300);
  const versionRef = useRef(0);

  useEffect(() => {
    if (!currentUserId) return;

    const thisVersion = ++versionRef.current;

    const preferences = Object.entries(prefState).map(([key, state]) => {
      const [type, value] = key.split(':');
      return { type, value, state };
    });

    searchFilms(debouncedQuery, currentUserId, preferences).then((data) => {
      if (versionRef.current !== thisVersion) return;
      dispatch({ type: 'SET_SEARCH_RESULTS', payload: data });
    });
  }, [debouncedQuery, currentUserId, prefState, searchVersion, dispatch]);
}
