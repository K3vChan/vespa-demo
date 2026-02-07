import { useCallback } from 'react';
import { useAppState, useAppDispatch } from '../../context/AppContext';
import { savePreferences } from '../../api/client';
import styles from './Preferences.module.css';

export default function SavePreferences() {
  const { currentUserId, prefState, saveStatus } = useAppState();
  const dispatch = useAppDispatch();

  const handleSave = useCallback(async () => {
    if (!currentUserId) return;

    dispatch({ type: 'SET_SAVE_STATUS', payload: 'saving' });

    const preferences = Object.entries(prefState).map(([key, state]) => {
      const [type, ...rest] = key.split(':');
      return { type, value: rest.join(':'), state };
    });

    await savePreferences(currentUserId, preferences);

    dispatch({ type: 'UPDATE_USER_PREFS', payload: { userId: currentUserId, preferences } });
    dispatch({ type: 'BUMP_SEARCH_VERSION' });
    dispatch({ type: 'SET_SAVE_STATUS', payload: 'saved' });

    setTimeout(() => dispatch({ type: 'SET_SAVE_STATUS', payload: '' }), 2000);
  }, [currentUserId, prefState, dispatch]);

  return (
    <div>
      <button
        className={styles.saveBtn}
        onClick={handleSave}
        disabled={saveStatus === 'saving'}
      >
        Save Preferences
      </button>
      {saveStatus && (
        <span className={styles.saveStatus}>
          {saveStatus === 'saving' ? 'Saving...' : 'Saved!'}
        </span>
      )}
    </div>
  );
}
