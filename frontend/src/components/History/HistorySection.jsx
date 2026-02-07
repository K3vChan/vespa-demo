import { useAppState, useAppDispatch } from '../../context/AppContext';
import HistoryFilm from './HistoryFilm';
import styles from './History.module.css';

export default function HistorySection() {
  const { history, historyOpen } = useAppState();
  const dispatch = useAppDispatch();

  return (
    <div className={styles.historySection}>
      <h2
        className={historyOpen ? styles.toggleOpen : styles.toggle}
        onClick={() => dispatch({ type: 'TOGGLE_HISTORY' })}
      >
        Watch History
      </h2>
      {historyOpen && (
        <div className={styles.historyContent}>
          {(!history || history.length === 0) ? (
            <div className={styles.historyEmpty}>
              No watch history yet.
            </div>
          ) : (
            history.map((film, i) => <HistoryFilm key={i} film={film} />)
          )}
        </div>
      )}
    </div>
  );
}
