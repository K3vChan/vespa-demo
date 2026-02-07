import { useAppState } from '../../context/AppContext';
import FilmCard from './FilmCard';
import styles from './SearchResults.module.css';

export default function SearchResults() {
  const { searchResults } = useAppState();

  const films = searchResults?.root?.children || [];

  return (
    <div>
      <h2 className={styles.heading}>Search Results</h2>
      <div className={styles.count}>{films.length} results</div>
      {films.map((hit, i) => (
        <FilmCard key={i} hit={hit} />
      ))}
    </div>
  );
}
