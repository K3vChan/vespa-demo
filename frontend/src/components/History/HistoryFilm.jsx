import styles from './History.module.css';

export default function HistoryFilm({ film }) {
  const stars = '\u2605'.repeat(film.user_rating) + '\u2606'.repeat(5 - film.user_rating);

  return (
    <div className={styles.historyFilm}>
      <span className={styles.hfName}>{film.film_title}</span>
      <span className={styles.hfMeta}>
        {' '}&middot; {film.film_genre} &middot; {film.film_year}{' '}
        <span className={styles.stars}>{stars}</span>
        {' '}
        {(film.film_tags || []).map((t) => (
          <span key={t} className={styles.tag}>{t}</span>
        ))}
      </span>
    </div>
  );
}
