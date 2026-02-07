import { useAppState, useAppDispatch } from '../../context/AppContext';
import styles from './Controls.module.css';

export default function UserSelector() {
  const { users, currentUserId } = useAppState();
  const dispatch = useAppDispatch();

  return (
    <select
      className={styles.select}
      value={currentUserId || ''}
      onChange={(e) => dispatch({ type: 'SET_CURRENT_USER', payload: e.target.value })}
    >
      {users.map((u) => (
        <option key={u.id} value={u.id}>
          {u.name}
        </option>
      ))}
    </select>
  );
}
