import { useAppState } from '../../context/AppContext';
import styles from './SearchResults.module.css';

export default function Tag({ type, value }) {
  const { prefState } = useAppState();
  const state = prefState[`${type}:${value}`] || 'neutral';

  let className = styles.tag;
  if (state === 'like') className += ` ${styles.boosted}`;
  else if (state === 'dislike') className += ` ${styles.penalized}`;

  return <span className={className}>{value}</span>;
}
