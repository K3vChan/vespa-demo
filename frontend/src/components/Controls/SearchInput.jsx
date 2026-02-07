import { useAppState, useAppDispatch } from '../../context/AppContext';
import styles from './Controls.module.css';

export default function SearchInput() {
  const { searchQuery } = useAppState();
  const dispatch = useAppDispatch();

  return (
    <input
      className={styles.input}
      type="text"
      placeholder="Search films (e.g. thriller, Nolan, space)..."
      value={searchQuery}
      onChange={(e) => dispatch({ type: 'SET_SEARCH_QUERY', payload: e.target.value })}
    />
  );
}
