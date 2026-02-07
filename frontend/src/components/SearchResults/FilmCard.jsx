import Tag from './Tag';
import styles from './SearchResults.module.css';

export default function FilmCard({ hit }) {
  const f = hit.fields;
  return (
    <div className={styles.film}>
      <div className={styles.filmHeader}>
        <span className={styles.filmName}>{f.title} ({f.year})</span>
        <span className={styles.filmScore}>score: {hit.relevance.toFixed(4)}</span>
      </div>
      <div className={styles.filmDesc}>{f.description}</div>
      <div className={styles.filmMeta}>
        <Tag type="genre" value={f.genre} />
        {(f.tags || []).map((t) => (
          <Tag key={t} type="tag" value={t} />
        ))}
        {' '}&middot; {f.director}
        {' '}&middot; {f.rating?.toFixed(1)}
        {f.cast && f.cast.length > 0 && (
          <> &middot; {f.cast.join(', ')}</>
        )}
      </div>
    </div>
  );
}
