import { useCallback } from 'react';
import { useAppState, useAppDispatch } from '../../context/AppContext';
import { fetchRecommendations } from '../../api/client';
import FilmCard from '../SearchResults/FilmCard';
import styles from './Recommendations.module.css';

export default function Recommendations() {
  const { currentUserId, recommendations, recommendLoading } = useAppState();
  const dispatch = useAppDispatch();

  const handleRecommend = useCallback(async () => {
    if (!currentUserId) return;
    dispatch({ type: 'SET_RECOMMEND_LOADING', payload: true });
    const data = await fetchRecommendations(currentUserId);
    dispatch({ type: 'SET_RECOMMENDATIONS', payload: data });
    dispatch({ type: 'SET_RECOMMEND_LOADING', payload: false });
  }, [currentUserId, dispatch]);

  const films = recommendations?.root?.children || [];

  return (
    <div className={styles.recsSection}>
      <div className={styles.recsHeader}>
        <h2>Recommendations</h2>
        <button
          className={styles.recBtn}
          onClick={handleRecommend}
          disabled={recommendLoading}
        >
          {recommendLoading ? 'Loading...' : 'Recommend 5 Films'}
        </button>
      </div>
      {films.length > 0 && (
        <div className={styles.recsList}>
          {films.map((hit, i) => (
            <FilmCard key={i} hit={hit} />
          ))}
        </div>
      )}
    </div>
  );
}
