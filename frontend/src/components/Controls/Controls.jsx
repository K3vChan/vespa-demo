import UserSelector from './UserSelector';
import SearchInput from './SearchInput';
import styles from './Controls.module.css';

export default function Controls() {
  return (
    <div className={styles.controls}>
      <UserSelector />
      <SearchInput />
    </div>
  );
}
