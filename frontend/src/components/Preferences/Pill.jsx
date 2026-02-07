import { useAppState, useAppDispatch } from '../../context/AppContext';
import { PILL_STATES } from '../../constants';
import styles from './Preferences.module.css';

export default function Pill({ type, value }) {
  const { prefState } = useAppState();
  const dispatch = useAppDispatch();

  const key = `${type}:${value}`;
  const current = prefState[key] || 'neutral';
  const nextState = PILL_STATES[(PILL_STATES.indexOf(current) + 1) % PILL_STATES.length];

  let className = styles.pill;
  if (current === 'like') className += ` ${styles.like}`;
  else if (current === 'dislike') className += ` ${styles.dislike}`;

  return (
    <span
      className={className}
      onClick={() => dispatch({ type: 'TOGGLE_PREF', payload: { key, nextState } })}
    >
      {value}
    </span>
  );
}
