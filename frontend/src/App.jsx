import { useEffect } from 'react';
import { AppProvider, useAppState, useAppDispatch } from './context/AppContext';
import { useSearch } from './hooks/useSearch';
import { fetchUsers, fetchHistory } from './api/client';
import Header from './components/Header/Header';
import Controls from './components/Controls/Controls';
import PreferencesSection from './components/Preferences/PreferencesSection';
import Recommendations from './components/Recommendations/Recommendations';
import HistorySection from './components/History/HistorySection';
import SearchResults from './components/SearchResults/SearchResults';

function AppInner() {
  const { currentUserId } = useAppState();
  const dispatch = useAppDispatch();

  useEffect(() => {
    fetchUsers().then((users) => {
      dispatch({ type: 'SET_USERS', payload: users });
      if (users.length > 0) {
        dispatch({ type: 'SET_CURRENT_USER', payload: users[0].id });
      }
    });
  }, [dispatch]);

  useEffect(() => {
    if (!currentUserId) return;
    fetchHistory(currentUserId).then((data) => {
      dispatch({ type: 'SET_HISTORY', payload: data });
    });
  }, [currentUserId, dispatch]);

  useSearch();

  return (
    <>
      <Header />
      <Controls />
      <PreferencesSection />
      <Recommendations />
      <SearchResults />
      <HistorySection />
    </>
  );
}

export default function App() {
  return (
    <AppProvider>
      <AppInner />
    </AppProvider>
  );
}
